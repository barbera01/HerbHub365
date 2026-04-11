// Package post provides helpers for finding and parsing Jekyll post files.
package post

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Post represents a parsed Jekyll blog post.
type Post struct {
	// Path is the absolute path to the post file.
	Path string `json:"path"`
	// Slug is the filename without the date prefix and extension.
	Slug string `json:"slug"`
	// Date is the date parsed from the filename prefix.
	Date time.Time `json:"date"`
	// Title is extracted from YAML front matter, or derived from the slug.
	Title string `json:"title"`
	// Excerpt is the first ~200 characters of the body (plain text).
	Excerpt string `json:"excerpt"`
	// Filename is the base filename.
	Filename string `json:"filename"`
	// RawContent is the full file content (front matter + body).
	RawContent string `json:"-"`
}

// postFileRe matches Jekyll post filenames: YYYY-MM-DD-slug.markdown / .md
var postFileRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)\.(markdown|md)$`)

// frontMatterTitleRe extracts the title from YAML front matter.
var frontMatterTitleRe = regexp.MustCompile(`(?m)^title:\s*["']?(.+?)["']?\s*$`)

// frontMatterRe matches the YAML front matter block.
var frontMatterRe = regexp.MustCompile(`(?s)^---\n.*?\n---\n?`)

// LoadPost reads a Jekyll post file from disk and returns a Post.
func LoadPost(path string) (*Post, error) {
	base := filepath.Base(path)
	m := postFileRe.FindStringSubmatch(base)
	if m == nil {
		return nil, fmt.Errorf("filename %q does not match YYYY-MM-DD-slug.{markdown,md}", base)
	}
	date, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return nil, fmt.Errorf("parse date from %q: %w", base, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read post %s: %w", path, err)
	}

	raw := string(content)
	title := extractTitle(raw, m[2])
	excerpt := extractExcerpt(raw)

	return &Post{
		Path:       path,
		Slug:       m[2],
		Date:       date.UTC(),
		Title:      title,
		Excerpt:    excerpt,
		Filename:   base,
		RawContent: raw,
	}, nil
}

// FindAllPosts returns all valid Jekyll post files in postsDir, sorted newest first.
func FindAllPosts(postsDir string) ([]*Post, error) {
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		return nil, fmt.Errorf("read posts dir %s: %w", postsDir, err)
	}
	var posts []*Post
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !postFileRe.MatchString(e.Name()) {
			continue
		}
		p, err := LoadPost(filepath.Join(postsDir, e.Name()))
		if err != nil {
			continue // skip malformed posts
		}
		posts = append(posts, p)
	}
	// Sort newest first.
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Date.After(posts[j].Date)
	})
	return posts, nil
}

// VideoFilename returns the expected MP4 output filename for a post.
func (p *Post) VideoFilename() string {
	return p.Date.Format("2006-01-02") + "-" + p.Slug + ".mp4"
}

// HasVideo checks whether a final MP4 already exists in outputDir.
func (p *Post) HasVideo(outputDir string) bool {
	path := filepath.Join(outputDir, p.VideoFilename())
	_, err := os.Stat(path)
	return err == nil
}

// extractTitle pulls title from YAML front matter, or slugifies the slug.
func extractTitle(raw, slug string) string {
	if m := frontMatterTitleRe.FindStringSubmatch(raw); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// Convert slug to title case: morning-sensor-report → Morning Sensor Report
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// extractExcerpt returns the first ~200 characters of the body after front matter.
func extractExcerpt(raw string) string {
	body := frontMatterRe.ReplaceAllString(raw, "")
	body = strings.TrimSpace(body)
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return body
}
