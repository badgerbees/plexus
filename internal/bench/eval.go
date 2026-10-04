package bench

import (
	"fmt"
	"math"
	"strings"
)

type Annotation struct {
	Text     string `json:"text"`
	Type     string `json:"type"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	IsDecoy  bool   `json:"is_decoy"`
	EntityID string `json:"entity_id"`
}

type TestCase struct {
	ID          string       `json:"id"`
	Input       string       `json:"input"`
	Annotations []Annotation `json:"annotations"`
}

type DetectedSpan struct {
	Text     string `json:"text"`
	Type     string `json:"type"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Replaced string `json:"replaced"`
}

type LinkageMapping struct {
	SourceEntityID    string `json:"source_entity_id"`
	SyntheticEntityID string `json:"synthetic_entity_id"`
}

type EvalResult struct {
	CaseID string

	TruePositives  int
	FalsePositives int
	FalseNegatives int

	DecoyCorrect int
	DecoyTotal   int
	DecoyErrors  int

	LinkagePairs    int
	LinkageCorrect  int
	LinkageMerged   int
	LinkageFragmented int
}

type BenchmarkResult struct {
	CaseResults []EvalResult

	SpanPrecision float64
	SpanRecall    float64
	SpanF1        float64

	DecoyAccuracy float64
	DecoyErrorRate float64

	LinkagePrecision float64
	LinkageRecall    float64
	LinkageF1        float64
}

// Evaluate scores one test case. decoyHits is the number of decoy-annotated
// spans that the engine actually masked (computed by the runner from the
// replacements); it directly measures decoy over-masking.
func Evaluate(tc TestCase, detected []DetectedSpan, linkages []LinkageMapping, decoyHits int) EvalResult {
	result := EvalResult{CaseID: tc.ID}

	piiAnnotations := filterPII(tc.Annotations)
	decoyAnnotations := filterDecoys(tc.Annotations)

	matched := make(map[int]bool)
	for _, det := range detected {
		hit := false
		for i, ann := range piiAnnotations {
			if matched[i] {
				continue
			}
			if spansOverlap(det.Start, det.End, ann.Start, ann.End) {
				result.TruePositives++
				matched[i] = true
				hit = true
				break
			}
		}
		if !hit {
			result.FalsePositives++
		}
	}

	result.FalseNegatives = len(piiAnnotations) - result.TruePositives
	result.DecoyTotal = len(decoyAnnotations)
	result.DecoyErrors = decoyHits
	result.DecoyCorrect = result.DecoyTotal - result.DecoyErrors

	entityGroups := groupByEntity(tc.Annotations)
	syntheticGroups := groupLinkages(linkages)

	for entityID, expectedCount := range entityGroups {
		syntheticID, hasSynthetic := findSyntheticFor(entityID, linkages)
		if !hasSynthetic {
			result.LinkageFragmented += expectedCount
			continue
		}

		result.LinkagePairs += expectedCount
		syntheticCount := syntheticGroups[syntheticID]
		if syntheticCount == expectedCount {
			result.LinkageCorrect += expectedCount
		} else if syntheticCount > expectedCount {
			result.LinkageMerged += syntheticCount - expectedCount
			result.LinkageCorrect += expectedCount
		} else {
			result.LinkageCorrect += syntheticCount
			result.LinkageFragmented += expectedCount - syntheticCount
		}
	}

	return result
}

func Aggregate(results []EvalResult) BenchmarkResult {
	br := BenchmarkResult{CaseResults: results}

	var totalTP, totalFP, totalFN int
	var totalDecoyCorrect, totalDecoyTotal, totalDecoyErrors int
	var totalLinkagePairs, totalLinkageCorrect, totalLinkageMerged int

	for _, r := range results {
		totalTP += r.TruePositives
		totalFP += r.FalsePositives
		totalFN += r.FalseNegatives
		totalDecoyCorrect += r.DecoyCorrect
		totalDecoyTotal += r.DecoyTotal
		totalDecoyErrors += r.DecoyErrors
		totalLinkagePairs += r.LinkagePairs
		totalLinkageCorrect += r.LinkageCorrect
		totalLinkageMerged += r.LinkageMerged
	}

	if totalTP+totalFP > 0 {
		br.SpanPrecision = float64(totalTP) / float64(totalTP+totalFP)
	}
	if totalTP+totalFN > 0 {
		br.SpanRecall = float64(totalTP) / float64(totalTP+totalFN)
	}
	if br.SpanPrecision+br.SpanRecall > 0 {
		br.SpanF1 = 2 * br.SpanPrecision * br.SpanRecall / (br.SpanPrecision + br.SpanRecall)
	}

	if totalDecoyTotal > 0 {
		br.DecoyAccuracy = float64(totalDecoyCorrect) / float64(totalDecoyTotal)
		br.DecoyErrorRate = float64(totalDecoyErrors) / float64(totalDecoyTotal)
	}

	if totalLinkagePairs > 0 {
		br.LinkageRecall = float64(totalLinkageCorrect) / float64(totalLinkagePairs)
	}
	if totalLinkageCorrect+totalLinkageMerged > 0 {
		br.LinkagePrecision = float64(totalLinkageCorrect) / float64(totalLinkageCorrect+totalLinkageMerged)
	}
	if br.LinkagePrecision+br.LinkageRecall > 0 {
		br.LinkageF1 = 2 * br.LinkagePrecision * br.LinkageRecall / (br.LinkagePrecision + br.LinkageRecall)
	}

	return br
}

func (br BenchmarkResult) Report() string {
	var sb strings.Builder
	sb.WriteString("╔══════════════════════════════════════════╗\n")
	sb.WriteString("║         PLEXUS PRIVACY BENCH             ║\n")
	sb.WriteString("╠══════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║  Span Precision:    %.4f               ║\n", br.SpanPrecision))
	sb.WriteString(fmt.Sprintf("║  Span Recall:       %.4f               ║\n", br.SpanRecall))
	sb.WriteString(fmt.Sprintf("║  Span F1:           %.4f               ║\n", br.SpanF1))
	sb.WriteString("╠══════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║  Decoy Accuracy:    %.4f               ║\n", br.DecoyAccuracy))
	sb.WriteString(fmt.Sprintf("║  Decoy Error Rate:  %.4f               ║\n", br.DecoyErrorRate))
	sb.WriteString("╠══════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║  Linkage Precision: %.4f               ║\n", br.LinkagePrecision))
	sb.WriteString(fmt.Sprintf("║  Linkage Recall:    %.4f               ║\n", br.LinkageRecall))
	sb.WriteString(fmt.Sprintf("║  Linkage F1:        %.4f               ║\n", br.LinkageF1))
	sb.WriteString("╠══════════════════════════════════════════╣\n")

	passSpan := br.SpanF1 >= 0.90
	passDecoy := br.DecoyErrorRate <= 0.40
	passLink := br.LinkageF1 >= 0.85

	sb.WriteString(fmt.Sprintf("║  Span F1 ≥ 0.90:   %s                   ║\n", passFail(passSpan)))
	sb.WriteString(fmt.Sprintf("║  Decoy Err ≤ 40%%:   %s                   ║\n", passFail(passDecoy)))
	sb.WriteString(fmt.Sprintf("║  Linkage F1 ≥ 0.85: %s                   ║\n", passFail(passLink)))
	sb.WriteString("╚══════════════════════════════════════════╝\n")

	return sb.String()
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func spansOverlap(s1, e1, s2, e2 int) bool {
	return s1 < e2 && s2 < e1
}

func filterPII(anns []Annotation) []Annotation {
	var out []Annotation
	for _, a := range anns {
		if !a.IsDecoy {
			out = append(out, a)
		}
	}
	return out
}

func filterDecoys(anns []Annotation) []Annotation {
	var out []Annotation
	for _, a := range anns {
		if a.IsDecoy {
			out = append(out, a)
		}
	}
	return out
}

func groupByEntity(anns []Annotation) map[string]int {
	counts := make(map[string]int)
	for _, a := range anns {
		if a.EntityID != "" && !a.IsDecoy {
			counts[a.EntityID]++
		}
	}
	return counts
}

func groupLinkages(linkages []LinkageMapping) map[string]int {
	counts := make(map[string]int)
	for _, l := range linkages {
		counts[l.SyntheticEntityID]++
	}
	return counts
}

func findSyntheticFor(entityID string, linkages []LinkageMapping) (string, bool) {
	for _, l := range linkages {
		if l.SourceEntityID == entityID {
			return l.SyntheticEntityID, true
		}
	}
	return "", false
}

var _ = math.Round
