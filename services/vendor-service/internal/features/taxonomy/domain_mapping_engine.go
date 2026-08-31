package taxonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type MappingCandidate struct {
	NodeCode string `json:"node_code"`

	MappingType MappingType `json:"mapping_type"`

	ConfidenceBasis MappingConfidenceBasis `json:"confidence_basis"`

	ConfidenceScore *int `json:"confidence_score,omitempty"`

	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type ReplaceFacetMappingsInput struct {
	NamespaceCode NamespaceCode `json:"namespace_code"`

	FacetCode string `json:"facet_code"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	BusinessRef *string `json:"business_ref,omitempty"`
	BranchRef   *string `json:"branch_ref,omitempty"`

	Mappings []MappingCandidate `json:"mappings"`

	Actor Actor `json:"-"`
}

type ReplaceFacetMappingsResult struct {
	NamespaceCode NamespaceCode `json:"namespace_code"`

	NamespaceID string `json:"namespace_id"`
	VersionID   string `json:"version_id"`

	FacetID   string `json:"facet_id"`
	FacetCode string `json:"facet_code"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	MappingIDs []string `json:"mapping_ids"`

	Replayed bool `json:"replayed"`
}

type normalizedReplaceFacetMappingsInput struct {
	NamespaceCode NamespaceCode `json:"namespace_code"`

	FacetCode string `json:"facet_code"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	BusinessRef string `json:"business_ref,omitempty"`
	BranchRef   string `json:"branch_ref,omitempty"`

	Mappings []MappingCandidate `json:"mappings"`
}

func normalizeMappingObjectJSON(
	raw json.RawMessage,
) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var object map[string]any

	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, ErrInvalidArgument
	}

	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}

	return canonical, nil
}

func prepareReplaceFacetMappingsInput(
	input ReplaceFacetMappingsInput,
) (
	normalizedReplaceFacetMappingsInput,
	string,
	error,
) {
	if input.NamespaceCode != NamespaceFood &&
		input.NamespaceCode != NamespaceShop {
		return normalizedReplaceFacetMappingsInput{},
			"",
			ErrInvalidArgument
	}

	facetCode := strings.ToUpper(
		strings.TrimSpace(input.FacetCode),
	)

	sourceType := strings.ToUpper(
		strings.TrimSpace(input.SourceType),
	)

	sourceRef := strings.TrimSpace(
		input.SourceRef,
	)

	if facetCode == "" ||
		sourceType == "" ||
		sourceRef == "" {
		return normalizedReplaceFacetMappingsInput{},
			"",
			ErrInvalidArgument
	}

	if err := validateSemanticCode(
		facetCode,
	); err != nil {
		return normalizedReplaceFacetMappingsInput{},
			"",
			err
	}

	normalized := normalizedReplaceFacetMappingsInput{
		NamespaceCode: input.NamespaceCode,
		FacetCode:     facetCode,
		SourceType:    sourceType,
		SourceRef:     sourceRef,
		BusinessRef:   valueOrEmpty(input.BusinessRef),
		BranchRef:     valueOrEmpty(input.BranchRef),
		Mappings:      make([]MappingCandidate, 0, len(input.Mappings)),
	}

	seenNodes := make(map[string]struct{})
	primaryCount := 0

	for _, candidate := range input.Mappings {
		nodeCode := strings.ToUpper(
			strings.TrimSpace(candidate.NodeCode),
		)

		if err := validateSemanticCode(
			nodeCode,
		); err != nil {
			return normalizedReplaceFacetMappingsInput{},
				"",
				err
		}

		if _, exists := seenNodes[nodeCode]; exists {
			return normalizedReplaceFacetMappingsInput{},
				"",
				fmt.Errorf(
					"%w: duplicate taxonomy node %s",
					ErrInvalidArgument,
					nodeCode,
				)
		}

		seenNodes[nodeCode] = struct{}{}

		switch candidate.MappingType {
		case MappingTypePrimary:
			primaryCount++

		case MappingTypeSecondary,
			MappingTypeDiscovery:

		default:
			return normalizedReplaceFacetMappingsInput{},
				"",
				ErrInvalidArgument
		}

		if primaryCount > 1 {
			return normalizedReplaceFacetMappingsInput{},
				"",
				fmt.Errorf(
					"%w: only one PRIMARY mapping is allowed per source/facet",
					ErrInvalidArgument,
				)
		}

		basis := candidate.ConfidenceBasis
		if basis == "" {
			basis = MappingConfidenceManual
		}

		switch basis {
		case MappingConfidenceManual,
			MappingConfidenceRule,
			MappingConfidenceImport,
			MappingConfidenceSystem:

		default:
			return normalizedReplaceFacetMappingsInput{},
				"",
				ErrInvalidArgument
		}

		if candidate.ConfidenceScore != nil &&
			(*candidate.ConfidenceScore < 0 ||
				*candidate.ConfidenceScore > 10000) {
			return normalizedReplaceFacetMappingsInput{},
				"",
				ErrInvalidArgument
		}

		metadata, err :=
			normalizeMappingObjectJSON(
				candidate.Metadata,
			)
		if err != nil {
			return normalizedReplaceFacetMappingsInput{},
				"",
				err
		}

		normalized.Mappings = append(
			normalized.Mappings,
			MappingCandidate{
				NodeCode:        nodeCode,
				MappingType:     candidate.MappingType,
				ConfidenceBasis: basis,
				ConfidenceScore: candidate.ConfidenceScore,
				Metadata:        metadata,
			},
		)
	}

	sort.Slice(
		normalized.Mappings,
		func(i, j int) bool {
			if normalized.Mappings[i].NodeCode !=
				normalized.Mappings[j].NodeCode {
				return normalized.Mappings[i].NodeCode <
					normalized.Mappings[j].NodeCode
			}

			return normalized.Mappings[i].MappingType <
				normalized.Mappings[j].MappingType
		},
	)

	payload, err := json.Marshal(normalized)
	if err != nil {
		return normalizedReplaceFacetMappingsInput{},
			"",
			err
	}

	sum := sha256.Sum256(payload)

	hash :=
		hex.EncodeToString(sum[:])

	return normalized, hash, nil
}
