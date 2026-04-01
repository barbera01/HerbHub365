package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"HerbHub365/services/blog-poster/internal/archive"
	"HerbHub365/services/blog-poster/internal/config"
	"HerbHub365/services/blog-poster/internal/model"
	"HerbHub365/services/blog-poster/internal/sensordata"
	"gopkg.in/yaml.v3"
)

var ErrNoSnapshots = errors.New("no snapshots found for target date")
var errInvalidRepoPost = errors.New("repo-post model output did not match blog format")

const maxGenerateAttempts = 3

type markdownGenerator interface {
	GenerateMarkdown(ctx context.Context, prompt string) (string, error)
	GenerateMarkdownWithSystemPrompt(ctx context.Context, systemPrompt, prompt string) (string, error)
}

type Generator struct {
	blogConfig   config.BlogConfig
	llmConfig    config.LLMConfig
	store        *archive.Store
	llm          markdownGenerator
	sensorWriter *sensordata.Writer
}

type PostResult struct {
	Path       string
	AssetPaths []string
}

type writeOptions struct {
	draft      bool
	categories string
	slugLabel  string
	layout     string
	extras     map[string]any
}

func NewGenerator(blogCfg config.BlogConfig, llmCfg config.LLMConfig, store *archive.Store, llm markdownGenerator, sensorWriter *sensordata.Writer) *Generator {
	return &Generator{blogConfig: blogCfg, llmConfig: llmCfg, store: store, llm: llm, sensorWriter: sensorWriter}
}

type measurement struct {
	Count int     `json:"count"`
	Min   float64 `json:"min,omitempty"`
	Max   float64 `json:"max,omitempty"`
	Avg   float64 `json:"avg,omitempty"`
	Start float64 `json:"start,omitempty"`
	End   float64 `json:"end,omitempty"`
}

type summary struct {
	Date          string                 `json:"date"`
	SnapshotCount int                    `json:"snapshot_count"`
	Devices       []string               `json:"devices,omitempty"`
	FirstSeen     string                 `json:"first_seen,omitempty"`
	LastSeen      string                 `json:"last_seen,omitempty"`
	Environment   map[string]measurement `json:"environment,omitempty"`
	Water         map[string]measurement `json:"water_reservoir,omitempty"`
	Temperatures  map[string]measurement `json:"temperatures,omitempty"`
	SoilMoisture  map[string]measurement `json:"soil_moisture,omitempty"`
	Warnings      map[string]int         `json:"warnings,omitempty"`
	Samples       []model.Snapshot       `json:"sample_snapshots,omitempty"`
}

type runningStats struct {
	count int
	sum   float64
	min   float64
	max   float64
	start float64
	end   float64
	set   bool
}

func (g *Generator) Generate(ctx context.Context, plan GeneratePlan) (PostResult, error) {
	snapshots, err := g.store.Load(plan.Day)
	if err != nil {
		return PostResult{}, err
	}
	snapshots = filterSnapshotsByWindow(snapshots, plan.WindowStart, plan.WindowEnd)
	if len(snapshots) == 0 {
		return PostResult{}, ErrNoSnapshots
	}
	return g.generateFromSnapshots(ctx, plan, snapshots, writeOptions{slugLabel: plan.SlugLabel})
}

func (g *Generator) GenerateDraft(ctx context.Context, day time.Time, snapshots []model.Snapshot) (PostResult, error) {
	if len(snapshots) == 0 {
		return PostResult{}, ErrNoSnapshots
	}
	plan := GeneratePlan{Day: day, Period: PeriodDaily, WindowStart: dayStart(day.UTC()), WindowEnd: dayStart(day.UTC()).Add(24 * time.Hour), PromptLabel: "full-day summary"}
	return g.generateFromSnapshots(ctx, plan, snapshots, writeOptions{draft: true})
}

func (g *Generator) GenerateRepoPost(ctx context.Context, day time.Time, prompt, titleHint string, draft bool, categories string) (PostResult, error) {
	markdown, err := g.generateWithRetry(ctx, repoPostSystemPrompt(), prompt, validateRepoPostMarkdown)
	if err != nil {
		return PostResult{}, err
	}

	title, body := splitMarkdown(markdown, day)
	if strings.TrimSpace(titleHint) != "" && title == fallbackTitle(day) {
		title = strings.TrimSpace(titleHint)
	}

	return g.writePost(day, title, body, writeOptions{draft: draft, categories: categories})
}

func (g *Generator) GeneratePrometheusPost(day time.Time, title, body string, draft bool, categories, layout string, assetPaths, publicChartPaths []string) (PostResult, error) {
	result, err := g.writePost(day, title, body, writeOptions{
		draft:      draft,
		categories: categories,
		layout:     layout,
		extras: map[string]any{
			"prometheus_charts":        true,
			"prometheus_chart_exports": publicChartPaths,
		},
	})
	if err != nil {
		return PostResult{}, err
	}
	result.AssetPaths = append(result.AssetPaths, assetPaths...)
	return result, nil
}

func (g *Generator) generateFromSnapshots(ctx context.Context, plan GeneratePlan, snapshots []model.Snapshot, opts writeOptions) (PostResult, error) {
	summaryPayload, err := buildSummary(plan, snapshots)
	if err != nil {
		return PostResult{}, err
	}

	prompt, err := g.buildPrompt(plan, summaryPayload)
	if err != nil {
		return PostResult{}, err
	}

	markdown, err := g.generateWithRetry(ctx, g.llmConfig.SystemPrompt, prompt, validateDailyPostMarkdown)
	if err != nil {
		return PostResult{}, err
	}
	refined, err := g.refinePost(ctx, markdown)
	if err != nil {
		log.Printf("refinement pass failed, using raw draft: %v", err)
		refined = markdown
	}

	title, body := splitMarkdown(refined, plan.Day)
	result, err := g.writePost(plan.Day, title, body, opts)
	if err != nil {
		return PostResult{}, err
	}

	if g.sensorWriter != nil {
		if dataPath, writeErr := g.sensorWriter.Write(snapshots); writeErr != nil {
			log.Printf("live_sensors update failed (non-fatal): %v", writeErr)
		} else if dataPath != "" {
			result.AssetPaths = append(result.AssetPaths, dataPath)
		}
	}

	return result, nil
}

func (g *Generator) generateWithRetry(ctx context.Context, systemPrompt, prompt string, validate func(string) error) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= maxGenerateAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		markdown, err := g.llm.GenerateMarkdownWithSystemPrompt(ctx, systemPrompt, prompt)
		if err != nil {
			lastErr = err
			log.Printf("generate attempt %d/%d failed: %v", attempt, maxGenerateAttempts, err)
			continue
		}
		if err := validate(markdown); err != nil {
			lastErr = err
			log.Printf("generate attempt %d/%d failed validation: %v", attempt, maxGenerateAttempts, err)
			continue
		}
		if attempt > 1 {
			log.Printf("generate succeeded on attempt %d/%d", attempt, maxGenerateAttempts)
		}
		return markdown, nil
	}
	if lastErr == nil {
		lastErr = errors.New("unknown generation failure")
	}
	return "", fmt.Errorf("all %d generate attempts failed: %w", maxGenerateAttempts, lastErr)
}

func refineSystemPrompt() string {
	return "You are a copy editor for Herb Hub 365 blog posts. Return ONLY cleaned markdown, nothing else. Keep the level-1 heading title, tighten it if wordy, and aim for 3 to 5 short paragraphs of prose after the title. Remove bullet lists unless they genuinely help readability. Remove any preamble, thinking traces, meta-commentary, and fenced code blocks. Do not invent data not present in the draft. Keep the tone warm, observational, and concise. Fix grammar and awkward phrasing."
}

func refineUserPrompt(draft string) string {
	return "Clean up this draft blog post:\n\n" + draft
}

func (g *Generator) refinePost(ctx context.Context, draft string) (string, error) {
	refined, err := g.llm.GenerateMarkdownWithSystemPrompt(ctx, refineSystemPrompt(), refineUserPrompt(draft))
	if err != nil {
		return "", err
	}
	refined = strings.TrimSpace(refined)
	if err := validateDailyPostMarkdown(refined); err != nil {
		return "", err
	}
	return refined, nil
}

func (g *Generator) buildPrompt(plan GeneratePlan, summaryPayload []byte) (string, error) {
	return fmt.Sprintf(
		"Write a %s blog post for %s (%s). The site URL is %s. Focus only on the data captured between %s and %s UTC. Use the JSON summary below as the only factual source. Mention noteworthy changes in moisture, water level, temperature, light, or warnings when present. Keep it readable for a public blog and do not use bullet lists unless clearly helpful.\n\n%s",
		plan.PromptLabel,
		g.llmConfig.PromptPlantName,
		g.llmConfig.PromptSiteName,
		g.llmConfig.PromptSiteURL,
		plan.WindowStart.Format(time.RFC3339),
		plan.WindowEnd.Format(time.RFC3339),
		string(summaryPayload),
	), nil
}

func (g *Generator) writePost(day time.Time, title, body string, opts writeOptions) (PostResult, error) {
	slug := slugify(title, g.blogConfig.SlugMaxWords)
	if slug == "" {
		slug = day.Format("2006-01-02")
	}

	baseDir := g.blogConfig.PostsDir
	fileName := fmt.Sprintf("%s-%s.markdown", day.Format("2006-01-02"), slug)
	if opts.slugLabel != "" {
		fileName = fmt.Sprintf("%s-%s-%s.markdown", day.Format("2006-01-02"), opts.slugLabel, slug)
	}
	if opts.draft {
		baseDir = g.blogConfig.DraftsDir
		prefix := strings.TrimSpace(g.blogConfig.DraftPrefix)
		if prefix != "" {
			fileName = prefix + "-" + fileName
		}
	}

	path := filepath.Join(baseDir, fileName)
	if !g.blogConfig.Overwrite {
		if _, err := os.Stat(path); err == nil {
			return PostResult{}, fmt.Errorf("post already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return PostResult{}, err
		}
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return PostResult{}, err
	}

	assetPaths := []string{}
	trimmedBody := strings.TrimSpace(body)
	if g.blogConfig.IncludeDayImage {
		imageMarkdown, assetPath, err := g.prepareDayImage(day, slug)
		if err != nil {
			return PostResult{}, err
		}
		if imageMarkdown != "" {
			trimmedBody = imageMarkdown + "\n\n" + trimmedBody
			assetPaths = append(assetPaths, assetPath)
		}
	}

	content := buildFrontMatter(g.blogConfig, day, title, opts.categories, opts.layout, opts.extras) + trimmedBody + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return PostResult{}, err
	}

	return PostResult{Path: path, AssetPaths: assetPaths}, nil
}

func (g *Generator) prepareDayImage(day time.Time, slug string) (string, string, error) {
	if strings.TrimSpace(g.blogConfig.ImageSourceDir) == "" {
		return "", "", nil
	}
	dayDir := filepath.Join(g.blogConfig.ImageSourceDir, day.Format("2006-01-02"))
	entries, err := os.ReadDir(dayDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", err
	}
	images := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
			images = append(images, filepath.Join(dayDir, entry.Name()))
		}
	}
	if len(images) == 0 {
		return "", "", nil
	}
	sort.Strings(images)
	selected := images[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(images))]
	if err := os.MkdirAll(g.blogConfig.ImageOutputDir, 0o755); err != nil {
		return "", "", err
	}
	ext := strings.ToLower(filepath.Ext(selected))
	assetName := fmt.Sprintf("%s-%s%s", day.Format("2006-01-02"), slug, ext)
	assetPath := filepath.Join(g.blogConfig.ImageOutputDir, assetName)
	if err := copyFile(selected, assetPath); err != nil {
		return "", "", err
	}
	publicPath := strings.TrimRight(g.blogConfig.ImagePublicPath, "/") + "/" + assetName
	alt := fmt.Sprintf("Timelapse image for %s", day.Format("January 2, 2006"))
	return fmt.Sprintf("![%s](%s)", alt, publicPath), assetPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func buildFrontMatter(cfg config.BlogConfig, day time.Time, title, categories, layout string, extras map[string]any) string {
	var builder strings.Builder
	publishedAt := time.Date(day.Year(), day.Month(), day.Day(), 20, 0, 0, 0, time.UTC)
	now := time.Now().UTC().Truncate(time.Second)
	if day.Year() == now.Year() && day.YearDay() == now.YearDay() && publishedAt.After(now) {
		publishedAt = now
	}
	if strings.TrimSpace(categories) == "" {
		categories = cfg.Categories
	}
	if strings.TrimSpace(layout) == "" {
		layout = cfg.Layout
	}

	builder.WriteString("---\n")
	builder.WriteString("layout: ")
	builder.WriteString(layout)
	builder.WriteString("\n")
	builder.WriteString("title: ")
	builder.WriteString(strconvQuote(title))
	builder.WriteString("\n")
	builder.WriteString("date: ")
	builder.WriteString(publishedAt.Format("2006-01-02 15:04:05 -0700"))
	builder.WriteString("\n")
	builder.WriteString("categories: ")
	builder.WriteString(categories)
	builder.WriteString("\n")
	if cfg.Author != "" {
		builder.WriteString("author: ")
		builder.WriteString(strconvQuote(cfg.Author))
		builder.WriteString("\n")
	}
	if len(extras) > 0 {
		extraYAML, err := yaml.Marshal(extras)
		if err == nil {
			builder.WriteString(string(extraYAML))
		}
	}
	builder.WriteString("---\n\n")
	return builder.String()
}

func buildSummary(plan GeneratePlan, snapshots []model.Snapshot) ([]byte, error) {
	summaryData := summary{
		Date:          plan.Day.Format("2006-01-02"),
		SnapshotCount: len(snapshots),
		Environment:   make(map[string]measurement),
		Water:         make(map[string]measurement),
		Temperatures:  make(map[string]measurement),
		SoilMoisture:  make(map[string]measurement),
		Warnings:      make(map[string]int),
	}

	devices := make(map[string]struct{})
	envStats := map[string]*runningStats{
		"temperature_c": newRunningStats(),
		"humidity_pct":  newRunningStats(),
		"pressure_hpa":  newRunningStats(),
		"light_lux":     newRunningStats(),
	}
	waterStats := map[string]*runningStats{
		"percent_full": newRunningStats(),
		"distance_cm":  newRunningStats(),
	}
	temperatureStats := make(map[string]*runningStats)
	soilStats := make(map[string]*runningStats)

	for _, snapshot := range snapshots {
		if summaryData.FirstSeen == "" || snapshot.Timestamp.Before(parseRFC3339(summaryData.FirstSeen)) {
			summaryData.FirstSeen = snapshot.Timestamp.UTC().Format(time.RFC3339)
		}
		if summaryData.LastSeen == "" || snapshot.Timestamp.After(parseRFC3339(summaryData.LastSeen)) {
			summaryData.LastSeen = snapshot.Timestamp.UTC().Format(time.RFC3339)
		}
		if snapshot.Device != "" {
			devices[snapshot.Device] = struct{}{}
		}

		envStats["temperature_c"].Add(snapshot.Environment.Temperature)
		envStats["humidity_pct"].Add(snapshot.Environment.Humidity)
		envStats["pressure_hpa"].Add(snapshot.Environment.Pressure)
		envStats["light_lux"].Add(snapshot.Environment.LightLux)

		waterStats["percent_full"].Add(snapshot.WaterReservoir.PercentFull)
		waterStats["distance_cm"].Add(snapshot.WaterReservoir.DistanceCM)

		for name, reading := range snapshot.Temperatures {
			stats := temperatureStats[name]
			if stats == nil {
				stats = newRunningStats()
				temperatureStats[name] = stats
			}
			stats.Add(reading)
		}

		for name, reading := range snapshot.SoilMoisture {
			stats := soilStats[name]
			if stats == nil {
				stats = newRunningStats()
				soilStats[name] = stats
			}
			stats.Add(reading.Percent)
		}

		for _, warning := range snapshot.Warnings {
			summaryData.Warnings[warning]++
		}
	}

	summaryData.Devices = sortedKeys(devices)
	for key, stats := range envStats {
		if stats.set {
			summaryData.Environment[key] = stats.Measurement()
		}
	}
	for key, stats := range waterStats {
		if stats.set {
			summaryData.Water[key] = stats.Measurement()
		}
	}
	for key, stats := range temperatureStats {
		if stats.set {
			summaryData.Temperatures[key] = stats.Measurement()
		}
	}
	for key, stats := range soilStats {
		if stats.set {
			summaryData.SoilMoisture[key] = stats.Measurement()
		}
	}

	summaryData.Samples = sampleSnapshots(snapshots)

	return json.MarshalIndent(summaryData, "", "  ")
}

func filterSnapshotsByWindow(snapshots []model.Snapshot, start, end time.Time) []model.Snapshot {
	filtered := make([]model.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		timestamp := snapshot.Timestamp.UTC()
		if timestamp.IsZero() && snapshot.CollectedAt != nil {
			timestamp = snapshot.CollectedAt.UTC()
		}
		if timestamp.IsZero() {
			continue
		}
		if !timestamp.Before(start) && timestamp.Before(end) {
			filtered = append(filtered, snapshot)
		}
	}
	return filtered
}

func sampleSnapshots(snapshots []model.Snapshot) []model.Snapshot {
	if len(snapshots) <= 6 {
		return snapshots
	}
	selected := make([]model.Snapshot, 0, 6)
	selected = append(selected, snapshots[:3]...)
	selected = append(selected, snapshots[len(snapshots)-3:]...)
	return selected
}

func splitMarkdown(markdown string, day time.Time) (string, string) {
	trimmed := strings.TrimSpace(markdown)
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 {
		return fallbackTitle(day), trimmed
	}

	first := strings.TrimSpace(lines[0])
	if strings.HasPrefix(first, "#") {
		title := strings.TrimSpace(strings.TrimLeft(first, "#"))
		body := strings.TrimSpace(strings.Join(lines[1:], "\n"))
		if title == "" {
			title = fallbackTitle(day)
		}
		if body == "" {
			body = "A new daily update was generated, but the model did not return any body content."
		}
		return title, body
	}

	return fallbackTitle(day), trimmed
}

func fallbackTitle(day time.Time) string {
	return fmt.Sprintf("Daily Herb Hub Update for %s", day.Format("January 2, 2006"))
}

func repoPostSystemPrompt() string {
	return "You are writing a public Herb Hub 365 technical blog post. Use only the supplied repository excerpts as factual sources. Do not reveal secrets, credentials, tokens, or private configuration details. Write clear markdown beginning with a level-1 heading, then explain what the component does, how it works, and why it matters. Do not critique the implementation, do not propose fixes, do not produce review notes, and do not include fenced code blocks."
}

func validateRepoPostMarkdown(markdown string) error {
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return errInvalidRepoPost
	}
	lowered := strings.ToLower(trimmed)
	invalidSnippets := []string{
		"## issues found",
		"corrected code",
		"code review",
		"critical bug",
		"key fixes",
		"issue | before | after",
		"| issue |",
		"```",
	}
	for _, snippet := range invalidSnippets {
		if strings.Contains(lowered, snippet) {
			return fmt.Errorf("%w: contains disallowed review-style output", errInvalidRepoPost)
		}
	}
	if !strings.HasPrefix(trimmed, "#") {
		return fmt.Errorf("%w: missing markdown title", errInvalidRepoPost)
	}
	return nil
}

func validateDailyPostMarkdown(markdown string) error {
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return fmt.Errorf("daily post output is empty")
	}
	if !strings.HasPrefix(trimmed, "#") {
		return fmt.Errorf("daily post is missing markdown title")
	}
	lowered := strings.ToLower(trimmed)
	badSnippets := []string{"thinking process:", "analyze the request:"}
	for _, snippet := range badSnippets {
		if strings.Contains(lowered, snippet) {
			return fmt.Errorf("daily post leaked model meta-commentary")
		}
	}
	nonEmptyLines := 0
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	if nonEmptyLines < 4 {
		return fmt.Errorf("daily post is too short")
	}
	return nil
}

func slugify(value string, maxWords int) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	words := strings.Fields(strings.ToLower(value))
	if maxWords > 0 && len(words) > maxWords {
		words = words[:maxWords]
	}
	slug := re.ReplaceAllString(strings.Join(words, "-"), "-")
	return strings.Trim(slug, "-")
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func newRunningStats() *runningStats {
	return &runningStats{min: math.MaxFloat64}
}

func (r *runningStats) Add(value *float64) {
	if value == nil {
		return
	}
	v := *value
	if !r.set {
		r.start = v
		r.min = v
		r.max = v
		r.set = true
	}
	r.end = v
	r.count++
	r.sum += v
	if v < r.min {
		r.min = v
	}
	if v > r.max {
		r.max = v
	}
}

func (r *runningStats) Measurement() measurement {
	return measurement{
		Count: r.count,
		Min:   round(r.min),
		Max:   round(r.max),
		Avg:   round(r.sum / float64(r.count)),
		Start: round(r.start),
		End:   round(r.end),
	}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func parseRFC3339(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
