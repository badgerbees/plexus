package types

import "sync"

type EntityType string

const (
	Person        EntityType = "PERSON"
	Email         EntityType = "EMAIL"
	Phone         EntityType = "PHONE_NUMBER"
	NationalID    EntityType = "NATIONAL_ID"
	CreditCard    EntityType = "CREDIT_CARD"
	Organization  EntityType = "ORGANIZATION"
	Address       EntityType = "ADDRESS"
	APIKey        EntityType = "API_KEY"
	UUID          EntityType = "UUID"
	EmployeeID    EntityType = "EMPLOYEE_ID"
	AccountNumber EntityType = "ACCOUNT_NUMBER"
	URL           EntityType = "URL"
)

type CandidateSpan struct {
	RawText    string     `json:"raw_text"`
	Type       EntityType `json:"entity_type"`
	StartByte  int        `json:"start_byte"`
	EndByte    int        `json:"end_byte"`
	Confidence float32    `json:"confidence"`
	SourceFile string     `json:"source_file"`
	Context    string     `json:"context"`
}

type PersonaTwin struct {
	SyntheticID string
	Name        string
	Email       string
	Phone       string
	NationalID  string
}

type AliasCluster struct {
	CanonicalKey string
	Aliases      []string
	Twin         *PersonaTwin
}

type IdentityVault struct {
	mu           sync.RWMutex
	sourceToTwin map[string]*PersonaTwin
	aliasToKey   map[string]string
	usedNames    map[string]bool
}

func NewIdentityVault() *IdentityVault {
	return &IdentityVault{
		sourceToTwin: make(map[string]*PersonaTwin),
		aliasToKey:   make(map[string]string),
		usedNames:    make(map[string]bool),
	}
}

func (v *IdentityVault) Lock()    { v.mu.Lock() }
func (v *IdentityVault) Unlock()  { v.mu.Unlock() }
func (v *IdentityVault) RLock()   { v.mu.RLock() }
func (v *IdentityVault) RUnlock() { v.mu.RUnlock() }

func (v *IdentityVault) LookupAlias(key string) (string, bool) {
	canonical, ok := v.aliasToKey[key]
	return canonical, ok
}

func (v *IdentityVault) LookupTwin(canonical string) (*PersonaTwin, bool) {
	twin, ok := v.sourceToTwin[canonical]
	return twin, ok
}

func (v *IdentityVault) RegisterAlias(alias, canonical string) {
	v.aliasToKey[alias] = canonical
}

func (v *IdentityVault) RegisterTwin(canonical string, twin *PersonaTwin) {
	v.sourceToTwin[canonical] = twin
	v.usedNames[twin.Name] = true
}

func (v *IdentityVault) NameTaken(name string) bool {
	return v.usedNames[name]
}

func (v *IdentityVault) TwinCount() int {
	return len(v.sourceToTwin)
}

type TriageDecision int

const (
	Accept   TriageDecision = iota // confirmed PII, route to graph
	Reject                         // confirmed decoy, suppress
	Escalate                       // ambiguous, send to LLM judge
)

type Document struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Format   string `json:"format"`
	Content  string `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Severity returns the privacy weight of an entity type, following the
// severity-weighting used in micro1's Privacy pillar: high-exposure
// identifiers (cards, accounts, credentials) weigh more than names and
// organizations.
func Severity(t EntityType) float64 {
	switch t {
	case CreditCard, NationalID, APIKey, AccountNumber:
		return 1.0
	case EmployeeID:
		return 0.9
	case Email, Phone:
		return 0.8
	case Person:
		return 0.7
	case Address:
		return 0.6
	case UUID:
		return 0.5
	case Organization:
		return 0.4
	case URL:
		return 0.3
	}
	return 0.5
}
