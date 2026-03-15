package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"HerbHub365/services/blog-poster/internal/archive"
	"HerbHub365/services/blog-poster/internal/config"
	"HerbHub365/services/blog-poster/internal/model"
)

var ErrNoSnapshots = errors.New("no snapshots found for target date")

type markdownGenerator interface {
	GenerateMarkdown(ctx context.Context, prompt string) (string, error)
}

type Generator struct {
	blogConfig config.BlogConfig
	llmConfig  config.LLMConfig
	store      *archive.Store
	llm        markdownGenerator
}

type writeOptions struct {
	draft bool
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

func NewGenerator(blogCfg config.BlogConfig, llmCfg config.LLMConfig, store *archive.Store, llm markdownGenerator) *Generator {
	return &Generator{blogConfig: blogCfg, llmConfig: llmCfg, store: store, llm: llm}
}

func (g *Generator) Generate(ctx context.Context, day time.Time) (string, error) {
	snapshots, err := g.store.Load(day)
	if err != nil {
		return "", err
	}
	if len(snapshots) == 0 {
		return "", ErrNoSnapshots
	}
	return g.generateFromSnapshots(ctx, day, snapshots, writeOptions{})
}

func (g *Generator) GenerateDraft(ctx context.Context, day time.Time, snapshots []model.Snapshot) (string, error) {
	if len(snapshots) == 0 {
		return "", ErrNoSnapshots
	}
	return g.generateFromSnapshots(ctx, day, snapshots, writeOptions{draft: true})
}

func (g *Generator) generateFromSnapshots(ctx context.Context, day time.Time, snapshots []model.Snapshot, opts writeOptions) (string, error) {
	summaryPayload, err := buildSummary(day, snapshots)
	if err != nil {
		return "", err
	}

	prompt, err := g.buildPrompt(summaryPayload)
	if err != nil {
		return "", err
	}

	markdown, err := g.llm.GenerateMarkdown(ctx, prompt)
	if err != nil {
		return "", err
	}

	title, body := splitMarkdown(markdown, day)
	return g.writePost(day, title, body, opts)
}

func (g *Generator) buildPrompt(summaryPayload []byte) (string, error) {
	return fmt.Sprintf(
		"Write today's blog post for %s (%s). The site URL is %s.\n\nUse the JSON summary below as the only factual source. Mention noteworthy changes in moisture, water level, temperature, light, or warnings when present. Keep it readable for a public blog and do not use bullet lists unless clearly helpful.\n\n%s",
		g.llmConfig.PromptPlantName,
		g.llmConfig.PromptSiteName,
		g.llmConfig.PromptSiteURL,
		string(summaryPayload),
	), nil
}

func (g *Generator) writePost(day time.Time, title, body string, opts writeOptions) (string, error) {
	slug := slugify(title, g.blogConfig.SlugMaxWords)
	if slug == "" {
		slug = day.Format("2006-01-02")
	}

	baseDir := g.blogConfig.PostsDir
	fileName := fmt.Sprintf("%s-%s.markdown", day.Format("2006-01-02"), slug)
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
			return "", fmt.Errorf("post already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}

	content := buildFrontMatter(g.blogConfig, day, title) + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}

	return path, nil
}

func buildFrontMatter(cfg config.BlogConfig, day time.Time, title string) string {
	var builder strings.Builder
	publishedAt := time.Date(day.Year(), day.Month(), day.Day(), 20, 0, 0, 0, time.UTC)
	now := time.Now().UTC().Truncate(time.Second)
	if day.Year() == now.Year() && day.YearDay() == now.YearDay() && publishedAt.After(now) {
		publishedAt = now
	}

	builder.WriteString("---\n")
	builder.WriteString("layout: ")
	builder.WriteString(cfg.Layout)
	builder.WriteString("\n")
	builder.WriteString("title: ")
	builder.WriteString(strconvQuote(title))
	builder.WriteString("\n")
	builder.WriteString("date: ")
	builder.WriteString(publishedAt.Format("2006-01-02 15:04:05 -0700"))
	builder.WriteString("\n")
	builder.WriteString("categories: ")
	builder.WriteString(cfg.Categories)
	builder.WriteString("\n")
	if cfg.Author != "" {
		builder.WriteString("author: ")
		builder.WriteString(strconvQuote(cfg.Author))
		builder.WriteString("\n")
	}
	builder.WriteString("---\n\n")
	return builder.String()
}

func buildSummary(day time.Time, snapshots []model.Snapshot) ([]byte, error) {
	summaryData := summary{
		Date:          day.Format("2006-01-02"),
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
