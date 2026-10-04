package rewriter

import (
	"strings"

	"plexus/internal/types"
)

type Rewriter struct {
	blocks []Block
	text   string
}

func New(text string) *Rewriter {
	return &Rewriter{
		blocks: SegmentText(text),
		text:   text,
	}
}

func (rw *Rewriter) ShouldRewrite(span types.CandidateSpan) bool {
	if SpanInProtectedBlock(rw.blocks, span.StartByte, span.EndByte) {
		return isProtectedBlockPII(span.Type)
	}
	return true
}

func (rw *Rewriter) Apply(text string, replacements []Replacement) string {
	if len(replacements) == 0 {
		return text
	}

	for i := 0; i < len(replacements); i++ {
		for j := i + 1; j < len(replacements); j++ {
			if replacements[j].Start > replacements[i].Start {
				replacements[i], replacements[j] = replacements[j], replacements[i]
			}
		}
	}

	result := text
	for _, r := range replacements {
		if r.Start >= 0 && r.End <= len(result) && r.Start < r.End {
			original := result[r.Start:r.End]
			synthetic := r.Text

			if r.PreserveFormat {
				synthetic = matchFormat(original, synthetic)
			}

			result = result[:r.Start] + synthetic + result[r.End:]
		}
	}
	return result
}

type Replacement struct {
	Start          int
	End            int
	Text           string
	PreserveFormat bool
	SyntheticID    string
	Type           types.EntityType
}

func matchFormat(original, synthetic string) string {
	if len(original) == 0 || len(synthetic) == 0 {
		return synthetic
	}

	if strings.Contains(original, "_") && !strings.Contains(synthetic, "_") {
		return strings.ReplaceAll(strings.ToLower(synthetic), " ", "_")
	}

	if strings.Contains(original, "-") && !strings.Contains(synthetic, "-") {
		return strings.ReplaceAll(strings.ToLower(synthetic), " ", "-")
	}

	if isAllUpper(original) {
		return strings.ToUpper(synthetic)
	}

	if isAllLower(original) {
		return strings.ToLower(synthetic)
	}

	if isTitleCase(original) {
		return toTitleCase(synthetic)
	}

	if isCamelCase(original) {
		return toCamelLike(synthetic)
	}

	return synthetic
}

func isAllUpper(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}

func isAllLower(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return false
		}
	}
	return true
}

func isTitleCase(s string) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if len(w) > 0 && (w[0] < 'A' || w[0] > 'Z') {
			return false
		}
	}
	return true
}

func isCamelCase(s string) bool {
	if len(s) < 2 {
		return false
	}
	hasLower := false
	hasUpper := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			hasLower = true
		}
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
	}
	return hasLower && hasUpper && !strings.Contains(s, " ")
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

func toCamelLike(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, "")
}

func isStructuralPII(t types.EntityType) bool {
	switch t {
	case types.Email, types.Phone, types.CreditCard, types.NationalID, types.APIKey:
		return true
	}
	return false
}

// isProtectedBlockPII reports whether a span inside code/SQL/JSON/shell
// blocks is genuine PII that must still be redacted. Person names qualify:
// code protection exists to keep false positives (tech terms, identifiers)
// intact, not to leak real names embedded in code or queries.
func isProtectedBlockPII(t types.EntityType) bool {
	switch t {
	case types.Email, types.Phone, types.CreditCard, types.NationalID, types.APIKey, types.UUID,
		types.EmployeeID, types.AccountNumber, types.URL, types.Person:
		return true
	}
	return false
}
