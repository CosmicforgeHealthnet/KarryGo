package taxonomy

import (
	"fmt"
	"strings"
)

// SeedAuthorityVocabularyDefinition exposes vocabulary owned by another
// authoritative Vendor Service domain without copying that domain's truth.
//
// Example: Section 12 owns PAINT and its conversion/calibration semantics.
// Section 13 may make "Paint" discoverable, but cannot redefine its quantity.
type SeedAuthorityVocabularyDefinition struct {
	AuthorityType string         `json:"authority_type"`
	AuthorityRef  string         `json:"authority_ref"`
	AuthorityCode string         `json:"authority_code"`
	Term          string         `json:"term"`
	Locale        string         `json:"locale"`
	Region        string         `json:"region,omitempty"`
	Metadata      map[string]any `json:"metadata"`
}

func (s SeedAuthorityVocabularyDefinition) NormalizedTerm() string {
	return NormalizeVocabularyTerm(s.Term)
}

func (b SeedBundle) validateAuthorityVocabulary() error {
	seenIdentity := make(map[string]struct{}, len(b.AuthorityVocabulary))
	seenTerm := make(map[string]struct{}, len(b.AuthorityVocabulary))

	for _, item := range b.AuthorityVocabulary {
		authorityType := strings.TrimSpace(item.AuthorityType)
		authorityRef := strings.TrimSpace(item.AuthorityRef)
		authorityCode := strings.TrimSpace(item.AuthorityCode)
		term := item.NormalizedTerm()
		locale := strings.ToLower(strings.TrimSpace(item.Locale))

		if authorityType == "" || authorityRef == "" || authorityCode == "" || term == "" {
			return fmt.Errorf("authority vocabulary type/ref/code/term is required")
		}
		if locale == "" {
			return fmt.Errorf("authority vocabulary locale is required")
		}

		if authorityType == "SECTION12_MEASUREMENT" && b.NamespaceCode != NamespaceShop {
			return fmt.Errorf("Section 12 measurement vocabulary is SHOP scoped")
		}

		identityKey := strings.Join([]string{
			authorityType,
			authorityRef,
			authorityCode,
		}, "|")

		if _, exists := seenIdentity[identityKey]; exists {
			return fmt.Errorf("duplicate authority vocabulary identity %s", identityKey)
		}
		seenIdentity[identityKey] = struct{}{}

		termKey := strings.Join([]string{
			authorityType,
			term,
			locale,
			strings.ToLower(strings.TrimSpace(item.Region)),
		}, "|")

		if _, exists := seenTerm[termKey]; exists {
			return fmt.Errorf("duplicate authority vocabulary term %s", term)
		}
		seenTerm[termKey] = struct{}{}
	}

	return nil
}
