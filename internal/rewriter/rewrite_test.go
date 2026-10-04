package rewriter

import (
	"testing"

	"plexus/internal/types"
)

func TestSegmentText_FencedCode(t *testing.T) {
	text := "Hello world\n```\nSELECT * FROM users;\n```\nGoodbye"
	blocks := SegmentText(text)

	foundCode := false
	foundPlain := false
	for _, b := range blocks {
		if b.Type == BlockCodeFenced {
			foundCode = true
		}
		if b.Type == BlockPlainText {
			foundPlain = true
		}
	}

	if !foundCode {
		t.Fatal("should detect fenced code block")
	}
	if !foundPlain {
		t.Fatal("should detect plain text blocks")
	}
}

func TestSegmentText_SQL(t *testing.T) {
	text := "Run this query:\nSELECT name FROM users WHERE id = 5;\nThen check results."
	blocks := SegmentText(text)

	foundSQL := false
	for _, b := range blocks {
		if b.Type == BlockSQLQuery {
			foundSQL = true
		}
	}
	if !foundSQL {
		t.Fatal("should detect SQL query")
	}
}

func TestSpanInProtectedBlock(t *testing.T) {
	text := "```\nAlex Rivera\n```\nAlex Rivera said hi."
	blocks := SegmentText(text)

	insideCode := SpanInProtectedBlock(blocks, 4, 15)
	outsideCode := SpanInProtectedBlock(blocks, 19, 30)

	if !insideCode {
		t.Fatal("span inside code block should be protected")
	}
	if outsideCode {
		t.Fatal("span outside code block should not be protected")
	}
}

func TestShouldRewrite_StructuralPIIInCode(t *testing.T) {
	text := "```\nemail: alex@corp.com\n```"
	rw := New(text)

	span := types.CandidateSpan{
		Type:      types.Email,
		StartByte: 11,
		EndByte:   24,
	}

	if !rw.ShouldRewrite(span) {
		t.Fatal("structural PII in code blocks should still be rewritten")
	}
}

// Regression test: person names inside protected code blocks are real PII
// and must still be redacted (code protection only guards against false
// positives like tech terms).
func TestShouldRewrite_PersonInCode(t *testing.T) {
	text := "```\nAlex Rivera\n```"
	rw := New(text)

	span := types.CandidateSpan{
		Type:      types.Person,
		StartByte: 4,
		EndByte:   15,
	}

	if !rw.ShouldRewrite(span) {
		t.Fatal("person name inside a code block should still be rewritten")
	}
}

func TestMatchFormat_Upper(t *testing.T) {
	result := matchFormat("ALEX RIVERA", "Jordan Vance")
	if result != "JORDAN VANCE" {
		t.Errorf("expected JORDAN VANCE, got %s", result)
	}
}

func TestMatchFormat_Lower(t *testing.T) {
	result := matchFormat("alex rivera", "Jordan Vance")
	if result != "jordan vance" {
		t.Errorf("expected jordan vance, got %s", result)
	}
}

func TestMatchFormat_Snake(t *testing.T) {
	result := matchFormat("alex_rivera", "Jordan Vance")
	if result != "jordan_vance" {
		t.Errorf("expected jordan_vance, got %s", result)
	}
}

func TestMatchFormat_Title(t *testing.T) {
	result := matchFormat("Alex Rivera", "jordan vance")
	if result != "Jordan Vance" {
		t.Errorf("expected Jordan Vance, got %s", result)
	}
}
