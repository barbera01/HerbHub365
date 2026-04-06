package preprocess

import (
	"regexp"
	"strings"
)

// Jekyll Liquid tag patterns to strip before TTS.
var (
	liquidBlockRe = regexp.MustCompile(`(?s)\{%-?\s*.*?-?%\}`)   // {% ... %}
	liquidVarRe   = regexp.MustCompile(`(?s)\{\{-?\s*.*?-?\}\}`) // {{ ... }}
	frontMatterRe = regexp.MustCompile(`(?s)^---\n.*?\n---\n?`)  // YAML front matter
)

// Process strips Jekyll artefacts then applies the full TTS rule pipeline.
// It accepts raw post file content (including front matter) and returns
// clean, spoken-word text ready for the TTS API.
func Process(rawContent string, rs *RuleSet) string {
	// 1. Strip YAML front matter.
	text := frontMatterRe.ReplaceAllString(rawContent, "")

	// 2. Strip Jekyll Liquid tags/variables.
	text = liquidBlockRe.ReplaceAllString(text, "")
	text = liquidVarRe.ReplaceAllString(text, "")

	// 3. Apply TTS preprocessing pipeline.
	text = rs.Apply(text)

	// 4. Collapse any run of blank lines left after stripping.
	reBlank := regexp.MustCompile(`\n{3,}`)
	text = reBlank.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}
