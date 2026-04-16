package repopost

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"HerbHub365/services/blog-poster/internal/config"
)

type Source struct {
	Path    string
	Content string
}

type Result struct {
	Prompt      string
	SourcePaths []string
}

var blockedDirNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	"tmp":          {},
	"temp":         {},
	"coverage":     {},
}

var blockedFilePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|/)\.env($|\.)`),
	regexp.MustCompile(`(?i)(^|/).*\.env$`),
	regexp.MustCompile(`(?i)(^|/).*(secret|secrets|credential|credentials|token|pat|private)[^/]*$`),
	regexp.MustCompile(`(?i)(^|/).+\.(pem|key|p12|crt|cer|tfvars)$`),
}

var sensitiveValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^(\s*[A-Z0-9_]*(SECRET|TOKEN|PASSWORD|API_KEY|PAT)[A-Z0-9_]*\s*[=:]\s*).+$`),
	regexp.MustCompile(`(?i)gh[pousr]_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)github_pat_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`),
	regexp.MustCompile(`(?i)(x-access-token:)[^\s"']+`),
}

func BuildPrompt(cfg config.Config) (Result, error) {
	if strings.TrimSpace(cfg.RepoPost.Prompt) == "" {
		return Result{}, fmt.Errorf("BLOG_REPO_POST_PROMPT is required for repo-post mode")
	}
	if len(cfg.RepoPost.Paths) == 0 {
		return Result{}, fmt.Errorf("BLOG_REPO_POST_PATHS is required for repo-post mode")
	}

	repoDir, err := filepath.Abs(cfg.Git.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve repo dir: %w", err)
	}

	sources, err := collectSources(repoDir, cfg.RepoPost)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Prompt:      formatPrompt(cfg, sources),
		SourcePaths: sourcePaths(sources),
	}, nil
}

func collectSources(repoDir string, cfg config.RepoPostConfig) ([]Source, error) {
	seen := make(map[string]struct{})
	sources := make([]Source, 0, cfg.MaxFiles)
	totalBytes := 0

	for _, requested := range cfg.Paths {
		absolutePath, relativePath, err := resolveRepoPath(repoDir, requested)
		if err != nil {
			return nil, err
		}
		if isBlockedRelativePath(relativePath) {
			return nil, fmt.Errorf("requested path is blocked for safety: %s", requested)
		}

		info, err := os.Stat(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", requested, err)
		}

		if info.IsDir() {
			err = filepath.WalkDir(absolutePath, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				rel, err := filepath.Rel(repoDir, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if d.IsDir() {
					if rel != "." {
						if _, blocked := blockedDirNames[filepath.Base(rel)]; blocked {
							return filepath.SkipDir
						}
					}
					return nil
				}
				if len(sources) >= cfg.MaxFiles || totalBytes >= cfg.MaxTotalBytes {
					return fs.SkipAll
				}
				if !isAllowedFile(rel) || isBlockedRelativePath(rel) {
					return nil
				}
				if _, ok := seen[rel]; ok {
					return nil
				}
				source, err := readSource(path, rel, cfg.MaxFileBytes, cfg.MaxTotalBytes-totalBytes)
				if err != nil {
					return nil
				}
				totalBytes += len(source.Content)
				sources = append(sources, source)
				seen[rel] = struct{}{}
				return nil
			})
			if err != nil && err != fs.SkipAll {
				return nil, fmt.Errorf("walk %s: %w", requested, err)
			}
			if len(sources) >= cfg.MaxFiles || totalBytes >= cfg.MaxTotalBytes {
				break
			}
			continue
		}

		if !isAllowedFile(relativePath) {
			return nil, fmt.Errorf("requested file type is not allowed: %s", requested)
		}
		if _, ok := seen[relativePath]; ok {
			continue
		}
		source, err := readSource(absolutePath, relativePath, cfg.MaxFileBytes, cfg.MaxTotalBytes-totalBytes)
		if err != nil {
			return nil, err
		}
		totalBytes += len(source.Content)
		sources = append(sources, source)
		seen[relativePath] = struct{}{}
		if len(sources) >= cfg.MaxFiles || totalBytes >= cfg.MaxTotalBytes {
			break
		}
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no safe source files found for BLOG_REPO_POST_PATHS")
	}

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Path < sources[j].Path
	})

	return sources, nil
}

func resolveRepoPath(repoDir, requested string) (string, string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(requested))
	if cleaned == "." || cleaned == "" {
		return "", "", fmt.Errorf("invalid repo path: %q", requested)
	}
	abs := cleaned
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoDir, cleaned)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(repoDir, abs)
	if err != nil {
		return "", "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", "", fmt.Errorf("path escapes repo root: %s", requested)
	}
	return abs, rel, nil
}

func isBlockedRelativePath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	for _, part := range strings.Split(path, "/") {
		if _, blocked := blockedDirNames[part]; blocked {
			return true
		}
	}
	for _, pattern := range blockedFilePatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

func isAllowedFile(path string) bool {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".sh", ".md", ".markdown", ".yml", ".yaml", ".json", ".toml", ".conf", ".txt", ".py", ".rb", ".html", ".css", ".js", ".ts", ".scss":
		return true
	}
	return strings.EqualFold(base, "dockerfile") || strings.EqualFold(base, "makefile")
}

func readSource(path, relativePath string, maxFileBytes, remainingBytes int) (Source, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Source{}, err
	}
	if info.Size() == 0 {
		return Source{}, fmt.Errorf("empty file")
	}
	limit := maxFileBytes
	if remainingBytes < limit {
		limit = remainingBytes
	}
	if limit <= 0 {
		return Source{}, fmt.Errorf("prompt budget exhausted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, err
	}
	content := string(data)
	if len(content) > limit {
		content = content[:limit] + "\n\n[truncated]"
	}
	content = redactSensitive(content)
	return Source{Path: relativePath, Content: content}, nil
}

func redactSensitive(content string) string {
	redacted := content
	for _, pattern := range sensitiveValuePatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.Contains(match, ":") {
				parts := strings.SplitN(match, ":", 2)
				return parts[0] + ":[REDACTED]"
			}
			if strings.Contains(match, "=") {
				parts := strings.SplitN(match, "=", 2)
				return parts[0] + "=[REDACTED]"
			}
			if strings.HasPrefix(strings.ToLower(match), "bearer ") {
				return "Bearer [REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return redacted
}

func formatPrompt(cfg config.Config, sources []Source) string {
	var builder strings.Builder

	// Source files first — models weight recent instructions more heavily,
	// so the article instruction appears last where it has the most influence.
	builder.WriteString("=== REFERENCE MATERIAL — FOR BACKGROUND ONLY ===\n")
	builder.WriteString("The following files are provided as factual background. Do not quote, reproduce, or analyse them. Use them only to understand what the feature does.\n\n")
	for _, source := range sources {
		builder.WriteString("--- ")
		builder.WriteString(source.Path)
		builder.WriteString(" ---\n")
		builder.WriteString(source.Content)
		builder.WriteString("\n\n")
	}

	builder.WriteString("=== YOUR TASK ===\n")
	if title := strings.TrimSpace(cfg.RepoPost.Title); title != "" {
		builder.WriteString("Title: ")
		builder.WriteString(title)
		builder.WriteString("\n\n")
	}
	builder.WriteString(cfg.RepoPost.Prompt)
	builder.WriteString("\n\n")
	builder.WriteString("Remember: write a flowing, narrative blog article in markdown starting with a level-1 heading. No code blocks, no inline code, no bullet-point lists of technical details, no code review, no implementation criticism.\n")
	return builder.String()
}

func sourcePaths(sources []Source) []string {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		paths = append(paths, source.Path)
	}
	return paths
}
