package prompost

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"HerbHub365/services/blog-poster/internal/config"
)

type QueryFile struct {
	Queries []QueryDefinition `json:"queries"`
}

type QueryDefinition struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	PromQL    string `json:"promql"`
	ChartType string `json:"chart_type,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Range     string `json:"range,omitempty"`
	Step      string `json:"step,omitempty"`
}

type ExportResult struct {
	Title       string
	Body        string
	AssetPaths  []string
	PublicPaths []string
}

type chartExport struct {
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Query              string          `json:"query"`
	ChartType          string          `json:"chart_type,omitempty"`
	Unit               string          `json:"unit,omitempty"`
	GeneratedAt        string          `json:"generated_at"`
	Start              string          `json:"start"`
	End                string          `json:"end"`
	Step               string          `json:"step"`
	PrometheusResponse json.RawMessage `json:"prometheus_response"`
}

func Generate(day time.Time, cfg config.PromPostConfig, siteName string) (ExportResult, error) {
	queryFile, err := loadQueryFile(cfg.QueryFile)
	if err != nil {
		return ExportResult{}, err
	}
	if len(queryFile.Queries) == 0 {
		return ExportResult{}, fmt.Errorf("no queries found in %s", cfg.QueryFile)
	}

	day = day.UTC()
	rangeEnd := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.UTC)
	rangeStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)

	client := &http.Client{Timeout: cfg.Timeout}
	dateDir := day.Format("2006-01-02")
	assetPaths := make([]string, 0, len(queryFile.Queries))
	publicPaths := make([]string, 0, len(queryFile.Queries))

	for _, query := range queryFile.Queries {
		span := cfg.DefaultSpan
		if strings.TrimSpace(query.Range) != "" {
			parsed, parseErr := time.ParseDuration(strings.TrimSpace(query.Range))
			if parseErr != nil {
				return ExportResult{}, fmt.Errorf("invalid range %q for %s: %w", query.Range, query.ID, parseErr)
			}
			span = parsed
		}
		step := cfg.DefaultStep
		if strings.TrimSpace(query.Step) != "" {
			parsed, parseErr := time.ParseDuration(strings.TrimSpace(query.Step))
			if parseErr != nil {
				return ExportResult{}, fmt.Errorf("invalid step %q for %s: %w", query.Step, query.ID, parseErr)
			}
			step = parsed
		}

		start := rangeEnd.Add(-span)
		if start.Before(rangeStart) {
			start = rangeStart
		}

		rawResponse, fetchErr := fetchPrometheusRange(client, cfg, query.PromQL, start, rangeEnd, step)
		if fetchErr != nil {
			return ExportResult{}, fmt.Errorf("fetch %s failed: %w", query.ID, fetchErr)
		}

		slug := cleanSlug(query.ID)
		if slug == "" {
			slug = cleanSlug(query.Title)
		}
		if slug == "" {
			slug = "prometheus-chart"
		}

		export := chartExport{
			ID:                 query.ID,
			Title:              query.Title,
			Query:              query.PromQL,
			ChartType:          defaultValue(query.ChartType, "line"),
			Unit:               query.Unit,
			GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
			Start:              start.Format(time.RFC3339),
			End:                rangeEnd.Format(time.RFC3339),
			Step:               step.String(),
			PrometheusResponse: rawResponse,
		}

		exportJSON, marshalErr := json.MarshalIndent(export, "", "  ")
		if marshalErr != nil {
			return ExportResult{}, fmt.Errorf("marshal export for %s: %w", query.ID, marshalErr)
		}

		assetPath := filepath.Join(cfg.ExportDir, dateDir, slug+".json")
		if mkErr := os.MkdirAll(filepath.Dir(assetPath), 0o755); mkErr != nil {
			return ExportResult{}, fmt.Errorf("create export dir: %w", mkErr)
		}
		if writeErr := os.WriteFile(assetPath, exportJSON, 0o644); writeErr != nil {
			return ExportResult{}, fmt.Errorf("write export %s: %w", assetPath, writeErr)
		}

		assetPaths = append(assetPaths, assetPath)
		publicPaths = append(publicPaths, "/assets/data/prometheus/"+dateDir+"/"+slug+".json")
	}

	title := fmt.Sprintf("Prometheus Metrics Snapshot — %s", day.Format("January 2, 2006"))
	body := fmt.Sprintf(
		"This metrics post was generated automatically from Prometheus exports for %s.\n\nThe interactive charts below are rendered from static JSON snapshots committed with this post, keeping the Hub site fast and reliable while still showing detailed trends.\n\nData source: %s",
		day.Format("January 2, 2006"),
		siteName,
	)

	return ExportResult{Title: title, Body: body, AssetPaths: assetPaths, PublicPaths: publicPaths}, nil
}

func loadQueryFile(path string) (QueryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return QueryFile{}, fmt.Errorf("read query file %s: %w", path, err)
	}
	var file QueryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return QueryFile{}, fmt.Errorf("parse query file %s: %w", path, err)
	}
	for i := range file.Queries {
		file.Queries[i].ID = strings.TrimSpace(file.Queries[i].ID)
		file.Queries[i].Title = strings.TrimSpace(file.Queries[i].Title)
		file.Queries[i].PromQL = strings.TrimSpace(file.Queries[i].PromQL)
		if file.Queries[i].ID == "" {
			file.Queries[i].ID = cleanSlug(file.Queries[i].Title)
		}
		if file.Queries[i].Title == "" || file.Queries[i].PromQL == "" {
			return QueryFile{}, fmt.Errorf("each query must include title and promql")
		}
	}
	return file, nil
}

func fetchPrometheusRange(client *http.Client, cfg config.PromPostConfig, promql string, start, end time.Time, step time.Duration) (json.RawMessage, error) {
	base := strings.TrimRight(cfg.BaseURL, "/")
	path := cfg.QueryPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	values := url.Values{}
	values.Set("query", promql)
	values.Set("start", fmt.Sprintf("%d", start.Unix()))
	values.Set("end", fmt.Sprintf("%d", end.Unix()))
	values.Set("step", formatStepSeconds(step))

	endpoint := base + path + "?" + values.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("invalid prometheus json response: %w", err)
	}
	if status, _ := parsed["status"].(string); status != "success" {
		return nil, fmt.Errorf("prometheus returned non-success status: %s", status)
	}

	return json.RawMessage(body), nil
}

func formatStepSeconds(step time.Duration) string {
	if step <= 0 {
		return "300"
	}
	seconds := int(step.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	return fmt.Sprintf("%d", seconds)
}

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func cleanSlug(value string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	slug := slugRegex.ReplaceAllString(strings.Join(parts, "-"), "-")
	return strings.Trim(slug, "-")
}

func defaultValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
