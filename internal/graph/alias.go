package graph

import (
	"strings"
	"sync"
	"unicode"

	"plexus/internal/types"
)

type Graph struct {
	vault    *types.IdentityVault
	resolver *AliasResolver
}

func New() *Graph {
	return &Graph{
		vault:    types.NewIdentityVault(),
		resolver: NewAliasResolver(),
	}
}

type AliasResolver struct {
	mu       sync.Mutex
	clusters []aliasCluster
}

type aliasCluster struct {
	canonical string
	members   []string
}

func NewAliasResolver() *AliasResolver {
	return &AliasResolver{}
}

func (ar *AliasResolver) Ingest(raw string) string {
	key := normalizeForAlias(raw)
	if key == "" {
		return raw
	}

	ar.mu.Lock()
	defer ar.mu.Unlock()

	matches := make([]int, 0, 1)
	for i, c := range ar.clusters {
		for _, member := range c.members {
			if fuzzyEqual(key, member) {
				matches = append(matches, i)
				break
			}
		}
	}

	if len(matches) == 1 {
		ar.clusters[matches[0]].members = append(ar.clusters[matches[0]].members, key)
		return ar.clusters[matches[0]].canonical
	}

	// A bare surname is ambiguous across people. Merge it only when exactly
	// one known cluster ends with it; otherwise keep it standalone so a
	// speculative merge cannot collide identities (micro1's collision class).
	if c, ok := ar.unambiguousSurname(key); ok {
		ar.clusters[c].members = append(ar.clusters[c].members, key)
		return ar.clusters[c].canonical
	}

	ar.clusters = append(ar.clusters, aliasCluster{
		canonical: key,
		members:   []string{key},
	})
	return key
}

// unambiguousSurname returns the index of the single cluster whose member's
// last token equals the single-token key, or false when zero or several
// clusters share that surname.
func (ar *AliasResolver) unambiguousSurname(key string) (int, bool) {
	parts := strings.Fields(key)
	if len(parts) != 1 || len(parts[0]) < 2 {
		return -1, false
	}
	surname := parts[0]
	found := -1
	for i, c := range ar.clusters {
		for _, member := range c.members {
			mp := strings.Fields(member)
			if len(mp) > 0 && mp[len(mp)-1] == surname {
				if found >= 0 && found != i {
					return -1, false // shared by several clusters: ambiguous
				}
				found = i
			}
		}
	}
	return found, found >= 0
}

func (ar *AliasResolver) IngestWithHints(raw string, hints []string) string {
	key := normalizeForAlias(raw)
	canonical := ar.Ingest(raw)

	for _, hint := range hints {
		hk := normalizeForAlias(hint)
		if hk == "" || hk == key {
			continue
		}
		ar.mergeInto(canonical, hk)
	}
	return canonical
}

func (ar *AliasResolver) mergeInto(canonical, newMember string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	for i, c := range ar.clusters {
		if c.canonical == canonical {
			for _, m := range c.members {
				if m == newMember {
					return
				}
			}
			ar.clusters[i].members = append(ar.clusters[i].members, newMember)
			return
		}
	}
}

func (ar *AliasResolver) Canonical(raw string) string {
	key := normalizeForAlias(raw)
	ar.mu.Lock()
	defer ar.mu.Unlock()
	for _, c := range ar.clusters {
		for _, member := range c.members {
			if fuzzyEqual(key, member) {
				return c.canonical
			}
		}
	}
	return key
}

func fuzzyEqual(a, b string) bool {
	if a == b {
		return true
	}

	ea := emailLocalPart(a)
	eb := emailLocalPart(b)

	if ea != "" && eb != "" {
		return ea == eb
	}

	// Cross-match email local part with normal name
	if ea != "" && strings.Contains(ea, strings.ReplaceAll(b, " ", "")) {
		return true
	}
	if eb != "" && strings.Contains(eb, strings.ReplaceAll(a, " ", "")) {
		return true
	}

	partsA := strings.Fields(a)
	partsB := strings.Fields(b)

	if len(partsA) == 0 || len(partsB) == 0 {
		return false
	}

	// Unsegmented form: "alex rivera" vs "alexrivera"
	if strings.Join(partsA, "") == strings.Join(partsB, "") {
		return true
	}

	if initialsMatch(partsA, partsB) || initialsMatch(partsB, partsA) {
		return true
	}

	if sharedLastName(partsA, partsB) && sharedFirstInitial(partsA, partsB) {
		return true
	}

	// A single token may match only the FIRST token of the other side
	// (a first-name mention, e.g. "Alex" vs "Alex Rivera"). It must not
	// match a surname ("Chen" vs "Wei Chen"): bare surnames are ambiguous
	// across people and merging them collides identities.
	if len(partsA) == 1 && len(partsB) > 1 {
		if partsA[0] == partsB[0] {
			return true
		}
	}
	if len(partsB) == 1 && len(partsA) > 1 {
		if partsB[0] == partsA[0] {
			return true
		}
	}

	return false
}

func initialsMatch(candidate, full []string) bool {
	if len(candidate) < 2 || len(full) < 2 {
		return false
	}

	for i, part := range candidate {
		if i >= len(full) {
			return false
		}
		if isInitialToken(part) {
			initial := unicode.ToLower(rune(part[0]))
			if unicode.ToLower(rune(full[i][0])) != initial {
				return false
			}
		} else if part != full[i] {
			return false
		}
	}
	return true
}

// isInitialToken reports whether a token is a name initial like "a" or "a.",
// as opposed to a numeric token ("1", "10") or any other single character.
// Without the letter check, "user 1" would wrongly match "user 10".
func isInitialToken(part string) bool {
	if part == "" || !unicode.IsLetter(rune(part[0])) {
		return false
	}
	if len(part) == 1 {
		return true
	}
	return len(part) == 2 && part[1] == '.'
}

func emailLocalPart(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	if idx := strings.Index(s, "@"); idx > 0 {
		return s[:idx]
	}
	return ""
}

func sharedLastName(a, b []string) bool {
	if len(a) < 2 || len(b) < 2 {
		return false
	}
	return a[len(a)-1] == b[len(b)-1]
}

func sharedFirstInitial(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	ra := rune(a[0][0])
	rb := rune(b[0][0])
	return unicode.ToLower(ra) == unicode.ToLower(rb)
}

var titlePrefixes = map[string]bool{
	"dr": true, "mr": true, "mrs": true, "ms": true, "miss": true,
	"prof": true, "mx": true, "sir": true, "madam": true, "sra": true,
}

func normalizeForAlias(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "@")

	// For email addresses the identity signal is the local part; the
	// domain only adds noise that breaks name matching.
	if idx := strings.Index(s, "@"); idx >= 0 {
		s = s[:idx]
	}

	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "-", " ")

	parts := strings.Fields(s)
	if len(parts) >= 2 && titlePrefixes[parts[0]] {
		parts = parts[1:]
	}
	for i, p := range parts {
		// Strip Slack workspace prefixes like U02 from <@U02TYSON_BEAUMONT>
		// so the handle links to the person's name cluster.
		if len(p) >= 3 && p[0] == 'u' && isHexDigit(p[1]) && isHexDigit(p[2]) {
			parts[i] = p[3:]
		}
	}
	return strings.Join(parts, " ")
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

// NormalizeAlias is the exported form of normalizeForAlias so other
// packages resolve and register alias keys with the same rules.
func NormalizeAlias(s string) string {
	return normalizeForAlias(s)
}
