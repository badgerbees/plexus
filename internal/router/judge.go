package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"plexus/internal/types"
)

const (
	DefaultJudgeEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	DefaultJudgeModel    = "nvidia/nemotron-3.5-lightning:free"
)

type LLMJudge struct {
	endpoint string
	apiKey   string
	model    string
	jev      bool
	client   *http.Client
	enabled  bool

	mu    sync.Mutex
	cache map[string]types.TriageDecision
}

type JudgeConfig struct {
	Endpoint string
	APIKey   string
	Model    string
	Timeout  time.Duration

	// Jev selects the Jev judge contract (POST {span, context, entity_type}
	// -> {is_pii, reason}) instead of the default OpenAI chat-completions
	// contract used by OpenRouter.
	Jev bool
}

func NewLLMJudge(cfg JudgeConfig) *LLMJudge {
	if cfg.Jev {
		// Jev requires an explicit endpoint; there is no default.
		if cfg.Endpoint == "" || cfg.APIKey == "" {
			return &LLMJudge{enabled: false}
		}
	} else {
		if cfg.Endpoint == "" {
			cfg.Endpoint = DefaultJudgeEndpoint
		}
		if cfg.Model == "" {
			cfg.Model = DefaultJudgeModel
		}
		if cfg.APIKey == "" {
			return &LLMJudge{enabled: false}
		}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}

	return &LLMJudge{
		endpoint: cfg.Endpoint,
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		jev:      cfg.Jev,
		client:   &http.Client{Timeout: timeout},
		enabled:  true,
		cache:    make(map[string]types.TriageDecision),
	}
}

type judgeResponse struct {
	IsPII  bool   `json:"is_pii"`
	Reason string `json:"reason"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	// Providers differ on the error field shape (OpenRouter: object,
	// Ollama: string), so keep it untyped.
	Error any `json:"error"`
}

func (j *LLMJudge) Review(span types.CandidateSpan) types.TriageDecision {
	decisions := j.ReviewBatch([]types.CandidateSpan{span})
	if len(decisions) == 0 {
		return types.Accept
	}
	return decisions[0]
}

// ReviewBatch judges several spans in a single request, amortizing the
// model latency across the batch. Decisions are fail-open (Accept) when the
// judge is disabled, errors, or returns fewer entries than spans.
func (j *LLMJudge) ReviewBatch(spans []types.CandidateSpan) []types.TriageDecision {
	decisions := make([]types.TriageDecision, len(spans))
	for i := range decisions {
		decisions[i] = types.Accept
	}

	if !j.enabled || len(spans) == 0 {
		return decisions
	}

	// Resolve cache hits locally.
	pending := make([]int, 0, len(spans))
	for i, s := range spans {
		cacheKey := strings.ToLower(strings.TrimSpace(s.RawText)) + "|" + string(s.Type) + "|" + s.Context
		j.mu.Lock()
		if d, ok := j.cache[cacheKey]; ok {
			decisions[i] = d
			j.mu.Unlock()
			continue
		}
		j.mu.Unlock()
		pending = append(pending, i)
	}
	if len(pending) == 0 {
		return decisions
	}

	batch := make([]types.CandidateSpan, 0, len(pending))
	for _, i := range pending {
		batch = append(batch, spans[i])
	}

	results := j.reviewBatchRemote(batch)

	for k, i := range pending {
		decisions[i] = results[k]
		j.mu.Lock()
		j.cache[strings.ToLower(strings.TrimSpace(spans[i].RawText))+"|"+string(spans[i].Type)+"|"+spans[i].Context] = results[k]
		j.mu.Unlock()
	}
	return decisions
}

func (j *LLMJudge) reviewBatchRemote(spans []types.CandidateSpan) []types.TriageDecision {
	results := make([]types.TriageDecision, len(spans))
	for i := range results {
		results[i] = types.Accept
	}

	var b strings.Builder
	b.WriteString(`You are a PII classifier for an anonymization pipeline. For each span below decide if it is a GENUINE entity that must be redacted (real person name, real company or institution, real location) or a generic phrase that is NOT real PII (department, greeting, technical term, hallucinated phrase).`)
	b.WriteString("\n\nRespond with ONLY a JSON array, one object per span, in the same order:\n")
	b.WriteString(`[{"is_pii": true/false, "reason": "brief"}]`)
	b.WriteString("\n\nSpans:\n")
	for i, s := range spans {
		b.WriteString(fmt.Sprintf(`%d. Span "%s", guessed type %s, context: "...%s..."`, i+1, s.RawText, string(s.Type), strings.TrimSpace(s.Context)))
		b.WriteString("\n")
	}

	body, err := json.Marshal(openRouterRequest{
		Model: j.model,
		Messages: []openRouterMessage{{
			Role:    "user",
			Content: b.String(),
		}},
	})
	if err != nil {
		log.Printf("judge marshal error: %v", err)
		return results
	}

	httpReq, err := http.NewRequest("POST", j.endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("judge request error: %v", err)
		return results
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if j.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+j.apiKey)
	}
	httpReq.Header.Set("X-Title", "plexus")

	resp, err := j.client.Do(httpReq)
	if err != nil {
		log.Printf("judge call failed: %v", err)
		return results
	}
	defer resp.Body.Close()

	var result openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("judge decode error: %v", err)
		return results
	}
	if resp.StatusCode != http.StatusOK || len(result.Choices) == 0 {
		log.Printf("judge API error %d", resp.StatusCode)
		return results
	}

	var parsed []judgeResponse
	if err := extractJudgeArray(result.Choices[0].Message.Content, &parsed); err != nil {
		log.Printf("judge content parse error: %v", err)
		return results
	}

	for i := 0; i < len(parsed) && i < len(results); i++ {
		if !parsed[i].IsPII {
			results[i] = types.Reject
		}
	}
	return results
}

// extractJudgeArray pulls a JSON array of {is_pii, reason} out of a model
// reply, tolerating markdown fences and prose.
func extractJudgeArray(content string, out *[]judgeResponse) error {
	s := strings.TrimSpace(content)
	for strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(s)
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")

	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return fmt.Errorf("no JSON array in response")
	}
	return json.Unmarshal([]byte(s[start:end+1]), out)
}

func (j *LLMJudge) reviewRemote(span types.CandidateSpan) types.TriageDecision {
	if j.jev {
		return j.reviewJev(span)
	}
	return j.reviewOpenRouter(span)
}

// reviewJev implements the original Jev judge contract: POST the span,
// its type and context, and read back {is_pii, reason}.
func (j *LLMJudge) reviewJev(span types.CandidateSpan) types.TriageDecision {
	req := struct {
		Span   string `json:"span"`
		Type   string `json:"entity_type"`
		Context string `json:"context"`
	}{Span: span.RawText, Type: string(span.Type), Context: span.Context}

	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("judge marshal error: %v", err)
		return types.Accept
	}

	httpReq, err := http.NewRequest("POST", j.endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("judge request error: %v", err)
		return types.Accept
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if j.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+j.apiKey)
	}

	resp, err := j.client.Do(httpReq)
	if err != nil {
		log.Printf("judge call failed: %v", err)
		return types.Accept
	}
	defer resp.Body.Close()

	var result judgeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("judge decode error: %v", err)
		return types.Accept
	}
	if result.IsPII {
		return types.Accept
	}
	return types.Reject
}

func (j *LLMJudge) reviewOpenRouter(span types.CandidateSpan) types.TriageDecision {
	body, err := json.Marshal(openRouterRequest{
		Model: j.model,
		Messages: []openRouterMessage{{
			Role:    "user",
			Content: BuildJudgePrompt(span),
		}},
	})
	if err != nil {
		log.Printf("judge marshal error: %v", err)
		return types.Accept
	}

	httpReq, err := http.NewRequest("POST", j.endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("judge request error: %v", err)
		return types.Accept
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+j.apiKey)
	httpReq.Header.Set("X-Title", "plexus")

	resp, err := j.client.Do(httpReq)
	if err != nil {
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			log.Printf("judge timed out for %q; accepting span (fail-open)", span.RawText)
		} else {
			log.Printf("judge call failed: %v", err)
		}
		return types.Accept
	}
	defer resp.Body.Close()

	var result openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("judge decode error: %v", err)
		return types.Accept
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("judge API error %d: %v", resp.StatusCode, result.Error)
		return types.Accept
	}

	if len(result.Choices) == 0 {
		log.Printf("judge returned no choices")
		return types.Accept
	}

	parsed, err := extractJudgeJSON(result.Choices[0].Message.Content)
	if err != nil {
		log.Printf("judge content parse error: %v", err)
		return types.Accept
	}

	if parsed.IsPII {
		return types.Accept
	}
	return types.Reject
}

// extractJudgeJSON pulls the {is_pii, reason} object out of a model reply,
// tolerating markdown fences and surrounding prose.
func extractJudgeJSON(content string) (*judgeResponse, error) {
	s := strings.TrimSpace(content)

	for strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(s)
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")

	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object in response")
	}

	var r judgeResponse
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return nil, fmt.Errorf("invalid judge JSON: %w", err)
	}
	return &r, nil
}

func BuildJudgePrompt(span types.CandidateSpan) string {
	return fmt.Sprintf(`You are a PII classifier for an anonymization pipeline. Decide whether the span below is a GENUINE entity that must be redacted, or a generic phrase that is not real PII.

Span: "%s"
Entity Type Guess: %s
Surrounding Context: "...%s..."

Respond with JSON only:
{"is_pii": true/false, "reason": "brief explanation"}

Rules:
- Real person names, usernames, email addresses, phone numbers ARE PII
- Real companies, institutions, universities, hospitals ARE PII (they identify the organization)
- Department names, generic job titles, greetings, sign-offs are NOT PII
- Software names, technical terms, product codenames are NOT PII
- Generic phrases the model hallucinated as entities are NOT PII`,
		span.RawText,
		string(span.Type),
		strings.TrimSpace(span.Context),
	)
}
