package taxonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type OpenGovernanceCaseInput struct {
	NamespaceCode NamespaceCode

	SubjectType string
	SubjectRef  string

	CaseType GovernanceCaseType
	Priority GovernancePriority

	ReasonCode  *string
	Description *string

	Metadata json.RawMessage

	Actor Actor
}

type GovernanceTransition string

const (
	GovernanceTransitionStartReview GovernanceTransition = "START_REVIEW"

	GovernanceTransitionAssign GovernanceTransition = "ASSIGN_CASE"

	GovernanceTransitionResolve GovernanceTransition = "RESOLVE_CASE"

	GovernanceTransitionReject GovernanceTransition = "REJECT_CASE"

	GovernanceTransitionCancel GovernanceTransition = "CANCEL_CASE"
)

type TransitionGovernanceCaseInput struct {
	CaseID string

	ExpectedRevision int64

	Transition GovernanceTransition

	AssignedActorRef *string

	ResolutionCode  *string
	ResolutionNotes *string

	Metadata json.RawMessage

	Actor Actor
}

type GovernanceCommandResult struct {
	Case GovernanceCase `json:"case"`

	Replayed bool `json:"replayed"`
}

func deterministicGovernanceCaseID(
	namespaceCode NamespaceCode,
	versionID string,
	subjectType string,
	subjectRef string,
	caseType GovernanceCaseType,
) string {
	raw := strings.Join(
		[]string{
			string(namespaceCode),
			versionID,
			strings.ToUpper(
				strings.TrimSpace(subjectType),
			),
			strings.TrimSpace(subjectRef),
			string(caseType),
		},
		"|",
	)

	sum := sha256.Sum256([]byte(raw))

	return "ctgov_" +
		hex.EncodeToString(sum[:16])
}

func governanceRequestHash(
	value any,
) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(payload)

	return hex.EncodeToString(sum[:]), nil
}

func validGovernanceCaseType(
	value GovernanceCaseType,
) bool {
	switch value {
	case GovernanceCaseMappingReview,
		GovernanceCaseClassificationDispute,
		GovernanceCaseDuplicateConcept,
		GovernanceCaseMisclassification,
		GovernanceCaseVocabularyReview,
		GovernanceCasePolicyReview:
		return true
	default:
		return false
	}
}

func validGovernancePriority(
	value GovernancePriority,
) bool {
	switch value {
	case GovernancePriorityLow,
		GovernancePriorityNormal,
		GovernancePriorityHigh,
		GovernancePriorityCritical:
		return true
	default:
		return false
	}
}

func validGovernanceTransition(
	value GovernanceTransition,
) bool {
	switch value {
	case GovernanceTransitionStartReview,
		GovernanceTransitionAssign,
		GovernanceTransitionResolve,
		GovernanceTransitionReject,
		GovernanceTransitionCancel:
		return true
	default:
		return false
	}
}
