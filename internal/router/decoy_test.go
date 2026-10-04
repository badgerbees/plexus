package router

import (
	"testing"

	"plexus/internal/types"
)

func TestDecoyLexicon_ContainsToken(t *testing.T) {
	d := NewDecoyLexicon()
	if !d.ContainsToken("Apollo project") {
		t.Fatal("multi-word span containing a decoy token should match")
	}
	if !d.ContainsToken("Kubernetes") {
		t.Fatal("single decoy token should match")
	}
	if d.ContainsToken("Acme Corporation") {
		t.Fatal("span without decoy tokens must not match")
	}
}

func TestArbiter_RejectsDecoyPhrase(t *testing.T) {
	a := NewArbiter()
	span := types.CandidateSpan{
		RawText:    "Apollo project",
		Type:       types.Organization,
		Confidence: 0.99,
	}
	if got := a.Triage(span); got != types.Reject {
		t.Fatalf("decoy phrase got %v, want Reject", got)
	}
}

func TestArbiter_AcceptsRealOrganization(t *testing.T) {
	a := NewArbiter()
	span := types.CandidateSpan{
		RawText:    "Dana-Farber Cancer Institute",
		Type:       types.Organization,
		Confidence: 0.99,
	}
	if got := a.Triage(span); got != types.Accept {
		t.Fatalf("real organization got %v, want Accept", got)
	}
}

func TestArbiter_StructuralSkipsDecoyCheck(t *testing.T) {
	a := NewArbiter()
	span := types.CandidateSpan{
		RawText:    "alex.rivera@acmecorp.com",
		Type:       types.Email,
		Confidence: 0.99,
	}
	if got := a.Triage(span); got != types.Accept {
		t.Fatalf("structural span got %v, want Accept", got)
	}
}