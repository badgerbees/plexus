package router

import (
	"plexus/internal/types"
)

type Arbiter struct {
	decoys         *DecoyLexicon
	highConfidence float32
}

func NewArbiter() *Arbiter {
	return &Arbiter{
		decoys:         NewDecoyLexicon(),
		highConfidence: 0.90,
	}
}

func (a *Arbiter) AddDecoys(terms ...string) {
	a.decoys.Add(terms...)
}

func (a *Arbiter) Triage(span types.CandidateSpan) types.TriageDecision {
	if isStructuralType(span.Type) {
		return types.Accept
	}

	if a.decoys.IsDecoy(span.RawText) {
		return types.Reject
	}

	// Multi-word NER spans of decoy-prone types (e.g. "Apollo project") are
	// rejected when any token is a known decoy term.
	if (span.Type == types.Organization || span.Type == types.Address) && a.decoys.ContainsToken(span.RawText) {
		return types.Reject
	}

	if span.Confidence >= a.highConfidence {
		return types.Accept
	}

	// Anything ambiguous goes to the LLM judge; the arbiter's fallback
	// for an unavailable judge is Accept.
	return types.Escalate
}

func isStructuralType(t types.EntityType) bool {
	switch t {
	case types.Email, types.Phone, types.CreditCard, types.NationalID, types.APIKey,
		types.UUID, types.EmployeeID, types.AccountNumber, types.URL:
		return true
	}
	return false
}
