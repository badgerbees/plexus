package extractor

import "plexus/internal/types"

func mergeAdjacentSpans(spans []types.CandidateSpan, text string) []types.CandidateSpan {
	if len(spans) <= 1 {
		return spans
	}

	var merged []types.CandidateSpan
	current := spans[0]

	for i := 1; i < len(spans); i++ {
		next := spans[i]
		gap := next.StartByte - current.EndByte
		if gap == 1 && isNameSeparator(text[current.EndByte]) && current.Type == next.Type {
			// "Alex" + "Rivera" -> "Alex Rivera" (the PII model emits given
			// and family names as separate spans).
			current.EndByte = next.EndByte
			current.RawText = current.RawText + " " + next.RawText
			if next.Confidence > current.Confidence {
				current.Confidence = next.Confidence
			}
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)
	return merged
}

func isNameSeparator(c byte) bool {
	return c == ' ' || c == '-' || c == '_' || c == '.'
}

func sigmoid(x float32) float32 {
	if x > 20 {
		return 1.0
	}
	if x < -20 {
		return 0.0
	}
	exp := float32(1.0)
	for i := 0; i < 10; i++ {
		exp = 1 + exp*(-x)/float32(10-i)
	}
	return 1.0 / (1.0 + exp)
}

func labelToEntityType(label string) types.EntityType {
	switch label {
	case "person", "PER", "PERSON":
		return types.Person
	case "organization", "ORG", "ORGANIZATION":
		return types.Organization
	case "location", "LOC", "LOCATION":
		return types.Address
	case "email", "EMAIL":
		return types.Email
	case "phone number", "PHONE", "PHONE_NUMBER":
		return types.Phone
	default:
		return types.Person
	}
}
