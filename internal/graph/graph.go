package graph

import (
	"fmt"
	"strings"

	"plexus/internal/types"
	"plexus/pkg/checksum"
)

func (g *Graph) Resolve(rawSpan string, eType types.EntityType) string {
	text, _ := g.ResolveInfo(rawSpan, eType, nil)
	return text
}

func (g *Graph) ResolveWithContext(rawSpan string, eType types.EntityType, contextHints []string) string {
	text, _ := g.ResolveInfo(rawSpan, eType, contextHints)
	return text
}

// ResolveInfo resolves a raw span to a synthetic replacement text and the
// persona twin's stable synthetic ID. Hints are normalized forms of nearby
// spans that may identify the same entity (e.g. an email next to a phone
// number); the first hint that resolves to an existing identity is adopted,
// so spans of the same entity always share one twin.
func (g *Graph) ResolveInfo(rawSpan string, eType types.EntityType, hints []string) (string, string) {
	g.vault.RLock()
	canonical := ""
	for _, h := range hints {
		hk := NormalizeAlias(h)
		if hk == "" {
			continue
		}
		if c, ok := g.vault.LookupAlias(hk); ok {
			canonical = c
			break
		}
		if _, ok := g.vault.LookupTwin(hk); ok {
			canonical = hk
			break
		}
	}
	g.vault.RUnlock()

	if canonical == "" {
		canonical = g.resolver.Ingest(rawSpan)
	}

	g.vault.Lock()
	defer g.vault.Unlock()

	if c, ok := g.vault.LookupAlias(canonical); ok {
		canonical = c
	}
	g.vault.RegisterAlias(NormalizeAlias(rawSpan), canonical)

	twin, exists := g.vault.LookupTwin(canonical)
	if !exists {
		twin = g.mintTwin(canonical)
		g.vault.RegisterTwin(canonical, twin)
	}

	return formatFor(twin, eType), twin.SyntheticID
}

// CanonicalKey returns the normalized cluster key the alias resolver would
// assign to a raw span, without minting anything. Used by the pipeline's
// mention re-scan to know which name tokens belong to a detected person.
func (g *Graph) CanonicalKey(raw string) string {
	return g.resolver.Canonical(raw)
}

func (g *Graph) LinkAliases(aliases []string) {
	if len(aliases) < 2 {
		return
	}

	g.vault.Lock()
	defer g.vault.Unlock()

	canonical := NormalizeAlias(aliases[0])
	for _, a := range aliases[1:] {
		g.vault.RegisterAlias(NormalizeAlias(a), canonical)
	}
}

func (g *Graph) mintTwin(canonical string) *types.PersonaTwin {
	idx := g.vault.TwinCount() + 1
	first := syntheticFirstNames[idx%len(syntheticFirstNames)]
	last := syntheticLastNames[(idx/len(syntheticFirstNames))%len(syntheticLastNames)]

	maxCombos := len(syntheticFirstNames) * len(syntheticLastNames)
	attempts := 0
	for g.vault.NameTaken(first+" "+last) && attempts < maxCombos {
		idx++
		first = syntheticFirstNames[idx%len(syntheticFirstNames)]
		last = syntheticLastNames[(idx/len(syntheticFirstNames))%len(syntheticLastNames)]
		attempts++
	}

	name := fmt.Sprintf("%s %s", first, last)
	if g.vault.NameTaken(name) {
		// Every combination is exhausted; disambiguate with the numeric id,
		// which is unique per twin.
		name = fmt.Sprintf("%s %s %d", first, last, idx)
	}

	return &types.PersonaTwin{
		SyntheticID: fmt.Sprintf("plx_%04d", idx),
		Name:        name,
		Email:       fmt.Sprintf("%s.%s@synthetic-corp.internal", strings.ToLower(first), strings.ToLower(last)),
		Phone:       fmt.Sprintf("+1-555-%04d", 1000+idx),
		NationalID:  checksum.LuhnGenerate(fmt.Sprintf("%04d", idx), 16),
	}
}

func formatFor(twin *types.PersonaTwin, eType types.EntityType) string {
	switch eType {
	case types.Email:
		return twin.Email
	case types.Person:
		return twin.Name
	case types.Phone:
		return twin.Phone
	case types.NationalID, types.CreditCard:
		return twin.NationalID
	default:
		return twin.SyntheticID
	}
}

var syntheticFirstNames = []string{
	"Jordan", "Morgan", "Taylor", "Casey", "Riley",
	"Quinn", "Avery", "Hayden", "Rowan", "Finley",
	"Sage", "Dakota", "River", "Phoenix", "Emery",
	"Blair", "Kendall", "Reese", "Skyler", "Lennox",
}

var syntheticLastNames = []string{
	"Vance", "Mercer", "Langford", "Ashworth", "Caldwell",
	"Thornton", "Prescott", "Whitmore", "Ellison", "Hargrove",
	"Sinclair", "Drummond", "Fairchild", "Pemberton", "Lockwood",
	"Westbrook", "Aldridge", "Holbrook", "Kingsley", "Ravenscroft",
}
