package workforce

import (
	"encoding/json"
	"time"
)

type Flags struct {
	Workforce, Shifts, Leave, Qualifications, Training, Performance, Workers, KillSwitch bool
}

type Profile struct {
	ID, OrganizationID, BranchID, MembershipID, EmployeeNumber, RelationshipType, Role, SupervisorMembershipID, Status string
	StartDate, CreatedAt, UpdatedAt                                                                                    time.Time
	EndDate                                                                                                            *time.Time
	Version                                                                                                            int64
}
type Shift struct {
	ID, OrganizationID, BranchID, Name, Role, Timezone, Status, PlannerUserID string
	StartAt, EndAt, CreatedAt, UpdatedAt                                      time.Time
	RequiredCapacity                                                          int
	Version                                                                   int64
}
type ShiftAssignment struct {
	ID, OrganizationID, ShiftID, MembershipID, AssignmentType, Status, CheckInSource, CheckOutSource string
	ExpectedStart, ExpectedEnd, CreatedAt, UpdatedAt                                                 time.Time
	CheckedInAt, CheckedOutAt                                                                        *time.Time
	Version                                                                                          int64
}
type Leave struct {
	ID, OrganizationID, MembershipID, LeaveType, Reason, Status, ApprovedBy string
	StartAt, EndAt, CreatedAt, UpdatedAt                                    time.Time
	ApprovedAt                                                              *time.Time
	Version                                                                 int64
}
type Qualification struct {
	ID, OrganizationID, Code, Name, Status                    string
	ApplicableRoles, ApplicableServices, ApplicableAssetTypes []string
	ValidityPeriodSeconds                                     *int64
	EvidenceRequired                                          bool
	Version                                                   int64
	CreatedAt, UpdatedAt                                      time.Time
}
type ProviderQualification struct {
	ID, OrganizationID, MembershipID, QualificationID, EvidenceMediaID, VerificationReference, Status, VerifiedBy string
	IssuedAt, ExpiresAt, VerifiedAt                                                                               *time.Time
	Version                                                                                                       int64
	CreatedAt, UpdatedAt                                                                                          time.Time
}
type Course struct {
	ID, OrganizationID, Code, Title, ProviderReference, Status string
	RequiredRoles, RequiredServices                            []string
	ValidityPeriodSeconds                                      *int64
	PassingScore                                               float64
	Version                                                    int64
	CreatedAt, UpdatedAt                                       time.Time
}
type Completion struct {
	ID, OrganizationID, CourseID, MembershipID, Status, CertificateMediaID, VerifiedBy string
	Score                                                                              float64
	CompletedAt                                                                        time.Time
	ExpiresAt                                                                          *time.Time
	Version                                                                            int64
	CreatedAt, UpdatedAt                                                               time.Time
}
type PerformanceSnapshot struct {
	ID, OrganizationID, MembershipID, GenerationStatus, FormulaVersion string
	PeriodStart, PeriodEnd, GeneratedAt                                time.Time
	MetricInputs, CalculatedMetrics, Explanation, SourceVersions       json.RawMessage
	Version                                                            int64
}
type Eligibility struct {
	Eligible   bool     `json:"eligible"`
	HardBlocks []string `json:"hardBlocks"`
	Warnings   []string `json:"warnings"`
	Reasons    []string `json:"reasons"`
}
type Page struct {
	BranchID, Status string
	Limit, Offset    int
}

var profileTransitions = map[string]map[string]bool{"ACTIVE": {"INACTIVE": true, "ENDED": true, "ARCHIVED": true}, "INACTIVE": {"ACTIVE": true, "ENDED": true, "ARCHIVED": true}, "ENDED": {"ARCHIVED": true}}
var shiftTransitions = map[string]map[string]bool{"DRAFT": {"PUBLISHED": true, "CANCELLED": true}, "PUBLISHED": {"IN_PROGRESS": true, "COMPLETED": true, "CANCELLED": true}, "IN_PROGRESS": {"COMPLETED": true, "CANCELLED": true}}
var assignmentTransitions = map[string]map[string]bool{"PENDING": {"CONFIRMED": true, "DECLINED": true, "CANCELLED": true}, "CONFIRMED": {"CHECKED_IN": true, "NO_SHOW": true, "CANCELLED": true}, "CHECKED_IN": {"CHECKED_OUT": true}}
var leaveTransitions = map[string]map[string]bool{"REQUESTED": {"APPROVED": true, "REJECTED": true, "CANCELLED": true}, "APPROVED": {"CANCELLED": true, "COMPLETED": true}}

func transition(m map[string]map[string]bool, from, to string) bool { return from == to || m[from][to] }
func ValidProfileTransition(a, b string) bool                       { return transition(profileTransitions, a, b) }
func ValidShiftTransition(a, b string) bool                         { return transition(shiftTransitions, a, b) }
func ValidAssignmentTransition(a, b string) bool                    { return transition(assignmentTransitions, a, b) }
func ValidLeaveTransition(a, b string) bool                         { return transition(leaveTransitions, a, b) }
