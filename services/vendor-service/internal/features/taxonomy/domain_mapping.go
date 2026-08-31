package taxonomy

import (
	"encoding/json"
	"strings"
	"time"
)

type MappingType string

const (
	MappingTypePrimary   MappingType = "PRIMARY"
	MappingTypeSecondary MappingType = "SECONDARY"
	MappingTypeDiscovery MappingType = "DISCOVERY"
)

type MappingStatus string

const (
	MappingStatusActive   MappingStatus = "ACTIVE"
	MappingStatusArchived MappingStatus = "ARCHIVED"
)

type MappingConfidenceBasis string

const (
	MappingConfidenceManual MappingConfidenceBasis = "MANUAL"
	MappingConfidenceRule   MappingConfidenceBasis = "RULE"
	MappingConfidenceImport MappingConfidenceBasis = "IMPORT"
	MappingConfidenceSystem MappingConfidenceBasis = "SYSTEM"
)

type TaxonomyMapping struct {
	ID string

	NamespaceID string
	VersionID   string
	NodeID      string

	SourceType string
	SourceRef  string

	MappingType MappingType
	Status      MappingStatus

	ConfidenceBasis MappingConfidenceBasis
	ConfidenceScore *int

	BusinessRef *string
	BranchRef   *string

	Metadata json.RawMessage

	Revision int64

	CreatedByActorRef string
	UpdatedByActorRef string

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m TaxonomyMapping) Validate() error {
	if strings.TrimSpace(m.NamespaceID) == "" ||
		strings.TrimSpace(m.VersionID) == "" ||
		strings.TrimSpace(m.NodeID) == "" ||
		strings.TrimSpace(m.SourceType) == "" ||
		strings.TrimSpace(m.SourceRef) == "" {
		return ErrInvalidArgument
	}

	switch m.MappingType {
	case MappingTypePrimary,
		MappingTypeSecondary,
		MappingTypeDiscovery:
	default:
		return ErrInvalidArgument
	}

	switch m.Status {
	case MappingStatusActive,
		MappingStatusArchived:
	default:
		return ErrInvalidArgument
	}

	switch m.ConfidenceBasis {
	case MappingConfidenceManual,
		MappingConfidenceRule,
		MappingConfidenceImport,
		MappingConfidenceSystem:
	default:
		return ErrInvalidArgument
	}

	if m.ConfidenceScore != nil &&
		(*m.ConfidenceScore < 0 ||
			*m.ConfidenceScore > 10000) {
		return ErrInvalidArgument
	}

	return nil
}

type GovernanceCaseType string

const (
	GovernanceCaseMappingReview GovernanceCaseType = "MAPPING_REVIEW"

	GovernanceCaseClassificationDispute GovernanceCaseType = "CLASSIFICATION_DISPUTE"

	GovernanceCaseDuplicateConcept GovernanceCaseType = "DUPLICATE_CONCEPT"

	GovernanceCaseMisclassification GovernanceCaseType = "MISCLASSIFICATION"

	GovernanceCaseVocabularyReview GovernanceCaseType = "VOCABULARY_REVIEW"

	GovernanceCasePolicyReview GovernanceCaseType = "POLICY_REVIEW"
)

type GovernanceStatus string

const (
	GovernanceStatusOpen      GovernanceStatus = "OPEN"
	GovernanceStatusInReview  GovernanceStatus = "IN_REVIEW"
	GovernanceStatusResolved  GovernanceStatus = "RESOLVED"
	GovernanceStatusRejected  GovernanceStatus = "REJECTED"
	GovernanceStatusCancelled GovernanceStatus = "CANCELLED"
)

type GovernancePriority string

const (
	GovernancePriorityLow      GovernancePriority = "LOW"
	GovernancePriorityNormal   GovernancePriority = "NORMAL"
	GovernancePriorityHigh     GovernancePriority = "HIGH"
	GovernancePriorityCritical GovernancePriority = "CRITICAL"
)

type GovernanceCase struct {
	ID string

	NamespaceID string
	VersionID   string

	SubjectType string
	SubjectRef  string

	CaseType GovernanceCaseType
	Status   GovernanceStatus
	Priority GovernancePriority

	ReasonCode  *string
	Description *string

	ResolutionCode  *string
	ResolutionNotes *string

	AssignedActorRef *string

	OpenedByActorRef   string
	ResolvedByActorRef *string

	Metadata json.RawMessage

	Revision int64

	CreatedAt  time.Time
	UpdatedAt  time.Time
	ResolvedAt *time.Time
}
