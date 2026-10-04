package graph

import "testing"

func TestFuzzyEqual_ExactMatch(t *testing.T) {
	if !fuzzyEqual("alex rivera", "alex rivera") {
		t.Fatal("exact match should return true")
	}
}

func TestFuzzyEqual_Initials(t *testing.T) {
	if !fuzzyEqual("a rivera", "alex rivera") {
		t.Fatal("initials should match full name")
	}
	if !fuzzyEqual("a. rivera", "alex rivera") {
		t.Fatal("dotted initials should match full name")
	}
}

func TestFuzzyEqual_DifferentPeople(t *testing.T) {
	if fuzzyEqual("alex rivera", "alex chen") {
		t.Fatal("different last names should not match")
	}
}

func TestNormalizeForAlias(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"@alex_rivera", "alex rivera"},
		{"alex.rivera", "alex rivera"},
		{"Alex-Rivera", "alex rivera"},
		{"  Alex Rivera  ", "alex rivera"},
	}
	for _, c := range cases {
		got := normalizeForAlias(c.input)
		if got != c.want {
			t.Errorf("normalizeForAlias(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestAliasResolver_AutoClusters(t *testing.T) {
	ar := NewAliasResolver()

	c1 := ar.Ingest("Alex Rivera")
	c2 := ar.Ingest("A. Rivera")
	c3 := ar.Ingest("@alex_rivera")

	if c1 != c2 {
		t.Errorf("A. Rivera should cluster with Alex Rivera: got %q vs %q", c2, c1)
	}
	if c1 != c3 {
		t.Errorf("@alex_rivera should cluster with Alex Rivera: got %q vs %q", c3, c1)
	}
}

func TestAliasResolver_SeparatePeople(t *testing.T) {
	ar := NewAliasResolver()

	c1 := ar.Ingest("Alex Rivera")
	c2 := ar.Ingest("Dr. Wei Chen")

	if c1 == c2 {
		t.Errorf("different people should not cluster: both got %q", c1)
	}
}

func TestGraph_ConsistentResolution(t *testing.T) {
	g := New()

	r1 := g.Resolve("alex.rivera@acmecorp.com", "EMAIL")
	r2 := g.Resolve("alex.rivera@acmecorp.com", "EMAIL")

	if r1 != r2 {
		t.Errorf("same input should always resolve to same output: %q vs %q", r1, r2)
	}
}
