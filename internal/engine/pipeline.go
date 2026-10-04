package engine

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode"

	"plexus/internal/extractor"
	"plexus/internal/graph"
	"plexus/internal/rewriter"
	"plexus/internal/router"
	"plexus/internal/types"
)

type Pipeline struct {
	extractor *extractor.Extractor
	arbiter   *router.Arbiter
	judge     *router.LLMJudge
	graph     *graph.Graph
	stats     Stats
	mu        sync.Mutex
}

type Stats struct {
	DocsProcessed  int
	SpansDetected  int
	SpansAccepted  int
	SpansRejected  int
	SpansEscalated int
	SpansProtected int
	SpansJudged    int
}

type Config struct {
	NEREndpoint   string
	Labels        []string
	JudgeEndpoint string
	JudgeAPIKey   string
	JudgeModel    string
	JudgeJev      bool
}

type spanReplacement struct {
	start, end int
	text       string
}

func New() *Pipeline {
	return &Pipeline{
		extractor: extractor.New(),
		arbiter:   router.NewArbiter(),
		judge:     router.NewLLMJudge(router.JudgeConfig{}),
		graph:     graph.New(),
	}
}

func NewWithConfig(cfg Config) (*Pipeline, error) {
	var ext *extractor.Extractor
	var err error

	if cfg.NEREndpoint != "" {
		ext, err = extractor.NewWithModel(extractor.Config{
			NEREndpoint: cfg.NEREndpoint,
			Labels:      cfg.Labels,
		})
		if err != nil {
			return nil, fmt.Errorf("init extractor: %w", err)
		}
	} else {
		ext = extractor.New()
	}

	judge := router.NewLLMJudge(router.JudgeConfig{
		Endpoint: cfg.JudgeEndpoint,
		APIKey:   cfg.JudgeAPIKey,
		Model:    cfg.JudgeModel,
		Jev:      cfg.JudgeJev,
	})

	return &Pipeline{
		extractor: ext,
		arbiter:   router.NewArbiter(),
		judge:     judge,
		graph:     graph.New(),
	}, nil
}

func (p *Pipeline) AddDecoys(terms ...string) {
	p.arbiter.AddDecoys(terms...)
}

func (p *Pipeline) LinkAliases(aliases []string) {
	p.graph.LinkAliases(aliases)
}

func (p *Pipeline) Process(doc types.Document) (string, []rewriter.Replacement, error) {
	spans := p.extractor.Extract(doc.Content, doc.ID)

	rw := rewriter.New(doc.Content)

	p.mu.Lock()
	p.stats.DocsProcessed++
	p.stats.SpansDetected += len(spans)
	p.mu.Unlock()

	var replacements []rewriter.Replacement
	type pending struct {
		span  types.CandidateSpan
		hints []string
	}
	var pendingSpans []pending

	for i, span := range spans {
		if !rw.ShouldRewrite(span) {
			p.mu.Lock()
			p.stats.SpansProtected++
			p.mu.Unlock()
			continue
		}

		var hints []string
		if needsIdentityHints(span.Type) {
			hints = collectIdentityHints(spans, i)
		}

		decision := p.arbiter.Triage(span)

		if decision == types.Escalate {
			p.mu.Lock()
			p.stats.SpansJudged++
			p.mu.Unlock()
			pendingSpans = append(pendingSpans, pending{span: span, hints: hints})
			continue
		}

		p.mu.Lock()
		switch decision {
		case types.Accept:
			p.stats.SpansAccepted++
		case types.Reject:
			p.stats.SpansRejected++
		}
		p.mu.Unlock()

		if decision == types.Reject {
			continue
		}

		replacements = append(replacements, p.resolveSpan(span, hints))
	}

	if len(pendingSpans) > 0 {
		judgeSpans := make([]types.CandidateSpan, len(pendingSpans))
		for k, pd := range pendingSpans {
			judgeSpans[k] = pd.span
		}
		decisions := p.judge.ReviewBatch(judgeSpans)

		for k, pd := range pendingSpans {
			decision := decisions[k]
			p.mu.Lock()
			switch decision {
			case types.Accept:
				p.stats.SpansAccepted++
			case types.Reject:
				p.stats.SpansRejected++
			default:
				p.stats.SpansEscalated++
				decision = types.Accept
			}
			p.mu.Unlock()

			if decision == types.Reject {
				continue
			}
			replacements = append(replacements, p.resolveSpan(pd.span, pd.hints))
		}
	}

	replacements = p.rescanPersonMentions(doc.Content, replacements)

	return rw.Apply(doc.Content, replacements), replacements, nil
}

func (p *Pipeline) resolveSpan(span types.CandidateSpan, hints []string) rewriter.Replacement {
	synthetic, twinID := p.graph.ResolveInfo(span.RawText, span.Type, hints)
	return rewriter.Replacement{
		Start:          span.StartByte,
		End:            span.EndByte,
		Text:           synthetic,
		PreserveFormat: true,
		SyntheticID:    twinID,
		Type:           span.Type,
	}
}

// rescanPersonMentions finds bare mentions of a detected person's first name
// that the NER model missed (e.g. "Alex" after "Alex Rivera") and maps them
// to the same synthetic twin. Only capitalized, whole-word occurrences are
// considered, and low-entropy clusters (a bare surname like "Chen" or a
// single-letter initial) are skipped to protect linkage precision.
func (p *Pipeline) rescanPersonMentions(text string, replacements []rewriter.Replacement) []rewriter.Replacement {
	covered := make([]bool, len(text))
	for _, r := range replacements {
		for i := r.Start; i < r.End && i < len(text); i++ {
			covered[i] = true
		}
	}

	var extra []rewriter.Replacement
	seenTokens := make(map[string]bool)

	for _, r := range replacements {
		if r.Type != types.Person {
			continue
		}
		if r.Start < 0 || r.End > len(text) || r.Start >= r.End {
			continue
		}

		canonical := p.graph.CanonicalKey(text[r.Start:r.End])
		tokens := strings.Fields(canonical)
		if len(tokens) < 2 {
			continue
		}
		first := tokens[0]
		if len(first) < 2 || seenTokens[first] {
			continue
		}
		seenTokens[first] = true

		for i := 0; i+len(first) <= len(text); i++ {
			if covered[i] {
				continue
			}
			if i > 0 && isWordChar(text[i-1]) {
				continue
			}
			if i+len(first) < len(text) && isWordChar(text[i+len(first)]) {
				continue
			}
			if !unicode.IsUpper(rune(text[i])) {
				continue
			}
			if !strings.EqualFold(text[i:i+len(first)], first) {
				continue
			}

			synthetic, twinID := p.graph.ResolveInfo(text[i:i+len(first)], types.Person, nil)
			extra = append(extra, rewriter.Replacement{
				Start:          i,
				End:            i + len(first),
				Text:           synthetic,
				PreserveFormat: true,
				SyntheticID:    twinID,
				Type:           types.Person,
			})
			for j := i; j < i+len(first); j++ {
				covered[j] = true
			}
		}
	}

	if len(extra) > 0 {
		p.mu.Lock()
		p.stats.SpansAccepted += len(extra)
		p.mu.Unlock()
	}

	return append(replacements, extra...)
}

func isWordChar(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// needsIdentityHints reports span types that carry no self-linking identity
// signal (a phone number alone does not say whose it is) and should adopt
// the identity of a nearby email or name. Credit cards, API keys and UUIDs
// are resource identifiers, not person attributes, so they keep their own
// synthetic identity.
func needsIdentityHints(t types.EntityType) bool {
	return t == types.Phone
}

// collectIdentityHints returns up to three raw texts of the nearest
// email/person spans to the given span, ordered by byte distance.
func collectIdentityHints(spans []types.CandidateSpan, target int) []string {
	type hint struct {
		raw      string
		distance int
	}
	var candidates []hint
	for j, s := range spans {
		if j == target {
			continue
		}
		if s.Type != types.Email && s.Type != types.Person {
			continue
		}
		dist := s.StartByte - spans[target].StartByte
		if dist < 0 {
			dist = -dist
		}
		candidates = append(candidates, hint{raw: s.RawText, distance: dist})
	}

	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].distance < candidates[i].distance {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	hints := make([]string, 0, 3)
	for _, c := range candidates {
		if len(hints) >= 3 {
			break
		}
		if c.distance > 80 {
			break
		}
		hints = append(hints, c.raw)
	}
	return hints
}

func (p *Pipeline) ProcessBatch(docs []types.Document) ([]string, error) {
	outputs, _, err := p.ProcessBatchDetailed(docs)
	return outputs, err
}

// ProcessBatchDetailed processes documents concurrently, returning both the
// rewritten texts and their replacements. Results preserve input order.
func (p *Pipeline) ProcessBatchDetailed(docs []types.Document) ([]string, [][]rewriter.Replacement, error) {
	outputs := make([]string, len(docs))
	replacements := make([][]rewriter.Replacement, len(docs))
	errs := make([]error, len(docs))
	var wg sync.WaitGroup

	for i, doc := range docs {
		wg.Add(1)
		go func(idx int, d types.Document) {
			defer wg.Done()
			out, reps, err := p.Process(d)
			outputs[idx] = out
			replacements[idx] = reps
			errs[idx] = err
		}(i, doc)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return outputs, replacements, fmt.Errorf("doc %d: %w", i, err)
		}
	}
	return outputs, replacements, nil
}

func (p *Pipeline) GetStats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

func (p *Pipeline) Close() {
	p.extractor.Close()
	s := p.GetStats()
	log.Printf("pipeline closed | docs=%d spans=%d accepted=%d rejected=%d escalated=%d protected=%d judged=%d",
		s.DocsProcessed, s.SpansDetected, s.SpansAccepted, s.SpansRejected, s.SpansEscalated, s.SpansProtected, s.SpansJudged)
}
