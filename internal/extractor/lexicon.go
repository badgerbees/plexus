package extractor

import (
	"embed"
	"strings"
	"sync"
	"unicode"

	"plexus/internal/types"
)

//go:embed data/surnames.txt data/orgs.txt data/firstnames.txt
var lexiconFS embed.FS

var (
	surnamesOnce sync.Once
	surnames     map[string]bool

	firstNamesOnce sync.Once
	firstNames     map[string]bool

	orgsOnce   sync.Once
	orgsMulti  map[string]bool
	orgsSingle map[string]bool
)

func loadSurnames() map[string]bool {
	surnamesOnce.Do(func() {
		surnames = make(map[string]bool)
		data, err := lexiconFS.ReadFile("data/surnames.txt")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			if name := strings.TrimSpace(line); name != "" {
				surnames[name] = true
			}
		}
	})
	return surnames
}

func loadFirstNames() map[string]bool {
	firstNamesOnce.Do(func() {
		firstNames = make(map[string]bool)
		data, err := lexiconFS.ReadFile("data/firstnames.txt")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			if name := strings.TrimSpace(line); name != "" {
				firstNames[name] = true
			}
		}
	})
	return firstNames
}

func loadOrgs() (map[string]bool, map[string]bool) {
	orgsOnce.Do(func() {
		orgsMulti = make(map[string]bool)
		orgsSingle = make(map[string]bool)
		data, err := lexiconFS.ReadFile("data/orgs.txt")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			name := strings.ToLower(strings.TrimSpace(line))
			name = strings.ReplaceAll(name, "&", "and")
			name = strings.Join(strings.Fields(name), " ")
			if name == "" {
				continue
			}
			if strings.Contains(name, " ") {
				orgsMulti[name] = true
			} else {
				orgsSingle[name] = true
			}
		}
	})
	return orgsMulti, orgsSingle
}

type wordTok struct {
	start, end int
	word       string
}

// scanLexiconSpans finds entities the NER model misses using deterministic
// gazetteers:
//   - multi-word organization names, case-insensitive ("procter & gamble");
//   - single-word organizations, capitalized occurrences only ("Nike");
//   - surnames, capitalized and preceded by a capitalized word or initial
//     ("Jordan Kemp" -> "Kemp"), which keeps the rule precise enough to avoid
//     common-word collisions.
func scanLexiconSpans(text string, covered []types.CandidateSpan) []types.CandidateSpan {
	surnameSet := loadSurnames()
	firstNameSet := loadFirstNames()
	multi, single := loadOrgs()

	var out []types.CandidateSpan

	isCovered := func(s, e int) bool {
		for _, c := range covered {
			if s < c.EndByte && c.StartByte < e {
				return true
			}
		}
		for _, o := range out {
			if s < o.EndByte && o.StartByte < e {
				return true
			}
		}
		return false
	}

	emit := func(s, e int, t types.EntityType) {
		// The matching rules are deliberately conservative (first-name-gated
		// surnames, capitalized single-word orgs), so hits auto-accept rather
		// than routing through the judge, which vetoes bare surnames it
		// cannot verify.
		out = append(out, types.CandidateSpan{
			RawText:    text[s:e],
			Type:       t,
			StartByte:  s,
			EndByte:    e,
			Confidence: 0.99,
		})
	}

	// Multi-word organizations. Lowercasing is byte-length-preserving for
	// ASCII, so offsets map 1:1; "and" and "&" variants cover both spellings.
	lowText := strings.ToLower(text)
	for name := range multi {
		variants := []string{name, strings.ReplaceAll(name, " and ", " & ")}
		for _, v := range variants {
			for start := 0; start < len(lowText); {
				idx := strings.Index(lowText[start:], v)
				if idx < 0 {
					break
				}
				absStart := start + idx
				absEnd := absStart + len(v)
				start = absEnd
				if absStart > 0 && isWordByte(lowText[absStart-1]) {
					continue
				}
				if absEnd < len(lowText) && isWordByte(lowText[absEnd]) {
					continue
				}
				if isCovered(absStart, absEnd) {
					continue
				}
				emit(absStart, absEnd, types.Organization)
			}
		}
	}

	// Tokenize letter runs for single-word matching.
	var toks []wordTok
	for i := 0; i < len(text); {
		if !isASCIIAlpha(text[i]) {
			i++
			continue
		}
		j := i
		for j < len(text) && isASCIIAlpha(text[j]) {
			j++
		}
		toks = append(toks, wordTok{start: i, end: j, word: text[i:j]})
		i = j
	}

	capitalized := func(w string) bool {
		return len(w) > 0 && unicode.IsUpper(rune(w[0]))
	}

	for i, t := range toks {
		if isCovered(t.start, t.end) {
			continue
		}
		lower := strings.ToLower(t.word)

		if capitalized(t.word) && single[lower] {
			emit(t.start, t.end, types.Organization)
			continue
		}

		if capitalized(t.word) && len(t.word) >= 2 && surnameSet[lower] && i > 0 {
			prev := toks[i-1]
			gap := t.start - prev.end
			if gap == 1 && isNameSeparator(text[prev.end]) {
				prevLower := strings.ToLower(prev.word)
				prevIsInitial := len(prev.word) == 1 || (len(prev.word) == 2 && prev.word[1] == '.')
				if prevIsInitial || firstNameSet[prevLower] {
					emit(t.start, t.end, types.Person)
				}
			}
		}
	}

	return out
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isWordByte(c byte) bool {
	return isASCIIAlpha(c) || (c >= '0' && c <= '9') || c == '_'
}
