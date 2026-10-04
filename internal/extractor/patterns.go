package extractor

import (
	"regexp"
	"strings"

	"plexus/internal/types"
	"plexus/pkg/checksum"
)

type PatternMatcher struct {
	patterns []compiledPattern
}

type compiledPattern struct {
	regex    *regexp.Regexp
	eType    types.EntityType
	validate func(string) bool
	group    int
}

func NewPatternMatcher() *PatternMatcher {
	pm := &PatternMatcher{}
	pm.patterns = []compiledPattern{
		{
			regex:    regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
			eType:    types.Email,
			validate: func(s string) bool { return strings.Contains(s, "@") },
		},
		{
			// RE2 has no lookbehind, and \b fails before a leading "+"
			// (non-word char), so the boundary is expressed as a consumed
			// prefix and the number itself is captured in group 1.
			regex: regexp.MustCompile(`(?:^|[^0-9A-Za-z+])((\+?1[-.\s]?)?\(?[0-9]{3}\)?[-.\s]?[0-9]{3}[-.\s]?[0-9]{4})\b`),
			eType: types.Phone,
			group: 1,
		},
		{
			regex:    regexp.MustCompile(`\b[0-9]{13,19}\b`),
			eType:    types.CreditCard,
			validate: func(s string) bool { return checksum.LuhnValid(s) },
		},
		{
			regex: regexp.MustCompile(`\b[0-9]{16}\b`),
			eType: types.NationalID,
		},
		{
			regex: regexp.MustCompile(`(?i)\b(?:sk|pk|api)[_\-](?:live|test)[_\-][A-Za-z0-9]{24,}\b`),
			eType: types.APIKey,
		},
		{
			regex: regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`),
			eType: types.UUID,
		},
		{
			// @-mentions are person aliases the NER model often misses
			// because BERT tokenizes the @ and underscores away. Capture
			// the handle without the @ so the mention marker survives.
			regex: regexp.MustCompile(`(?:^|[^A-Za-z0-9_])@([A-Za-z0-9_.\-]{2,})`),
			eType: types.Person,
			group: 1,
		},
		{
			// Slack user references, e.g. <@U02TYSON_BEAUMONT>. The U02
			// prefix is stripped during normalization so the handle links
			// to the person's name cluster.
			regex: regexp.MustCompile(`(?:^|[^A-Za-z0-9_])<@([A-Za-z0-9_.\-]+)>`),
			eType: types.Person,
			group: 1,
		},
		{
			// Employee IDs like CMG-204817 or JD118440. At least five digits
			// so department+year codes (CLR-2026) are not swallowed.
			regex: regexp.MustCompile(`\b[A-Za-z]{2,4}(?:-\d{5,8}|\d{5,8})\b`),
			eType: types.EmployeeID,
			validate: func(s string) bool {
				low := strings.ToLower(s)
				for _, m := range []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"} {
					if strings.HasPrefix(low, m) && len(low) > len(m) && low[len(m)] >= '0' && low[len(m)] <= '9' {
						return false
					}
				}
				return true
			},
		},
		{
			// Account numbers like 5583-1188, ****-3342 or 481-002917-901.
			// A validate hook rejects year-shaped prefixes (2026-...) so
			// dates are not swallowed.
			regex: regexp.MustCompile(`\b(\d{3,4}|\*{3,4})[-\s]\d{4}(?:[-\s]\d{3})?\b|\b\d{3,4}[-\s]\d{6}[-\s]\d{3}\b`),
			eType: types.AccountNumber,
			validate: func(s string) bool {
				first := s
				for _, sep := range []string{"-", " "} {
					if i := strings.Index(first, sep); i > 0 {
						first = first[:i]
						break
					}
				}
				if len(first) == 4 && (strings.HasPrefix(first, "19") || strings.HasPrefix(first, "20")) {
					return false
				}
				return true
			},
		},
		{
			// Full URLs only; bare domains are too noisy to pattern-match.
			regex: regexp.MustCompile(`\bhttps?://[^\s<>"']+`),
			eType: types.URL,
		},
	}
	return pm
}

func (pm *PatternMatcher) Scan(text, sourceFile string) []types.CandidateSpan {
	var spans []types.CandidateSpan

	for _, p := range pm.patterns {
		matches := p.regex.FindAllStringSubmatchIndex(text, -1)
		for _, idx := range matches {
			start, end := idx[2*p.group], idx[2*p.group+1]
			if start < 0 || end <= start {
				continue
			}
			raw := text[start:end]
			if p.validate != nil && !p.validate(raw) {
				continue
			}

			contextStart := start - 50
			if contextStart < 0 {
				contextStart = 0
			}
			contextEnd := end + 50
			if contextEnd > len(text) {
				contextEnd = len(text)
			}

			spans = append(spans, types.CandidateSpan{
				RawText:    raw,
				Type:       p.eType,
				StartByte:  start,
				EndByte:    end,
				Confidence: 0.99,
				SourceFile: sourceFile,
				Context:    text[contextStart:contextEnd],
			})
		}
	}
	return spans
}
