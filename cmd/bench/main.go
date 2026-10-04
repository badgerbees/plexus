package main

import (
	"flag"
	"fmt"
	"os"

	"plexus/internal/bench"
	"plexus/internal/config"
	"plexus/internal/engine"
	"plexus/internal/rewriter"
	"plexus/internal/types"
)

func main() {
	fmt.Println("Running Plexus PrivacyBench...")
	fmt.Println()

	judgeEnabled := flag.Bool("judge", false, "enable the LLM judge (default off for deterministic runs)")
	flag.Parse()

	cfg := config.Load()

	judgeEndpoint, judgeKey, judgeModel := "", "", ""
	judgeJev := false
	if *judgeEnabled {
		judge := cfg.Judge("", "", "")
		judgeEndpoint = judge.Endpoint
		judgeKey = judge.APIKey
		judgeModel = judge.Model
		judgeJev = judge.Jev
	}

	pipe, err := engine.NewWithConfig(engine.Config{
		NEREndpoint:   cfg.NEREndpoint,
		JudgeEndpoint: judgeEndpoint,
		JudgeAPIKey:   judgeKey,
		JudgeModel:    judgeModel,
		JudgeJev:      judgeJev,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init pipeline: %v\n", err)
		os.Exit(1)
	}
	defer pipe.Close()

	suite := bench.GenerateTestSuite()
	var results []bench.EvalResult

	for _, tc := range suite {
		doc := types.Document{
			ID:      tc.ID,
			Source:  "bench",
			Format:  "text",
			Content: tc.Input,
		}

		_, replacements, err := pipe.Process(doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: case %s failed: %v\n", tc.ID, err)
			continue
		}

		detected := extractDetectedSpans(replacements, tc.Annotations)
		linkages := extractLinkagesFromReplacements(replacements, tc.Annotations)
		decoyHits := countDecoyHits(replacements, tc.Annotations)

		result := bench.Evaluate(tc, detected, linkages, decoyHits)
		results = append(results, result)

		fmt.Printf("Case: %-30s TP=%d FP=%d FN=%d DecoyErr=%d\n",
			tc.ID, result.TruePositives, result.FalsePositives, result.FalseNegatives, result.DecoyErrors)
	}

	fmt.Println()
	aggregate := bench.Aggregate(results)
	fmt.Print(aggregate.Report())

	stats := pipe.GetStats()
	fmt.Fprintf(os.Stderr, "\npipeline: docs=%d spans=%d accepted=%d rejected=%d escalated=%d protected=%d\n",
		stats.DocsProcessed, stats.SpansDetected, stats.SpansAccepted,
		stats.SpansRejected, stats.SpansEscalated, stats.SpansProtected)
}

func extractDetectedSpans(replacements []rewriter.Replacement, annotations []bench.Annotation) []bench.DetectedSpan {
	var detected []bench.DetectedSpan

	for _, ann := range annotations {
		if ann.IsDecoy {
			continue
		}

		for _, rep := range replacements {
			if spansOverlap(rep.Start, rep.End, ann.Start, ann.End) {
				detected = append(detected, bench.DetectedSpan{
					Text:     ann.Text,
					Type:     ann.Type,
					Start:    ann.Start,
					End:      ann.End,
					Replaced: rep.Text,
				})
				break
			}
		}
	}

	return detected
}

func extractLinkagesFromReplacements(replacements []rewriter.Replacement, annotations []bench.Annotation) []bench.LinkageMapping {
	var linkages []bench.LinkageMapping

	for _, ann := range annotations {
		if ann.IsDecoy || ann.EntityID == "" {
			continue
		}

		best := -1
		bestOverlap := 0
		for i, rep := range replacements {
			ov := overlapLen(rep.Start, rep.End, ann.Start, ann.End)
			if ov > bestOverlap {
				bestOverlap = ov
				best = i
			}
		}
		if best >= 0 {
			linkages = append(linkages, bench.LinkageMapping{
				SourceEntityID:    ann.EntityID,
				SyntheticEntityID: replacements[best].SyntheticID,
			})
		}
	}

	return linkages
}

func spansOverlap(s1, e1, s2, e2 int) bool {
	return s1 < e2 && s2 < e1
}

// countDecoyHits counts decoy-annotated spans that the engine actually
// masked, i.e. the decoy over-masking the Utility pillar penalizes.
func countDecoyHits(replacements []rewriter.Replacement, annotations []bench.Annotation) int {
	hits := 0
	for _, ann := range annotations {
		if !ann.IsDecoy {
			continue
		}
		for _, rep := range replacements {
			if spansOverlap(rep.Start, rep.End, ann.Start, ann.End) {
				hits++
				break
			}
		}
	}
	return hits
}

func overlapLen(s1, e1, s2, e2 int) int {
	start := s1
	if s2 > start {
		start = s2
	}
	end := e1
	if e2 < end {
		end = e2
	}
	if end <= start {
		return 0
	}
	return end - start
}
