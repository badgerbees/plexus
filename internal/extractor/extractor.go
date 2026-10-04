package extractor

import (
	"log"

	"plexus/internal/types"
)

type Extractor struct {
	patterns *PatternMatcher
	remote   *RemoteExtractor
}

type Config struct {
	NEREndpoint string
	Labels      []string
}

func New() *Extractor {
	return &Extractor{
		patterns: NewPatternMatcher(),
	}
}

func NewWithModel(cfg Config) (*Extractor, error) {
	e := &Extractor{
		patterns: NewPatternMatcher(),
	}

	if cfg.NEREndpoint != "" {
		e.remote = NewRemoteExtractor(cfg.NEREndpoint)
		log.Printf("remote NER configured at %s", cfg.NEREndpoint)
	}

	return e, nil
}

func (e *Extractor) Extract(text, sourceFile string) []types.CandidateSpan {
	var all []types.CandidateSpan

	structural := e.patterns.Scan(text, sourceFile)
	structural = dedupeOverlaps(structural)
	all = append(all, structural...)

	if e.remote != nil {
		nerSpans, err := e.remote.Extract(text, sourceFile)
		if err != nil {
			log.Printf("remote NER extraction failed: %v", err)
		} else {
			nerSpans = deduplicateSpans(all, nerSpans)
			// The PII model emits given and family names as separate spans;
			// rejoin adjacent ones so identity resolution sees full names.
			nerSpans = mergeAdjacentSpans(nerSpans, text)
			all = append(all, nerSpans...)
		}
	}

	all = append(all, scanLexiconSpans(text, all)...)

	return all
}

// dedupeOverlaps keeps the first of any overlapping structural spans. The
// matcher emits spans in pattern priority order (e.g. CreditCard before
// NationalID), so a Luhn-valid 16-digit card that also matches the
// NationalID pattern resolves to CreditCard.
func dedupeOverlaps(spans []types.CandidateSpan) []types.CandidateSpan {
	if len(spans) <= 1 {
		return spans
	}
	var kept []types.CandidateSpan
	for _, s := range spans {
		overlaps := false
		for _, k := range kept {
			if spansOverlap(s, k) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			kept = append(kept, s)
		}
	}
	return kept
}

func deduplicateSpans(existing, incoming []types.CandidateSpan) []types.CandidateSpan {
	var unique []types.CandidateSpan
	for _, inc := range incoming {
		overlaps := false
		for _, ex := range existing {
			if spansOverlap(ex, inc) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			unique = append(unique, inc)
		}
	}
	return unique
}

func spansOverlap(a, b types.CandidateSpan) bool {
	return a.StartByte < b.EndByte && b.StartByte < a.EndByte
}

func (e *Extractor) Close() {
	// nothing to close
}
