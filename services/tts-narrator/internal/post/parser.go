// Package post provides helpers for reading Jekyll post files and
// extracting metadata from their YAML front matter.
package post

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Post represents a parsed Jekyll blog post.
type Post struct {
	// Path is the absolute path to the post file.
	Path string
	// Slug is the filename without the date prefix and extension,
	// e.g. "morning-sensor-report".
	Slug string
	// Date is the date parsed from the filename prefix.
	Date time.Time
	// AudioURL is the value of audio_url in the front matter, if present.
	AudioURL string
	// RawContent is the full file content (front matter + body).
	RawContent string
}

// postFileRe matches Jekyll post filenames: YYYY-MM-DD-slug.markdown / .md
var postFileRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)\.(markdown|md)$`)

// audioURLRe matches an existing audio_url front matter line.
var audioURLRe = regexp.MustCompile(`(?m)^audio_url:\s*\S+`)

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
	slug := m[2]

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read post %s: %w", path, err)
	}

	p := &Post{
		Path:       path,
		Slug:       slug,
		Date:       date.UTC(),
		RawContent: string(content),
	}

	// Extract audio_url if already present.
	if am := audioURLRe.FindString(p.RawContent); am != "" {
		parts := strings.SplitN(am, ":", 2)
		if len(parts) == 2 {
			p.AudioURL = strings.TrimSpace(parts[1])
		}
	}

	return p, nil
}

// FindPostsForDate returns all post files whose filename date matches day.
func FindPostsForDate(postsDir string, day time.Time) ([]*Post, error) {
	prefix := day.UTC().Format("2006-01-02")
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		return nil, fmt.Errorf("read posts dir %s: %w", postsDir, err)
	}

	var posts []*Post
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !postFileRe.MatchString(name) {
			continue
		}
		p, err := LoadPost(filepath.Join(postsDir, name))
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// FindAllPosts returns all valid Jekyll post files in postsDir.
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
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// AudioFilename returns the expected MP3 filename for a post,
// e.g. "2026-04-06-morning-sensor-report.mp3".
func (p *Post) AudioFilename() string {
	return p.Date.Format("2006-01-02") + "-" + p.Slug + ".mp3"
}
