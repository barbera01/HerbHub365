package blog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"HerbHub365/services/blog-poster/internal/config"
)

type Period string

const (
	PeriodDaily Period = "daily"
	PeriodAM    Period = "am"
	PeriodPM    Period = "pm"
)

type GeneratePlan struct {
	Day         time.Time
	Period      Period
	WindowStart time.Time
	WindowEnd   time.Time
	PromptLabel string
	SlugLabel   string
}

func ResolveGeneratePlan(cfg config.Config, now time.Time) (GeneratePlan, error) {
	targetDate, err := cfg.ResolveTargetDate(now)
	if err != nil {
		return GeneratePlan{}, err
	}

	mode := strings.TrimSpace(strings.ToLower(cfg.Generation.PeriodMode))
	if mode == "" {
		mode = "auto"
	}
	if cfg.Generation.SplitHour < 1 || cfg.Generation.SplitHour > 23 {
		return GeneratePlan{}, fmt.Errorf("invalid BLOG_GENERATE_SPLIT_HOUR %d: use 1-23", cfg.Generation.SplitHour)
	}

	today := dayStart(now.UTC())
	if targetDate.Before(today) && mode == "auto" {
		mode = "daily"
	}

	switch mode {
	case "daily":
		return newGeneratePlan(targetDate, PeriodDaily, cfg.Generation.SplitHour), nil
	case "am":
		return newGeneratePlan(targetDate, PeriodAM, cfg.Generation.SplitHour), nil
	case "pm":
		return newGeneratePlan(targetDate, PeriodPM, cfg.Generation.SplitHour), nil
	case "auto":
		if targetDate.Before(today) || targetDate.After(today) {
			return newGeneratePlan(targetDate, PeriodDaily, cfg.Generation.SplitHour), nil
		}
		if now.UTC().Hour() < cfg.Generation.SplitHour {
			return newGeneratePlan(targetDate, PeriodAM, cfg.Generation.SplitHour), nil
		}
		hasPost, err := HasPublishedPost(cfg.Blog.PostsDir, targetDate)
		if err != nil {
			return GeneratePlan{}, err
		}
		if hasPost {
			return newGeneratePlan(targetDate, PeriodPM, cfg.Generation.SplitHour), nil
		}
		return newGeneratePlan(targetDate, PeriodDaily, cfg.Generation.SplitHour), nil
	default:
		return GeneratePlan{}, fmt.Errorf("invalid BLOG_GENERATE_PERIOD_MODE %q: use auto, daily, am, or pm", cfg.Generation.PeriodMode)
	}
}

func HasPublishedPost(postsDir string, day time.Time) (bool, error) {
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read posts dir: %w", err)
	}

	prefix := day.UTC().Format("2006-01-02") + "-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".markdown" || ext == ".md" {
			return true, nil
		}
	}

	return false, nil
}

func newGeneratePlan(day time.Time, period Period, splitHour int) GeneratePlan {
	windowStart := dayStart(day.UTC())
	windowEnd := windowStart.Add(24 * time.Hour)
	promptLabel := "full-day summary"
	slugLabel := ""

	switch period {
	case PeriodAM:
		windowEnd = windowStart.Add(time.Duration(splitHour) * time.Hour)
		promptLabel = "morning update"
		slugLabel = "am"
	case PeriodPM:
		windowStart = windowStart.Add(time.Duration(splitHour) * time.Hour)
		promptLabel = "afternoon update"
		slugLabel = "pm"
	}

	return GeneratePlan{
		Day:         day.UTC(),
		Period:      period,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		PromptLabel: promptLabel,
		SlugLabel:   slugLabel,
	}
}

func dayStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}
