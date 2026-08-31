package taxonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type SeedNodeTerm struct {
	NamespaceCode    NamespaceCode        `json:"namespace_code"`
	NodeCode         string               `json:"node_code"`
	Term             string               `json:"term"`
	RelationshipType TermRelationshipType `json:"relationship_type"`
	Locale           string               `json:"locale"`
	Region           string               `json:"region,omitempty"`
	Metadata         map[string]any       `json:"metadata"`
	VersionKey       string               `json:"-"`
}

func (s SeedNodeTerm) Validate() error {
	if strings.TrimSpace(string(s.NamespaceCode)) == "" {
		return fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(s.NodeCode) == "" {
		return fmt.Errorf("node code is required")
	}
	if NormalizeVocabularyTerm(s.Term) == "" {
		return fmt.Errorf("term is required")
	}
	if !s.RelationshipType.Valid() {
		return fmt.Errorf("unsupported relationship type %q", s.RelationshipType)
	}
	return nil
}

// DeterministicTermID makes repeated seed execution converge on the same
// vocabulary identity rather than creating duplicate lexical records.
func DeterministicTermID(seed SeedNodeTerm) string {
	raw := strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(string(seed.NamespaceCode))),

		strings.TrimSpace(seed.VersionKey),
		strings.ToUpper(strings.TrimSpace(seed.NodeCode)),
		string(seed.RelationshipType),
		NormalizeVocabularyTerm(seed.Term),
		strings.ToLower(strings.TrimSpace(seed.Locale)),
		strings.ToLower(strings.TrimSpace(seed.Region)),
	}, "|")

	digest := sha256.Sum256([]byte(raw))
	return "cterm_" + hex.EncodeToString(digest[:16])
}
