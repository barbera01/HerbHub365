// Package preprocess ports tts-preprocess.py to Go.
// It loads tts-rules.json and applies the full TTS preprocessing pipeline.
package preprocess

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ─── JSON schema types ────────────────────────────────────────────────────────

type rulesFile struct {
	WindDirections map[string]string `json:"wind_directions"`
	Rules          []json.RawMessage `json:"rules"`
}

// ruleEntry is the parsed form of one item inside "rules".
type ruleEntry struct {
	Group       string    `json:"_group"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Pattern     string    `json:"pattern"`
	Replacement string    `json:"replacement"`
	Flags       *[]string `json:"flags"` // nil = missing (use IGNORECASE), non-nil empty = no flags
	Enabled     *bool     `json:"enabled"`
}

// ─── Compiled rule ────────────────────────────────────────────────────────────

// handlerFunc is the signature for all special-case handlers.
// It takes the text, the wind-direction table, and returns the modified text.
type handlerFunc func(text string, windDirs map[string]string) string

// compiledRule holds either a regex+replacement pair or a special handler.
type compiledRule struct {
	name        string
	pattern     *regexp.Regexp // nil for special rules
	replacement string
	handler     handlerFunc // non-nil for special rules
}

// ─── Rule-set ─────────────────────────────────────────────────────────────────

// RuleSet is the loaded and compiled preprocessing pipeline.
type RuleSet struct {
	rules    []compiledRule
	windDirs map[string]string
}

// LoadRules reads and compiles the rules JSON from path.
func LoadRules(path string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules file: %w", err)
	}

	var rf rulesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse rules file: %w", err)
	}

	rs := &RuleSet{windDirs: rf.WindDirections}

	for _, raw := range rf.Rules {
		var entry ruleEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("parse rule entry: %w", err)
		}

		// Group separator — skip.
		if entry.Group != "" {
			continue
		}
		if entry.Name == "" {
			continue
		}

		// Default enabled=true
		if entry.Enabled != nil && !*entry.Enabled {
			continue
		}

		entryType := entry.Type
		if entryType == "" {
			entryType = "regex"
		}

		if entryType == "special" {
			h := specialHandler(entry.Name)
			if h == nil {
				fmt.Fprintf(os.Stderr, "[tts-narrator] WARNING: unknown special handler %q — skipping\n", entry.Name)
				continue
			}
			rs.rules = append(rs.rules, compiledRule{name: entry.Name, handler: h})
			continue
		}

		// Regex rule: build flag prefix.
		// Missing flags key → (?i) (case-insensitive).
		// Present empty slice → no prefix (case-sensitive).
		// Present non-empty → build from list.
		var prefix string
		if entry.Flags == nil {
			prefix = "(?i)"
		} else {
			prefix = buildFlagPrefix(*entry.Flags)
		}

		patternStr := prefix + entry.Pattern

		// Convert Python backreferences \1 → ${1} for Go regexp.
		replacement := pythonToGoReplacement(entry.Replacement)

		compiled, err := regexp.Compile(patternStr)
		if err != nil {
			return nil, fmt.Errorf("compile rule %q (%s): %w", entry.Name, patternStr, err)
		}
		rs.rules = append(rs.rules, compiledRule{
			name:        entry.Name,
			pattern:     compiled,
			replacement: replacement,
		})
	}

	return rs, nil
}

// buildFlagPrefix converts a JSON flag list to a Go regexp flag prefix string.
func buildFlagPrefix(flags []string) string {
	var sb strings.Builder
	sb.WriteString("(?")
	count := 0
	for _, f := range flags {
		switch strings.ToUpper(f) {
		case "IGNORECASE":
			sb.WriteByte('i')
			count++
		case "MULTILINE":
			sb.WriteByte('m')
			count++
		case "DOTALL":
			sb.WriteByte('s')
			count++
		}
	}
	if count == 0 {
		return ""
	}
	sb.WriteByte(')')
	return sb.String()
}

// pythonToGoReplacement converts Python RE replacement syntax to Go's.
// \1 → ${1}, \2 → ${2}, etc.
var pyBackref = regexp.MustCompile(`\\([1-9][0-9]?)`)

func pythonToGoReplacement(r string) string {
	return pyBackref.ReplaceAllString(r, `${${1}}`)
}

// ─── Apply ────────────────────────────────────────────────────────────────────

// Apply runs the full preprocessing pipeline and returns the processed text.
func (rs *RuleSet) Apply(text string) string {
	for _, rule := range rs.rules {
		if rule.handler != nil {
			text = rule.handler(text, rs.windDirs)
		} else {
			text = rule.pattern.ReplaceAllString(text, rule.replacement)
		}
	}

	// Final whitespace normalisation.
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	text = strings.Join(lines, "\n")
	// Collapse 3+ blank lines to 2.
	reBlankLines := regexp.MustCompile(`\n{3,}`)
	text = reBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// ─── Special handlers ─────────────────────────────────────────────────────────

func specialHandler(name string) handlerFunc {
	switch name {
	case "html-entities":
		return handlerHTMLEntities
	case "wind-directions":
		return handlerWindDirections
	case "md-heading":
		return handlerMDHeadings
	case "md-list":
		return handlerMDList
	case "md-ordered-list":
		return handlerMDOrderedList
	case "currency-gbp":
		return makeCurrencyHandler("currency-gbp", `£(\d+)\.(\d{2})\b`, "pound", "pounds", "pence")
	case "currency-usd":
		return makeCurrencyHandler("currency-usd", `\$(\d+)\.(\d{2})\b`, "dollar", "dollars", "cents")
	case "currency-eur":
		return makeCurrencyHandler("currency-eur", `€(\d+)\.(\d{2})\b`, "euro", "euros", "cents")
	case "time-24h":
		return handlerTime24H
	case "date-dd-mm-yyyy":
		return handlerDateDMY
	case "ordinals":
		return handlerOrdinals
	case "md-bold-italic":
		return handlerMDBoldItalic
	case "md-italic":
		return handlerMDItalic
	case "uv-index":
		return handlerUVIndex
	case "dist-metres":
		return makeUnitHandler(`(\d+(?:\.\d+)?)\s*m(\W|$)`, "$1 metres$2")
	case "weight-tonnes":
		return makeUnitHandler(`(\d+(?:\.\d+)?)\s*t(\W|$)`, "$1 tonnes$2")
	case "weight-grams":
		return makeUnitHandler(`(\d+(?:\.\d+)?)\s*g(\W|$)`, "$1 grams$2")
	case "vol-litres":
		return makeUnitHandler(`(\d+(?:\.\d+)?)\s*[lL](\W|$)`, "$1 litres$2")
	case "abbr-vs":
		return makeAbbrHandler(`\bvs?\.?(\W|$)`, "versus$1")
	case "abbr-approx":
		return makeAbbrHandler(`\bapprox\.?(\W|$)`, "approximately$1")
	case "abbr-max":
		return makeAbbrHandler(`\bmax\.?(\W|$)`, "maximum$1")
	case "abbr-min":
		return makeAbbrHandler(`\bmin\.?(\W|$)`, "minimum$1")
	case "abbr-avg":
		return makeAbbrHandler(`\bavg\.?(\W|$)`, "average$1")
	case "abbr-est":
		return makeAbbrHandler(`\best\.?(\W|$)`, "estimated$1")
	case "abbr-ca":
		return makeAbbrHandler(`\bca\.?(\W|$)`, "approximately$1")
	case "symbol-plus":
		return handlerSymbolPlus
	case "symbol-equals":
		return handlerSymbolEquals
	case "symbol-ampersand":
		return handlerSymbolAmpersand
	}
	return nil
}

// ─── html-entities ────────────────────────────────────────────────────────────

var htmlEntities = map[string]string{
	"&amp;":    "&",
	"&lt;":     "<",
	"&gt;":     ">",
	"&quot;":   `"`,
	"&#39;":    "'",
	"&apos;":   "'",
	"&nbsp;":   " ",
	"&mdash;":  "—",
	"&ndash;":  "–",
	"&hellip;": "…",
	"&laquo;":  "«",
	"&raquo;":  "»",
	"&copy;":   "©",
	"&reg;":    "®",
	"&trade;":  "™",
}

var htmlEntityRe = regexp.MustCompile(`&(?:#\d+|#x[0-9a-fA-F]+|[a-zA-Z]+);`)

func handlerHTMLEntities(text string, _ map[string]string) string {
	return htmlEntityRe.ReplaceAllStringFunc(text, func(entity string) string {
		if v, ok := htmlEntities[entity]; ok {
			return v
		}
		// Numeric character references: &#NN; or &#xHH;
		lower := strings.ToLower(entity)
		if strings.HasPrefix(lower, "&#x") {
			hexStr := lower[3 : len(lower)-1]
			var n int
			fmt.Sscanf(hexStr, "%x", &n)
			if n > 0 {
				return string(rune(n))
			}
		} else if strings.HasPrefix(lower, "&#") {
			numStr := lower[2 : len(lower)-1]
			var n int
			fmt.Sscanf(numStr, "%d", &n)
			if n > 0 {
				return string(rune(n))
			}
		}
		return entity
	})
}

// ─── wind-directions ──────────────────────────────────────────────────────────

func handlerWindDirections(text string, windDirs map[string]string) string {
	// Sort longest abbreviation first to avoid partial matches (e.g. NNE before NE).
	abbrs := make([]string, 0, len(windDirs))
	for k := range windDirs {
		abbrs = append(abbrs, k)
	}
	sort.Slice(abbrs, func(i, j int) bool { return len(abbrs[i]) > len(abbrs[j]) })

	for _, abbr := range abbrs {
		expansion := windDirs[abbr]
		// Use \b word boundaries (Go RE2 supports these).
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(abbr) + `\b`)
		text = re.ReplaceAllString(text, expansion)
	}
	return text
}

// ─── md-heading ───────────────────────────────────────────────────────────────

var mdHeadingRe = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)

func handlerMDHeadings(text string, _ map[string]string) string {
	return mdHeadingRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := mdHeadingRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		level := len(parts[1])
		content := strings.TrimSpace(parts[2])
		switch level {
		case 1:
			return content + ".\n"
		case 2:
			return "\n— " + content + " —\n"
		default:
			return content + " — "
		}
	})
}

// ─── md-list ──────────────────────────────────────────────────────────────────

var (
	mdListBlockRe = regexp.MustCompile(`(?m)(?:^[ \t]*[•·‣▸▪+\-\*]\s+[^\n]+\n?)+`)
	mdListItemRe  = regexp.MustCompile(`(?m)^[ \t]*[•·‣▸▪+\-\*]\s+([^\n]+)`)
)

func handlerMDList(text string, _ map[string]string) string {
	return mdListBlockRe.ReplaceAllStringFunc(text, func(block string) string {
		matches := mdListItemRe.FindAllStringSubmatch(block, -1)
		if len(matches) == 0 {
			return block
		}
		items := make([]string, len(matches))
		for i, m := range matches {
			items[i] = ensurePunct(m[1])
		}
		switch len(items) {
		case 1:
			return items[0] + "\n"
		case 2:
			return items[0] + " And " + items[1] + "\n"
		default:
			parts := append(items[:len(items)-1], "And "+items[len(items)-1])
			return strings.Join(parts, " ") + "\n"
		}
	})
}

// ─── md-ordered-list ─────────────────────────────────────────────────────────

var (
	mdOLBlockRe = regexp.MustCompile(`(?m)(?:^[ \t]*\d+\.\s+[^\n]+\n?)+`)
	mdOLItemRe  = regexp.MustCompile(`(?m)^[ \t]*\d+\.\s+([^\n]+)`)
)

var spokenOrdinals = []string{
	"First", "Second", "Third", "Fourth", "Fifth",
	"Sixth", "Seventh", "Eighth", "Ninth", "Tenth",
}

func handlerMDOrderedList(text string, _ map[string]string) string {
	return mdOLBlockRe.ReplaceAllStringFunc(text, func(block string) string {
		matches := mdOLItemRe.FindAllStringSubmatch(block, -1)
		if len(matches) == 0 {
			return block
		}
		items := make([]string, len(matches))
		for i, m := range matches {
			items[i] = ensurePunct(m[1])
		}
		spoken := make([]string, len(items))
		for i, item := range items {
			body := strings.ToLower(item[:1]) + item[1:]
			if len(items) == 1 {
				spoken[i] = item
			} else if i == len(items)-1 {
				spoken[i] = "And finally, " + body
			} else if i < len(spokenOrdinals) {
				spoken[i] = spokenOrdinals[i] + ", " + body
			} else {
				spoken[i] = item
			}
		}
		return strings.Join(spoken, " ") + "\n"
	})
}

// ─── currency ────────────────────────────────────────────────────────────────

func makeCurrencyHandler(_, patternStr, singular, plural, penceWord string) handlerFunc {
	re := regexp.MustCompile(patternStr)
	return func(text string, _ map[string]string) string {
		return re.ReplaceAllStringFunc(text, func(m string) string {
			parts := re.FindStringSubmatch(m)
			if len(parts) < 3 {
				return m
			}
			var whole, pence int
			fmt.Sscanf(parts[1], "%d", &whole)
			fmt.Sscanf(parts[2], "%d", &pence)
			unit := plural
			if whole == 1 {
				unit = singular
			}
			if pence > 0 {
				pWord := penceWord
				if pence == 1 {
					pWord = "penny"
				}
				return fmt.Sprintf("%d %s and %d %s", whole, unit, pence, pWord)
			}
			return fmt.Sprintf("%d %s", whole, unit)
		})
	}
}

// ─── time-24h ────────────────────────────────────────────────────────────────

var time24Re = regexp.MustCompile(`\b([01]?\d|2[0-3]):([0-5]\d)\b`)

func handlerTime24H(text string, _ map[string]string) string {
	return time24Re.ReplaceAllStringFunc(text, func(m string) string {
		parts := time24Re.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		var h, mn int
		fmt.Sscanf(parts[1], "%d", &h)
		fmt.Sscanf(parts[2], "%d", &mn)
		suffix := "AM"
		if h >= 12 {
			suffix = "PM"
		}
		h12 := h % 12
		if h12 == 0 {
			h12 = 12
		}
		if mn == 0 {
			return fmt.Sprintf("%d %s", h12, suffix)
		}
		return fmt.Sprintf("%d:%s %s", h12, parts[2], suffix)
	})
}

// ─── date dd/mm/yyyy ─────────────────────────────────────────────────────────

var dateDMYRe = regexp.MustCompile(`\b(0?[1-9]|[12]\d|3[01])/(0?[1-9]|1[0-2])/(\d{4})\b`)

var months = map[string]string{
	"01": "January", "02": "February", "03": "March", "04": "April",
	"05": "May", "06": "June", "07": "July", "08": "August",
	"09": "September", "10": "October", "11": "November", "12": "December",
}

func handlerDateDMY(text string, _ map[string]string) string {
	return dateDMYRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := dateDMYRe.FindStringSubmatch(m)
		if len(parts) < 4 {
			return m
		}
		var d int
		fmt.Sscanf(parts[1], "%d", &d)
		mo := fmt.Sprintf("%02s", parts[2])
		if len(mo) < 2 {
			mo = "0" + mo
		}
		if len(parts[2]) == 1 {
			mo = "0" + parts[2]
		} else {
			mo = parts[2]
		}
		// zero-pad to get map key
		if len(mo) < 2 {
			mo = "0" + mo
		}
		month, ok := months[mo]
		if !ok {
			month = mo
		}
		return "the " + ordinalWord(d) + " of " + month + " " + parts[3]
	})
}

// ─── ordinals ────────────────────────────────────────────────────────────────

var ordinalRe = regexp.MustCompile(`(?i)\b(\d+)(?:st|nd|rd|th)\b`)

var ordinalWords = map[int]string{
	1: "first", 2: "second", 3: "third", 4: "fourth", 5: "fifth",
	6: "sixth", 7: "seventh", 8: "eighth", 9: "ninth", 10: "tenth",
	11: "eleventh", 12: "twelfth", 13: "thirteenth", 14: "fourteenth",
	15: "fifteenth", 16: "sixteenth", 17: "seventeenth", 18: "eighteenth",
	19: "nineteenth", 20: "twentieth", 21: "twenty-first", 22: "twenty-second",
	23: "twenty-third", 24: "twenty-fourth", 25: "twenty-fifth", 26: "twenty-sixth",
	27: "twenty-seventh", 28: "twenty-eighth", 29: "twenty-ninth", 30: "thirtieth",
	31: "thirty-first",
}

func handlerOrdinals(text string, _ map[string]string) string {
	return ordinalRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := ordinalRe.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		var n int
		fmt.Sscanf(parts[1], "%d", &n)
		return ordinalWord(n)
	})
}

func ordinalWord(n int) string {
	if w, ok := ordinalWords[n]; ok {
		return w
	}
	return fmt.Sprintf("%dth", n)
}

// ─── uv-index ────────────────────────────────────────────────────────────────
// UV(\d+) → UV index \1 — but not when already followed by " index"

var uvIndexRe = regexp.MustCompile(`(?i)\bUV\s*(\d+)\b`)
var uvIndexAlreadyRe = regexp.MustCompile(`(?i)\bUV\s+index\b`)

func handlerUVIndex(text string, _ map[string]string) string {
	// Only replace "UV \d" that is NOT already "UV index"
	return uvIndexRe.ReplaceAllStringFunc(text, func(m string) string {
		// Check if the original match was already "UV index ..." by seeing if
		// "index" appears right after UV in the source — but since we matched
		// UV(\d+) the next char after UV is already a digit, so it's safe.
		parts := uvIndexRe.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return "UV index " + parts[1]
	})
}

// ─── unit handlers (metres, tonnes, grams, litres) ───────────────────────────
// Replace number+unit where unit is NOT followed by a word character.
// We capture the trailing non-word char (or end) and put it back.

func makeUnitHandler(patStr, repl string) handlerFunc {
	re := regexp.MustCompile(`(?i)` + patStr)
	return func(text string, _ map[string]string) string {
		return re.ReplaceAllString(text, repl)
	}
}

// ─── abbreviation handlers ───────────────────────────────────────────────────

func makeAbbrHandler(patStr, repl string) handlerFunc {
	re := regexp.MustCompile(`(?i)` + patStr)
	return func(text string, _ map[string]string) string {
		return re.ReplaceAllString(text, repl)
	}
}

// ─── symbol handlers ─────────────────────────────────────────────────────────
// symbol-plus: \s*\+\s*(?=\w) → ' plus '  — lookahead replaced by capturing word char
// symbol-equals: \s*=\s*(?=\d) → ' equals ' — lookahead replaced by capturing digit
// symbol-ampersand: &(?!#?\w+;) → ' and ' — negative lookahead for HTML entities

var symbolPlusRe = regexp.MustCompile(`\s*\+\s*(\w)`)

func handlerSymbolPlus(text string, _ map[string]string) string {
	return symbolPlusRe.ReplaceAllString(text, " plus $1")
}

var symbolEqualsRe = regexp.MustCompile(`\s*=\s*(\d)`)

func handlerSymbolEquals(text string, _ map[string]string) string {
	return symbolEqualsRe.ReplaceAllString(text, " equals $1")
}

// For ampersand: replace bare & (not part of &entity;) with " and "
// Strategy: replace all &, then htmlEntities handler (runs earlier) has already
// decoded real entities, so any remaining & is bare.
// Here we handle it by using a two-pass approach: protect entities, replace &, restore.
var bareAmpersandEntityRe = regexp.MustCompile(`&(#?[a-zA-Z0-9]+;)`)

func handlerSymbolAmpersand(text string, _ map[string]string) string {
	// Temporarily replace real HTML entities with a placeholder
	type saved struct{ placeholder, original string }
	var saves []saved
	result := bareAmpersandEntityRe.ReplaceAllStringFunc(text, func(m string) string {
		ph := fmt.Sprintf("\x00ENT%d\x00", len(saves))
		saves = append(saves, saved{ph, m})
		return ph
	})
	// Now replace remaining bare &
	result = strings.ReplaceAll(result, "&", " and ")
	// Restore entities
	for _, s := range saves {
		result = strings.ReplaceAll(result, s.placeholder, s.original)
	}
	return result
}

// ─── md-bold-italic ───────────────────────────────────────────────────────────
// Handles **bold** and __bold__ → — bold — (backreference-free via two regexes)

var (
	mdBoldStarRe  = regexp.MustCompile(`\*\*(.*?)\*\*`)
	mdBoldUnderRe = regexp.MustCompile(`__(.*?)__`)
)

func handlerMDBoldItalic(text string, _ map[string]string) string {
	repl := " \u2014 $1 \u2014 "
	text = mdBoldStarRe.ReplaceAllString(text, repl)
	text = mdBoldUnderRe.ReplaceAllString(text, repl)
	return text
}

// ─── md-italic ────────────────────────────────────────────────────────────────
// Handles *italic* and _italic_ → plain text (backreference-free via two regexes)

var (
	mdItalicStarRe  = regexp.MustCompile(`\*(.*?)\*`)
	mdItalicUnderRe = regexp.MustCompile(`_(.*?)_`)
)

func handlerMDItalic(text string, _ map[string]string) string {
	text = mdItalicStarRe.ReplaceAllString(text, "$1")
	text = mdItalicUnderRe.ReplaceAllString(text, "$1")
	return text
}

// ─── helper ──────────────────────────────────────────────────────────────────

func ensurePunct(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	last := s[len(s)-1]
	if last == '.' || last == '!' || last == '?' {
		return s
	}
	return s + "."
}
