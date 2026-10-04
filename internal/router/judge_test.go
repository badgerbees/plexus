package router

import (
	"testing"

	"plexus/internal/types"
)

// Regression tests for the OpenRouter judge wiring: the model may return the
// JSON object bare, wrapped in markdown fences, or embedded in prose, and the
// parser must tolerate all three.
func TestExtractJudgeJSON_Plain(t *testing.T) {
	r, err := extractJudgeJSON(`{"is_pii": true, "reason": "person name"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsPII {
		t.Fatal("expected is_pii true")
	}
}

func TestExtractJudgeJSON_Fenced(t *testing.T) {
	r, err := extractJudgeJSON("```json\n{\"is_pii\": false, \"reason\": \"tech term\"}\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.IsPII {
		t.Fatal("expected is_pii false")
	}
}

func TestExtractJudgeJSON_ProseWrapped(t *testing.T) {
	r, err := extractJudgeJSON("Here's my answer: {\"is_pii\": true, \"reason\": \"real name\"} hope that helps.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsPII {
		t.Fatal("expected is_pii true")
	}
}

func TestExtractJudgeJSON_Invalid(t *testing.T) {
	if _, err := extractJudgeJSON("no json in here at all"); err == nil {
		t.Fatal("expected error for non-JSON response")
	}
}

// Regression test for the fail-open behavior: without an API key the judge
// must be disabled and every review must accept the span (redact it).
func TestNewLLMJudge_DisabledWithoutKey(t *testing.T) {
	j := NewLLMJudge(JudgeConfig{APIKey: "", Endpoint: ""})
	if j.enabled {
		t.Fatal("judge should be disabled without an API key")
	}
	if got := j.Review(types.CandidateSpan{RawText: "Kubernetes", Type: types.Organization}); got != types.Accept {
		t.Fatalf("disabled judge returned %v, want Accept", got)
	}
}

func TestNewLLMJudge_Defaults(t *testing.T) {
	j := NewLLMJudge(JudgeConfig{APIKey: "sk-test"})
	if !j.enabled {
		t.Fatal("judge should be enabled with an API key")
	}
	if j.endpoint != DefaultJudgeEndpoint {
		t.Fatalf("endpoint = %q, want %q", j.endpoint, DefaultJudgeEndpoint)
	}
	if j.model != DefaultJudgeModel {
		t.Fatalf("model = %q, want %q", j.model, DefaultJudgeModel)
	}
}

func TestExtractJudgeArray(t *testing.T) {
	content := "```json\n[{\"is_pii\": false, \"reason\": \"greeting\"}, {\"is_pii\": true, \"reason\": \"name\"}]\n```"
	var out []judgeResponse
	if err := extractJudgeArray(content, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0].IsPII || !out[1].IsPII {
		t.Fatalf("unexpected parse result: %+v", out)
	}
}

func TestExtractJudgeArray_Invalid(t *testing.T) {
	var out []judgeResponse
	if err := extractJudgeArray("no array here", &out); err == nil {
		t.Fatal("expected error for non-array response")
	}
}

func TestReviewBatch_DisabledIsFailOpen(t *testing.T) {
	j := NewLLMJudge(JudgeConfig{APIKey: "", Endpoint: ""})
	spans := []types.CandidateSpan{{RawText: "A"}, {RawText: "B"}}
	decisions := j.ReviewBatch(spans)
	if len(decisions) != 2 || decisions[0] != types.Accept || decisions[1] != types.Accept {
		t.Fatalf("disabled judge must accept everything: %v", decisions)
	}
}
