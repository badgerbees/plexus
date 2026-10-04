package router

import (
	"strings"
	"unicode"
)

var defaultDecoys = []string{
	"apollo", "kubernetes", "docker", "postgres", "redis",
	"nginx", "linux", "darwin", "webpack", "vite",
	"graphql", "grpc", "kafka", "rabbitmq", "elasticsearch",
	"terraform", "ansible", "jenkins", "circleci", "github",
	"gitlab", "bitbucket", "jira", "confluence", "slack",
	"staging", "production", "canary", "nightly", "alpha",
	"beta", "sandbox", "localhost", "npm", "yarn", "pnpm",
	"react", "angular", "vue", "svelte", "nextjs",
	"typescript", "javascript", "golang", "python", "rust",
	"devops", "pr", "datadog", "sentry", "airflow",
}

type DecoyLexicon struct {
	entries map[string]struct{}
}

func NewDecoyLexicon() *DecoyLexicon {
	d := &DecoyLexicon{entries: make(map[string]struct{}, len(defaultDecoys))}
	for _, term := range defaultDecoys {
		d.entries[strings.ToLower(term)] = struct{}{}
	}
	return d
}

func (d *DecoyLexicon) Add(terms ...string) {
	for _, t := range terms {
		d.entries[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}
}

func (d *DecoyLexicon) IsDecoy(term string) bool {
	_, found := d.entries[strings.ToLower(strings.TrimSpace(term))]
	return found
}

// ContainsToken reports whether any whitespace/punctuation-delimited token
// of a phrase is in the lexicon. Used for multi-word NER spans like
// "Apollo project" that exact-string matching would miss.
func (d *DecoyLexicon) ContainsToken(phrase string) bool {
	for _, tok := range strings.FieldsFunc(phrase, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if _, found := d.entries[strings.ToLower(tok)]; found {
			return true
		}
	}
	return false
}
