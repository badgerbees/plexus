package bench

func GenerateTestSuite() []TestCase {
	return []TestCase{
		{
			ID:    "slack-multi-mention",
			Input: "Hey @alex_rivera, can you ping Dr. Chen about the kubernetes deployment? My email is alex.rivera@acmecorp.com and my phone is +1-555-234-5678.",
			Annotations: []Annotation{
				{Text: "alex_rivera", Type: "PERSON", Start: 5, End: 16, EntityID: "person-alex"},
				{Text: "Dr. Chen", Type: "PERSON", Start: 31, End: 39, EntityID: "person-chen"},
				{Text: "kubernetes", Type: "DECOY", Start: 50, End: 60, IsDecoy: true},
				{Text: "alex.rivera@acmecorp.com", Type: "EMAIL", Start: 78, End: 102, EntityID: "person-alex"},
				{Text: "+1-555-234-5678", Type: "PHONE_NUMBER", Start: 121, End: 136, EntityID: "person-alex"},
			},
		},
		{
			ID:    "cross-ref-alias",
			Input: "A. Rivera filed the bug. Alex Rivera will fix it. CC alex.rivera@acmecorp.com.",
			Annotations: []Annotation{
				{Text: "A. Rivera", Type: "PERSON", Start: 0, End: 9, EntityID: "person-alex"},
				{Text: "Alex Rivera", Type: "PERSON", Start: 25, End: 36, EntityID: "person-alex"},
				{Text: "alex.rivera@acmecorp.com", Type: "EMAIL", Start: 53, End: 77, EntityID: "person-alex"},
			},
		},
		{
			ID:    "decoy-heavy",
			Input: "The Apollo project uses Kubernetes and Postgres. Jordan from DevOps deployed to staging. Contact jordan.lee@internal.io.",
			Annotations: []Annotation{
				{Text: "Apollo", Type: "DECOY", Start: 4, End: 10, IsDecoy: true},
				{Text: "Kubernetes", Type: "DECOY", Start: 23, End: 33, IsDecoy: true},
				{Text: "Postgres", Type: "DECOY", Start: 38, End: 46, IsDecoy: true},
				{Text: "Jordan", Type: "PERSON", Start: 48, End: 54, EntityID: "person-jordan"},
				{Text: "DevOps", Type: "DECOY", Start: 60, End: 66, IsDecoy: true},
				{Text: "staging", Type: "DECOY", Start: 79, End: 86, IsDecoy: true},
				{Text: "jordan.lee@internal.io", Type: "EMAIL", Start: 97, End: 119, EntityID: "person-jordan"},
			},
		},
		{
			ID:    "code-block-protection",
			Input: "```\nSELECT * FROM users WHERE name = 'Alex Rivera';\n```\nAlex Rivera said the query works.",
			Annotations: []Annotation{
				{Text: "Alex Rivera", Type: "PERSON", Start: 39, End: 50, EntityID: "person-alex"},
				{Text: "Alex Rivera", Type: "PERSON", Start: 56, End: 67, EntityID: "person-alex"},
			},
		},
		{
			ID:    "credit-card-format",
			Input: "Card on file: 4532015112830366. Please update billing for Alex Rivera.",
			Annotations: []Annotation{
				{Text: "4532015112830366", Type: "CREDIT_CARD", Start: 14, End: 30, EntityID: "card-1"},
				{Text: "Alex Rivera", Type: "PERSON", Start: 57, End: 68, EntityID: "person-alex"},
			},
		},
		{
			ID:    "multi-person-separation",
			Input: "Alex Rivera and Wei Chen both reviewed the PR. Alex approved, Wei requested changes.",
			Annotations: []Annotation{
				{Text: "Alex Rivera", Type: "PERSON", Start: 0, End: 11, EntityID: "person-alex"},
				{Text: "Wei Chen", Type: "PERSON", Start: 16, End: 24, EntityID: "person-chen"},
				{Text: "Alex", Type: "PERSON", Start: 47, End: 51, EntityID: "person-alex"},
				{Text: "Wei", Type: "PERSON", Start: 62, End: 65, EntityID: "person-chen"},
			},
		},
		{
			ID:    "api-key-detection",
			Input: "Set the env: export STRIPE_KEY=pk_test_abc123def456ghi789jkl012mno345pqr678. Ask Morgan about the config.",
			Annotations: []Annotation{
				{Text: "pk_test_abc123def456ghi789jkl012mno345pqr678", Type: "API_KEY", Start: 31, End: 75, EntityID: "key-1"},
				{Text: "Morgan", Type: "PERSON", Start: 81, End: 87, EntityID: "person-morgan"},
			},
		},
		{
			ID:    "email-threading",
			Input: "From: chen.wei@acmecorp.com\nTo: alex.rivera@acmecorp.com\nCC: jordan.lee@acmecorp.com\n\nHi Alex, Dr. Chen here. Jordan will join.",
			Annotations: []Annotation{
				{Text: "chen.wei@acmecorp.com", Type: "EMAIL", Start: 6, End: 27, EntityID: "person-chen"},
				{Text: "alex.rivera@acmecorp.com", Type: "EMAIL", Start: 32, End: 56, EntityID: "person-alex"},
				{Text: "jordan.lee@acmecorp.com", Type: "EMAIL", Start: 61, End: 84, EntityID: "person-jordan"},
				{Text: "Alex", Type: "PERSON", Start: 89, End: 93, EntityID: "person-alex"},
				{Text: "Dr. Chen", Type: "PERSON", Start: 95, End: 103, EntityID: "person-chen"},
				{Text: "Jordan", Type: "PERSON", Start: 110, End: 116, EntityID: "person-jordan"},
			},
		},
	}
}
