package taxonomy

import "strings"

// TermRelationshipType describes how a lexical term relates to a canonical node.
// ALIAS, SYNONYM and RELATED_TERM intentionally remain distinct semantics.
type TermRelationshipType string

const (
	TermRelationshipAlias       TermRelationshipType = "ALIAS"
	TermRelationshipSynonym     TermRelationshipType = "SYNONYM"
	TermRelationshipRelatedTerm TermRelationshipType = "RELATED_TERM"
)

func (r TermRelationshipType) Valid() bool {
	switch r {
	case TermRelationshipAlias, TermRelationshipSynonym, TermRelationshipRelatedTerm:
		return true
	default:
		return false
	}
}

type TaxonomyNodeTerm struct {
	ID               string
	NamespaceID      string
	VersionID        string
	NodeID           string
	Term             string
	NormalizedTerm   string
	RelationshipType TermRelationshipType
	Locale           string
	Region           string
	Status           string
	Metadata         map[string]any
	Revision         int64
}

// NormalizeVocabularyTerm performs deterministic lexical normalization.
// Punctuation is intentionally preserved so explicit aliases such as
// PUFF-PUFF and PUFF PUFF can remain separately configured vocabulary.
func NormalizeVocabularyTerm(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
