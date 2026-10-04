package graph

import (
	"fmt"
	"testing"

	"plexus/internal/types"
)

// Regression test for the mintTwin infinite loop: first and last names were
// both indexed by the same `idx%20`, producing only 20 diagonal pairs, so
// minting a 21st twin never found an untaken name and hung forever.
func TestGraph_MintsUniqueNamesBeyondTwenty(t *testing.T) {
	g := New()

	seenNames := make(map[string]bool)
	seenIDs := make(map[string]bool)
	for i := 0; i < 60; i++ {
		raw := fmt.Sprintf("user%d@example.com", i)
		_, id := g.ResolveInfo(raw, types.Email, nil)
		if id == "" {
			t.Fatalf("empty synthetic id for doc %d", i)
		}

		twin, ok := g.vault.LookupTwin(NormalizeAlias(raw))
		if !ok {
			t.Fatalf("twin not registered for %q", raw)
		}
		if seenNames[twin.Name] {
			t.Errorf("duplicate synthetic name %q", twin.Name)
		}
		if seenIDs[twin.SyntheticID] {
			t.Errorf("duplicate synthetic id %q", twin.SyntheticID)
		}
		seenNames[twin.Name] = true
		seenIDs[twin.SyntheticID] = true
	}

	if got := g.vault.TwinCount(); got != 60 {
		t.Fatalf("TwinCount = %d, want 60", got)
	}
}

func TestGraph_TitleAndEmailShareTwin(t *testing.T) {
	g := New()
	_, emailID := g.ResolveInfo("chen.wei@acmecorp.com", types.Email, nil)
	_, nameID := g.ResolveInfo("Dr. Chen", types.Person, nil)
	if emailID != nameID {
		t.Fatalf("Dr. Chen should share a twin with chen.wei@acmecorp.com: got %q vs %q", nameID, emailID)
	}
}

func TestGraph_PhoneAdoptsNearbyEmailTwin(t *testing.T) {
	g := New()
	_, emailID := g.ResolveInfo("alex.rivera@acmecorp.com", types.Email, nil)
	_, phoneID := g.ResolveInfo("+1-555-234-5678", types.Phone, []string{"alex.rivera@acmecorp.com"})
	if emailID != phoneID {
		t.Fatalf("phone should adopt the twin of the nearby email: got %q, want %q", phoneID, emailID)
	}
}

func TestGraph_InitialsHandleAndEmailCluster(t *testing.T) {
	g := New()
	_, a := g.ResolveInfo("Alex Rivera", types.Person, nil)
	_, b := g.ResolveInfo("A. Rivera", types.Person, nil)
	_, c := g.ResolveInfo("@alex_rivera", types.Person, nil)
	_, d := g.ResolveInfo("alex.rivera@acmecorp.com", types.Email, nil)
	if a != b || a != c || a != d {
		t.Fatalf("aliases of the same person should share a twin: %q %q %q %q", a, b, c, d)
	}
}

func TestGraph_DifferentPeopleStaySeparate(t *testing.T) {
	g := New()
	_, a := g.ResolveInfo("Alex Rivera", types.Person, nil)
	_, b := g.ResolveInfo("Wei Chen", types.Person, nil)
	if a == b {
		t.Fatalf("different people must not share a twin: both %q", a)
	}
}

func TestNormalizeAlias_StripsHonorifics(t *testing.T) {
	cases := map[string]string{
		"Dr. Chen":  "chen",
		"dr chen":   "chen",
		"Mr Smith":  "smith",
		"Ms Rivera": "rivera",
		"Wei Chen":  "wei chen",
	}
	for in, want := range cases {
		if got := NormalizeAlias(in); got != want {
			t.Errorf("NormalizeAlias(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAlias_EmailUsesLocalPart(t *testing.T) {
	cases := map[string]string{
		"alex.rivera@acmecorp.com": "alex rivera",
		"chen.wei@pfizer.com":      "chen wei",
		"jordan_lee@internal.io":   "jordan lee",
	}
	for in, want := range cases {
		if got := NormalizeAlias(in); got != want {
			t.Errorf("NormalizeAlias(%q) = %q, want %q", in, got, want)
		}
	}
}

// Regression test: numeric tokens were treated as name initials, so "user 1"
// and "user 10" clustered into the same entity.
func TestAliasResolver_SurnameMergesWhenUnambiguous(t *testing.T) {
	ar := NewAliasResolver()
	c1 := ar.Ingest("alex.rivera@acmecorp.com")
	c2 := ar.Ingest("Rivera")
	if c1 != c2 {
		t.Fatalf("unique surname should merge into the only matching cluster: %q vs %q", c2, c1)
	}
}

func TestAliasResolver_SurnameStaysSeparateWhenAmbiguous(t *testing.T) {
	ar := NewAliasResolver()
	ar.Ingest("Alex Rivera")
	ar.Ingest("Jordan Rivera")
	c := ar.Ingest("Rivera")
	if c == "alex rivera" || c == "jordan rivera" {
		t.Fatalf("ambiguous surname must not merge: got %q", c)
	}
}

func TestFuzzyEqual_NumericTokenIsNotInitial(t *testing.T) {
	if fuzzyEqual("user 1", "user 10") {
		t.Fatal("numeric tokens must not be treated as name initials")
	}
	if fuzzyEqual("user 1", "user 1") == false {
		t.Fatal("identical keys must still match")
	}
}

func TestAliasResolver_DoesNotMergeNumericSuffix(t *testing.T) {
	ar := NewAliasResolver()
	c1 := ar.Ingest("user 1")
	c2 := ar.Ingest("user 10")
	if c1 == c2 {
		t.Fatalf("user 1 and user 10 must not cluster: both %q", c1)
	}
}

func TestAliasResolver_StillClustersLetterInitial(t *testing.T) {
	ar := NewAliasResolver()
	c1 := ar.Ingest("Alex Rivera")
	c2 := ar.Ingest("A. Rivera")
	if c1 != c2 {
		t.Fatalf("letter initial should still cluster: %q vs %q", c2, c1)
	}
}

// Regression test for the surname-collision class micro1 documented: a bare
// surname ("Chen") must NOT merge into a full name ("Wei Chen").
func TestAliasResolver_SurnameDoesNotMergeFullName(t *testing.T) {
	ar := NewAliasResolver()
	c1 := ar.Ingest("Chen")
	c2 := ar.Ingest("Wei Chen")
	if c1 == c2 {
		t.Fatalf("bare surname must not merge with full name: both %q", c1)
	}
}

// A first-name mention must still merge: "Wei" belongs to "Wei Chen".
func TestAliasResolver_FirstNameStillMerges(t *testing.T) {
	ar := NewAliasResolver()
	c1 := ar.Ingest("Wei Chen")
	c2 := ar.Ingest("Wei")
	if c1 != c2 {
		t.Fatalf("first-name mention should cluster: %q vs %q", c2, c1)
	}
}

func TestNormalizeAlias_StripsSlackWorkspacePrefix(t *testing.T) {
	cases := map[string]string{
		"U02TYSON_BEAUMONT": "tyson beaumont",
		"U02JOHN_REYES":     "john reyes",
		"tyson beaumont":    "tyson beaumont",
	}
	for in, want := range cases {
		if got := NormalizeAlias(in); got != want {
			t.Errorf("NormalizeAlias(%q) = %q, want %q", in, got, want)
		}
	}
}
