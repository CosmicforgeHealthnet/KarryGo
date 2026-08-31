package taxonomy

import (
	"encoding/json"
	"strings"
	"time"
)

type SemanticTagType string

const (
	SemanticTagAttribute     SemanticTagType = "ATTRIBUTE"
	SemanticTagDiscovery     SemanticTagType = "DISCOVERY"
	SemanticTagPreparation   SemanticTagType = "PREPARATION"
	SemanticTagMerchandising SemanticTagType = "MERCHANDISING"
)

type SemanticStatus string

const (
	SemanticStatusActive   SemanticStatus = "ACTIVE"
	SemanticStatusArchived SemanticStatus = "ARCHIVED"
)

type SemanticTag struct {
	ID          string
	NamespaceID string
	Code        string
	DisplayName string
	Description *string
	TagType     SemanticTagType
	Status      SemanticStatus
	Metadata    json.RawMessage
	Revision    int64
}

type DietaryVerificationPolicy string

const (
	DietaryVerificationDeclaration DietaryVerificationPolicy = "DECLARATION"

	DietaryVerificationEvidenceRequired DietaryVerificationPolicy = "EVIDENCE_REQUIRED"

	DietaryVerificationPlatformVerified DietaryVerificationPolicy = "PLATFORM_VERIFIED"
)

type DietaryRiskLevel string

const (
	DietaryRiskLow      DietaryRiskLevel = "LOW"
	DietaryRiskModerate DietaryRiskLevel = "MODERATE"
	DietaryRiskHigh     DietaryRiskLevel = "HIGH"
)

type DietaryClaim struct {
	ID                 string
	NamespaceID        string
	Code               string
	DisplayName        string
	Description        *string
	VerificationPolicy DietaryVerificationPolicy
	RiskLevel          DietaryRiskLevel
	Status             SemanticStatus
	Metadata           json.RawMessage
	Revision           int64
}

type DietaryClaimAssignmentStatus string

const (
	DietaryAssignmentDeclared DietaryClaimAssignmentStatus = "DECLARED"

	DietaryAssignmentPendingVerification DietaryClaimAssignmentStatus = "PENDING_VERIFICATION"

	DietaryAssignmentVerified DietaryClaimAssignmentStatus = "VERIFIED"

	DietaryAssignmentRejected DietaryClaimAssignmentStatus = "REJECTED"

	DietaryAssignmentExpired DietaryClaimAssignmentStatus = "EXPIRED"
)

type SemanticTagAssignment struct {
	ID          string
	NamespaceID string
	TagID       string
	SourceType  string
	SourceRef   string
	Status      SemanticStatus
	Revision    int64
}

type DietaryClaimAssignment struct {
	ID          string
	NamespaceID string
	ClaimID     string

	SourceType string
	SourceRef  string

	Status DietaryClaimAssignmentStatus

	EvidenceRef        *string
	VerifiedByActorRef *string
	VerifiedAt         *time.Time
	ExpiresAt          *time.Time

	Revision int64
}

func validateSemanticCode(code string) error {
	code = strings.TrimSpace(code)

	if code == "" {
		return ErrInvalidArgument
	}

	for i, r := range code {
		valid :=
			(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') ||
				r == '_'

		if !valid {
			return ErrInvalidArgument
		}

		if i == 0 && r >= '0' && r <= '9' {
			return ErrInvalidArgument
		}
	}

	return nil
}
