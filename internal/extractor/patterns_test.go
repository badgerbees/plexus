package extractor

import (
	"testing"

	"plexus/internal/types"
)

// Regression test for the phone regex: RE2's \b fails before a leading "+"
// (a non-word character), so the old pattern matched "1-555-234-5678" and
// left the "+" behind, corrupting the replacement. The number must now be
// captured with its leading "+".
func TestScan_PhoneKeepsPlusPrefix(t *testing.T) {
	spans := NewPatternMatcher().Scan("reach me at +1-555-234-5678 tomorrow", "f")
	for _, s := range spans {
		if s.Type == types.Phone && s.RawText == "+1-555-234-5678" {
			return
		}
	}
	t.Fatal("phone span with leading + not detected")
}

func TestScan_PhoneWithoutPlus(t *testing.T) {
	spans := NewPatternMatcher().Scan("call 555-234-5678 now", "f")
	for _, s := range spans {
		if s.Type == types.Phone && s.RawText == "555-234-5678" {
			return
		}
	}
	t.Fatal("plain 10-digit phone not detected")
}

// Regression test: BERT tokenizes @-mentions away, so the Go matcher must
// catch "@alex_rivera" as a person alias itself.
func TestScan_AtMentionDetected(t *testing.T) {
	spans := NewPatternMatcher().Scan("hey @alex_rivera can you take a look", "f")
	for _, s := range spans {
		if s.Type == types.Person && s.RawText == "alex_rivera" {
			return
		}
	}
	t.Fatal("@mention not detected as Person span")
}

func TestScan_AtMentionIgnoresEmail(t *testing.T) {
	spans := NewPatternMatcher().Scan("mail me at alex.rivera@acmecorp.com thanks", "f")
	for _, s := range spans {
		if s.Type == types.Person {
			t.Fatalf("email address must not produce a Person span: %q", s.RawText)
		}
	}
}

// Regression test: a Luhn-valid 16-digit card matched both the CreditCard and
// NationalID patterns, producing two overlapping replacements. After
// deduplication only the CreditCard span should remain.
func TestDedupeOverlaps_CardBeatsNationalID(t *testing.T) {
	spans := []types.CandidateSpan{
		{RawText: "4532015112830366", Type: types.CreditCard, StartByte: 0, EndByte: 16},
		{RawText: "4532015112830366", Type: types.NationalID, StartByte: 0, EndByte: 16},
	}
	kept := dedupeOverlaps(spans)
	if len(kept) != 1 {
		t.Fatalf("dedupeOverlaps kept %d spans, want 1", len(kept))
	}
	if kept[0].Type != types.CreditCard {
		t.Fatalf("dedupeOverlaps kept %s, want CreditCard", kept[0].Type)
	}
}

func TestScan_CardIsCreditCard(t *testing.T) {
	spans := NewPatternMatcher().Scan("charge 4532015112830366 now", "f")
	kept := dedupeOverlaps(spans)
	for _, s := range kept {
		if s.RawText == "4532015112830366" {
			if s.Type != types.CreditCard {
				t.Fatalf("16-digit Luhn-valid number classified as %s, want CreditCard", s.Type)
			}
			return
		}
	}
	t.Fatal("credit card number not detected")
}

func TestScan_SlackMentionDetected(t *testing.T) {
	spans := NewPatternMatcher().Scan("loop in <@U02TYSON_BEAUMONT> for review", "f")
	for _, s := range spans {
		if s.Type == types.Person && s.RawText == "U02TYSON_BEAUMONT" {
			return
		}
	}
	t.Fatal("slack <@U02...> mention not detected as Person")
}

func TestScan_EmployeeID(t *testing.T) {
	spans := NewPatternMatcher().Scan("emp CMG-204817 and JD118440 here", "f")
	found := map[string]bool{}
	for _, s := range spans {
		if s.Type == types.EmployeeID {
			found[s.RawText] = true
		}
	}
	for _, want := range []string{"CMG-204817", "JD118440"} {
		if !found[want] {
			t.Errorf("employee id %q not detected (found: %v)", want, found)
		}
	}
}

func TestScan_EmployeeIDRejectsDates(t *testing.T) {
	spans := NewPatternMatcher().Scan("scheduled Jan 2026 for the rollout", "f")
	for _, s := range spans {
		if s.Type == types.EmployeeID {
			t.Fatalf("date %q misclassified as EmployeeID", s.RawText)
		}
	}
}

func TestScan_AccountNumber(t *testing.T) {
	spans := NewPatternMatcher().Scan("acct 5583-1188 ref 481-002917-901 ok", "f")
	found := map[string]bool{}
	for _, s := range spans {
		if s.Type == types.AccountNumber {
			found[s.RawText] = true
		}
	}
	for _, want := range []string{"5583-1188", "481-002917-901"} {
		if !found[want] {
			t.Errorf("account number %q not detected (found: %v)", want, found)
		}
	}
}

func TestScan_AccountNumberRejectsDate(t *testing.T) {
	spans := NewPatternMatcher().Scan("meeting on 2026-01-15 with the team", "f")
	for _, s := range spans {
		if s.Type == types.AccountNumber {
			t.Fatalf("date %q misclassified as AccountNumber", s.RawText)
		}
	}
}

func TestScan_URL(t *testing.T) {
	spans := NewPatternMatcher().Scan("see https://api.bswift.com/v3/eligibility for details", "f")
	for _, s := range spans {
		if s.Type == types.URL {
			return
		}
	}
	t.Fatal("URL not detected")
}

func TestMergeAdjacentSpans_JoinsGivenFamily(t *testing.T) {
	text := "Alex Rivera approved"
	spans := []types.CandidateSpan{
		{RawText: "Alex", Type: types.Person, StartByte: 0, EndByte: 4},
		{RawText: "Rivera", Type: types.Person, StartByte: 5, EndByte: 11},
	}
	merged := mergeAdjacentSpans(spans, text)
	if len(merged) != 1 || merged[0].RawText != "Alex Rivera" {
		t.Fatalf("given+family should merge: %+v", merged)
	}
}

func TestMergeAdjacentSpans_DoesNotJoinAcrossComma(t *testing.T) {
	text := "Alex,Wei"
	spans := []types.CandidateSpan{
		{RawText: "Alex", Type: types.Person, StartByte: 0, EndByte: 4},
		{RawText: "Wei", Type: types.Person, StartByte: 5, EndByte: 8},
	}
	merged := mergeAdjacentSpans(spans, text)
	if len(merged) != 2 {
		t.Fatalf("comma-separated names must not merge: %+v", merged)
	}
}

func TestLexiconScan_SurnameAfterGiven(t *testing.T) {
	text := "Jordan Kemp will join"
	spans := scanLexiconSpans(text, nil)
	found := false
	for _, s := range spans {
		if s.Type == types.Person && s.RawText == "Kemp" {
			found = true
		}
	}
	if !found {
		t.Fatalf("surname after given name not detected: %+v", spans)
	}
}

func TestLexiconScan_SurnameNotAfterLowercase(t *testing.T) {
	text := "the kemp building"
	spans := scanLexiconSpans(text, nil)
	for _, s := range spans {
		if s.Type == types.Person {
			t.Fatalf("lowercase surname after lowercase word must not match: %+v", spans)
		}
	}
}

func TestLexiconScan_Organization(t *testing.T) {
	text := "we deployed on Stripe and reached out to Procter & Gamble"
	spans := scanLexiconSpans(text, nil)
	found := map[string]bool{}
	for _, s := range spans {
		if s.Type == types.Organization {
			found[s.RawText] = true
		}
	}
	if !found["Stripe"] {
		t.Fatalf("single-word org not detected: %+v", spans)
	}
	if !found["Procter & Gamble"] {
		t.Fatalf("multi-word org not detected: %+v", spans)
	}
}

func TestLexiconScan_LowercaseOrgNotMatched(t *testing.T) {
	text := "we talked to stripe about the deal"
	spans := scanLexiconSpans(text, nil)
	for _, s := range spans {
		if s.Type == types.Organization {
			t.Fatalf("lowercase single-word org must not match: %+v", spans)
		}
	}
}

func TestLexiconScan_SkipsCovered(t *testing.T) {
	text := "Alex Rivera and Kemp here"
	covered := []types.CandidateSpan{{StartByte: 0, EndByte: 11, Type: types.Person}}
	spans := scanLexiconSpans(text, covered)
	for _, s := range spans {
		if s.StartByte < 11 && s.EndByte > 0 {
			t.Fatalf("covered span must be skipped: %+v", spans)
		}
	}
}
