package post

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// frontMatterEndRe matches the closing --- of Jekyll front matter.
var frontMatterEndRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---`)

// PatchAudioURL injects or replaces the audio_url key in the post's YAML
// front matter, then writes the result back to disk.
// It returns the updated file content.
func PatchAudioURL(p *Post, audioURL string) (string, error) {
	updated := setFrontMatterKey(p.RawContent, "audio_url", audioURL)
	if err := os.WriteFile(p.Path, []byte(updated), 0644); err != nil {
		return "", fmt.Errorf("write patched post %s: %w", p.Path, err)
	}
	return updated, nil
}

// setFrontMatterKey sets key: value inside the YAML front matter block.
// If the key already exists its value is replaced; otherwise it is appended
// before the closing ---.
func setFrontMatterKey(content, key, value string) string {
	// Ensure value doesn't need quoting; for simple URLs it never does.
	line := key + ": " + value

	// Replace existing key.
	existingRe := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s*.*$`)
	if existingRe.MatchString(content) {
		return existingRe.ReplaceAllString(content, line)
	}

	// Inject before the closing ---.
	return frontMatterEndRe.ReplaceAllStringFunc(content, func(block string) string {
		// block is everything from the first --- up to and including the second ---.
		// Insert the new line before the final ---.
		idx := strings.LastIndex(block, "\n---")
		if idx < 0 {
			return block + "\n" + line
		}
		return block[:idx] + "\n" + line + block[idx:]
	})
}
