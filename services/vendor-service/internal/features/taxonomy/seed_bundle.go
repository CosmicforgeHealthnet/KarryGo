package taxonomy

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// SeedFacetDefinition describes taxonomy facet data without introducing
// Go enums for business vocabulary.
type SeedFacetDefinition struct {
	Code          string             `json:"code"`
	DisplayName   string             `json:"display_name"`
	Description   string             `json:"description"`
	StructureType FacetStructureType `json:"structure_type"`
	Position      int                `json:"position"`
	Metadata      map[string]any     `json:"metadata"`
}

// SeedNodeDefinition describes one canonical taxonomy node.
// ParentCode is another canonical code in the same facet.
type SeedNodeDefinition struct {
	CanonicalCode string         `json:"canonical_code"`
	FacetCode     string         `json:"facet_code"`
	DisplayName   string         `json:"display_name"`
	Description   string         `json:"description"`
	ParentCode    string         `json:"parent_code,omitempty"`
	Position      int            `json:"position"`
	Metadata      map[string]any `json:"metadata"`
}

// SeedBundle is a transport-neutral taxonomy seed definition.
// The engine consumes data through this structure; vocabulary additions
// therefore do not require new Go enum values.
type SeedBundle struct {
	SchemaVersion       int                                 `json:"schema_version"`
	NamespaceCode       NamespaceCode                       `json:"namespace_code"`
	SeedKey             string                              `json:"seed_key"`
	Facets              []SeedFacetDefinition               `json:"facets"`
	Nodes               []SeedNodeDefinition                `json:"nodes"`
	Terms               []SeedNodeTerm                      `json:"terms,omitempty"`
	AuthorityVocabulary []SeedAuthorityVocabularyDefinition `json:"authority_vocabulary,omitempty"`
}

func DecodeSeedBundle(reader io.Reader) (SeedBundle, error) {
	if reader == nil {
		return SeedBundle{}, ErrInvalidArgument
	}

	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var bundle SeedBundle
	if err := decoder.Decode(&bundle); err != nil {
		return SeedBundle{}, fmt.Errorf("decode taxonomy seed: %w", err)
	}

	if err := bundle.Validate(); err != nil {
		return SeedBundle{}, err
	}

	return bundle, nil
}

func (b SeedBundle) Validate() error {
	if b.SchemaVersion != 1 {
		return fmt.Errorf("unsupported taxonomy seed schema version %d", b.SchemaVersion)
	}

	switch b.NamespaceCode {
	case NamespaceFood, NamespaceShop:
	default:
		return fmt.Errorf("unsupported taxonomy seed namespace %q", b.NamespaceCode)
	}

	if strings.TrimSpace(b.SeedKey) == "" {
		return fmt.Errorf("taxonomy seed key is required")
	}

	if err := b.validateAuthorityVocabulary(); err != nil {
		return err
	}

	if err := b.validateSeedTerms(); err != nil {
		return err
	}

	if len(b.Facets) == 0 {
		return fmt.Errorf("taxonomy seed requires at least one facet")
	}

	facetByCode := make(map[string]SeedFacetDefinition, len(b.Facets))
	for _, facet := range b.Facets {
		code := strings.TrimSpace(facet.Code)
		if code == "" || strings.TrimSpace(facet.DisplayName) == "" {
			return fmt.Errorf("taxonomy seed facet code/display name is required")
		}
		if facet.Position < 0 {
			return fmt.Errorf("taxonomy seed facet %s has negative position", code)
		}
		if facet.StructureType != FacetStructureFlat &&
			facet.StructureType != FacetStructureHierarchical {
			return fmt.Errorf("taxonomy seed facet %s has invalid structure %q", code, facet.StructureType)
		}
		if _, exists := facetByCode[code]; exists {
			return fmt.Errorf("duplicate taxonomy seed facet %s", code)
		}
		facetByCode[code] = facet
	}

	prefix := string(b.NamespaceCode) + "_"
	nodeByCode := make(map[string]SeedNodeDefinition, len(b.Nodes))

	for _, node := range b.Nodes {
		code := strings.TrimSpace(node.CanonicalCode)
		facetCode := strings.TrimSpace(node.FacetCode)

		if code == "" || facetCode == "" || strings.TrimSpace(node.DisplayName) == "" {
			return fmt.Errorf("taxonomy seed node code/facet/display name is required")
		}
		if !strings.HasPrefix(code, prefix) {
			return fmt.Errorf("taxonomy seed node %s is outside namespace %s", code, b.NamespaceCode)
		}
		if node.Position < 0 {
			return fmt.Errorf("taxonomy seed node %s has negative position", code)
		}
		if _, exists := facetByCode[facetCode]; !exists {
			return fmt.Errorf("taxonomy seed node %s references unknown facet %s", code, facetCode)
		}
		if _, exists := nodeByCode[code]; exists {
			return fmt.Errorf("duplicate taxonomy seed node %s", code)
		}

		nodeByCode[code] = node
	}

	for _, node := range b.Nodes {
		if strings.TrimSpace(node.ParentCode) == "" {
			continue
		}

		if node.ParentCode == node.CanonicalCode {
			return fmt.Errorf("taxonomy seed node %s cannot parent itself", node.CanonicalCode)
		}

		parent, exists := nodeByCode[node.ParentCode]
		if !exists {
			return fmt.Errorf("taxonomy seed node %s references missing parent %s", node.CanonicalCode, node.ParentCode)
		}

		if parent.FacetCode != node.FacetCode {
			return fmt.Errorf("taxonomy seed node %s parent belongs to different facet", node.CanonicalCode)
		}

		if facetByCode[node.FacetCode].StructureType != FacetStructureHierarchical {
			return fmt.Errorf("flat taxonomy seed facet %s cannot contain parented nodes", node.FacetCode)
		}
	}

	state := make(map[string]uint8, len(nodeByCode))
	var visit func(string) error
	visit = func(code string) error {
		switch state[code] {
		case 1:
			return fmt.Errorf("taxonomy seed hierarchy cycle detected at %s", code)
		case 2:
			return nil
		}

		state[code] = 1
		node := nodeByCode[code]
		if node.ParentCode != "" {
			if err := visit(node.ParentCode); err != nil {
				return err
			}
		}
		state[code] = 2
		return nil
	}

	for code := range nodeByCode {
		if err := visit(code); err != nil {
			return err
		}
	}

	return nil
}
