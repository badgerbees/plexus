package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"plexus/internal/config"
	"plexus/internal/engine"
	"plexus/internal/rewriter"
	"plexus/internal/types"
)

type gtSpan struct {
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type doc struct {
	ID    string   `json:"id"`
	Text  string   `json:"text"`
	Spans []gtSpan `json:"spans"`
}

var supportedLabels = map[string]types.EntityType{
	"NAME_GIVEN":       types.Person,
	"NAME_FAMILY":      types.Person,
	"USERNAME":         types.Person,
	"ORGANIZATION":     types.Organization,
	"EMAIL_ADDRESS":    types.Email,
	"PHONE_NUMBER":     types.Phone,
	"LOCATION_ADDRESS": types.Address,
	"EMPLOYEE_ID":      types.EmployeeID,
	"ACCOUNT_NUMBER":   types.AccountNumber,
	"URL":              types.URL,
}

type typeStats struct {
	tp, fp, fn int
}

const batchSize = 8

func main() {
	inputPath := flag.String("input", "testdata/privacybench.jsonl", "path to exported Privacy-Bench JSONL")
	judge := flag.Bool("judge", false, "enable the LLM judge (default off for deterministic timing)")
	limit := flag.Int("limit", 0, "max docs to evaluate (0 = all)")
	flag.Parse()

	cfg := config.Load()

	judgeEndpoint, judgeKey, judgeModel := "", "", ""
	judgeJev := false
	if *judge {
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
		fmt.Fprintf(os.Stderr, "init pipeline: %v\n", err)
		os.Exit(1)
	}
	defer pipe.Close()

	docs, err := loadDocs(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load docs: %v\n", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(docs) {
		docs = docs[:*limit]
	}

	fmt.Printf("Running Plexus vs TonicAI/Privacy-Bench (%d docs, judge=%v, ner=%s)\n\n",
		len(docs), *judge, cfg.NEREndpoint)

	start := time.Now()
	var (
		byType        = map[string]*typeStats{}
		supported     typeStats
		allLabels     typeStats
		unsupportedGT int
		totalTokens   int
		docsOK        int
		tpW, fpW, fnW float64
		protTotal     int
		protSame      int
	)

	for start := 0; start < len(docs); start += batchSize {
		if start%100 == 0 {
			fmt.Fprintf(os.Stderr, "doc %d/%d...\n", start, len(docs))
		}
		end := start + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[start:end]
		batchDocs := make([]types.Document, len(batch))
		for i, d := range batch {
			totalTokens += len(strings.Fields(d.Text))
			batchDocs[i] = types.Document{ID: d.ID, Content: d.Text, Format: "text"}
		}

		outputs, batchReps, err := pipe.ProcessBatchDetailed(batchDocs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "batch %d failed: %v\n", start, err)
			continue
		}

		for i, d := range batch {
			docsOK++
			replacements := batchReps[i]

			dw, dfp, dfn := scoreDoc(&byType, &supported, &allLabels, &unsupportedGT, replacements, d.Spans)
			tpW += dw
			fpW += dfp
			fnW += dfn

			pt, ps := fidelity(d.Text, outputs[i], replacements)
			protTotal += pt
			protSame += ps
		}
	}

	elapsed := time.Since(start)
	secs := elapsed.Seconds()

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║        PRIVACY-BENCH (TonicAI) REPORT        ║")
	fmt.Println("╠══════════════════════════════════════════════╣")
	fmt.Printf("║  Docs evaluated:  %-6d                     ║\n", len(docs))
	fmt.Printf("║  Wall time:       %-12s              ║\n", elapsed.Round(time.Millisecond))
	fmt.Printf("║  Throughput:      %8.1f docs/s           ║\n", float64(len(docs))/secs)
	fmt.Printf("║  Tokens/sec:      %8.1f                  ║\n", float64(totalTokens)/secs)
	fmt.Printf("║  Avg latency/doc: %8.1f ms               ║\n", secs*1000/float64(len(docs)))
	fmt.Println("╠══════════════════════════════════════════════╣")
	p, r, f1 := prf(supported)
	fmt.Printf("║  Span F1 (supported types):  %.4f           ║\n", f1)
	fmt.Printf("║    Precision %.4f  Recall %.4f  (TP=%d FP=%d FN=%d)\n", p, r, supported.tp, supported.fp, supported.fn)
	fmt.Println("╠══════════════════════════════════════════════╣")
	p, r, f1 = prf(allLabels)
	fmt.Printf("║  Span F1 (ALL labels):       %.4f           ║\n", f1)
	fmt.Printf("║    Precision %.4f  Recall %.4f  (TP=%d FP=%d FN=%d)\n", p, r, allLabels.tp, allLabels.fp, allLabels.fn)
	fmt.Printf("║    Unsupported GT spans (no detector): %d     ║\n", unsupportedGT)
	fmt.Println("╠══════════════════════════════════════════════╣")
	fmt.Println("║  Per-type (supported):                       ║")
	for label, st := range byType {
		p, r, f1 := prf(*st)
		fmt.Printf("║    %-16s P=%.3f R=%.3f F1=%.3f (tp=%d fp=%d fn=%d)\n",
			label, p, r, f1, st.tp, st.fp, st.fn)
	}
	fmt.Println("╠══════════════════════════════════════════════╣")
	fmt.Println("║  TQI PILLARS (micro1 framework)              ║")
	privacy := 0.0
	if tpW+fnW > 0 {
		privacy = tpW / (tpW + fnW)
	}
	utility := 0.0
	if tpW+fpW > 0 {
		utility = tpW / (tpW + fpW)
	}
	coverage := float64(docsOK) / float64(len(docs))
	fid := 1.0
	if protTotal > 0 {
		fid = float64(protSame) / float64(protTotal)
	}
	const linkage = 1.0 // from the internal linkage bench (cmd/bench)
	tqi := 100 * math.Pow(privacy, 0.40) * math.Pow(utility, 0.30) *
		math.Pow(coverage, 0.05) * math.Pow(fid, 0.05) * math.Pow(linkage, 0.20)
	fmt.Printf("║    Privacy  (P, w=0.40): %.4f             ║\n", privacy)
	fmt.Printf("║    Utility  (U, w=0.30): %.4f             ║\n", utility)
	fmt.Printf("║    Coverage (C, w=0.05): %.4f             ║\n", coverage)
	fmt.Printf("║    Fidelity (F, w=0.05): %.4f             ║\n", fid)
	fmt.Printf("║    Coherence(L, w=0.20): %.4f (internal)  ║\n", linkage)
	fmt.Printf("║    TQI = 100 * P^.40 U^.30 C^.05 F^.05 L^.20 ║\n")
	fmt.Printf("║    TQI: %.2f / 100                        ║\n", tqi)
	fmt.Println("╚══════════════════════════════════════════════╝")
}

func prf(s typeStats) (float64, float64, float64) {
	p := 0.0
	r := 0.0
	if s.tp+s.fp > 0 {
		p = float64(s.tp) / float64(s.tp+s.fp)
	}
	if s.tp+s.fn > 0 {
		r = float64(s.tp) / float64(s.tp+s.fn)
	}
	f1 := 0.0
	if p+r > 0 {
		f1 = 2 * p * r / (p + r)
	}
	return p, r, f1
}

func scoreDoc(byType *map[string]*typeStats, supported, allLabels *typeStats, unsupportedGT *int, reps []rewriter.Replacement, gts []gtSpan) (tpW, fpW, fnW float64) {
	matched := make([]bool, len(gts))

	for _, rep := range reps {
		hit := false
		for i, gt := range gts {
			if matched[i] {
				continue
			}
			et, ok := supportedLabels[gt.Label]
			if !ok {
				continue
			}
			if !overlap(rep.Start, rep.End, gt.Start, gt.End) {
				continue
			}
			if et != rep.Type {
				continue
			}
			matched[i] = true
			hit = true
			st := statsFor(byType, gt.Label)
			st.tp++
			supported.tp++
			allLabels.tp++
			tpW += types.Severity(et)
		}
		if !hit {
			supported.fp++
			allLabels.fp++
			fpW += types.Severity(rep.Type)
		}
	}

	for i, gt := range gts {
		if matched[i] {
			continue
		}
		if et, ok := supportedLabels[gt.Label]; ok {
			st := statsFor(byType, gt.Label)
			st.fn++
			supported.fn++
			allLabels.fn++
			fnW += types.Severity(et)
		} else {
			*unsupportedGT++
			allLabels.fn++
		}
	}
	return tpW, fpW, fnW
}

// fidelity measures the F pillar: the fraction of bytes inside protected
// blocks (code fences, SQL, JSON, shell) that the transformation preserved.
// Bytes intentionally rewritten inside a block (real PII) are excluded, and
// the comparison is alignment-aware so replacements earlier in the document
// (which shift the output) do not count as corruption.
func fidelity(in, out string, reps []rewriter.Replacement) (total, same int) {
	sorted := make([]rewriter.Replacement, len(reps))
	copy(sorted, reps)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Start < sorted[i].Start {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	for _, b := range rewriter.SegmentText(in) {
		if !rewriter.IsProtectedBlock(b.Type) {
			continue
		}
		delta := 0
		repIdx := 0
		for i := b.Start; i < b.End && i < len(in); i++ {
			for repIdx < len(sorted) && sorted[repIdx].End <= i {
				delta += len(sorted[repIdx].Text) - (sorted[repIdx].End - sorted[repIdx].Start)
				repIdx++
			}
			if repIdx < len(sorted) && sorted[repIdx].Start <= i && i < sorted[repIdx].End {
				continue // intentional PII replacement inside the block
			}
			outIdx := i + delta
			if outIdx >= len(out) {
				continue
			}
			total++
			if in[i] == out[outIdx] {
				same++
			}
		}
	}
	return total, same
}

func statsFor(byType *map[string]*typeStats, label string) *typeStats {
	st, ok := (*byType)[label]
	if !ok {
		st = &typeStats{}
		(*byType)[label] = st
	}
	return st
}

func overlap(s1, e1, s2, e2 int) bool {
	return s1 < e2 && s2 < e1
}

func loadDocs(path string) ([]doc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var docs []doc
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var d doc
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			return nil, fmt.Errorf("parse: %w", err)
		}
		// Ground-truth offsets are character offsets (as exported by the
		// Python exporter); the engine works in byte offsets. Convert so
		// overlap matching is correct for non-ASCII text.
		for i := range d.Spans {
			d.Spans[i].Start = charToByte(d.Text, d.Spans[i].Start)
			d.Spans[i].End = charToByte(d.Text, d.Spans[i].End)
		}
		docs = append(docs, d)
	}
	return docs, scanner.Err()
}

func charToByte(s string, charOffset int) int {
	if charOffset <= 0 {
		return 0
	}
	for i := range s {
		charOffset--
		if charOffset == 0 {
			_, size := utf8.DecodeRuneInString(s[i:])
			return i + size
		}
	}
	return len(s)
}
