package workforce

import (
	"encoding/json"
	"time"
)

type Flags struct {
	Workforce, Shifts, Leave, Qualifications, Training, Performance, Workers, KillSwitch bool
}

type Profile struct {
	ID                     string     `json:"id"`
	OrganizationID         string     `json:"organizationId"`
	BranchID               string     `json:"branchId"`
	MembershipID           string     `json:"membershipId"`
	EmployeeNumber         string     `json:"employeeNumber"`
	RelationshipType       string     `json:"relationshipType"`
	Role                   string     `json:"role"`
	SupervisorMembershipID string     `json:"supervisorMembershipId"`
	Status                 string     `json:"status"`
	StartDate              time.Time  `json:"startDate"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
	EndDate                *time.Time `json:"endDate"`
	Version                int64      `json:"version"`
}
type Shift struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	BranchID         string    `json:"branchId"`
	Name             string    `json:"name"`
	Role             string    `json:"role"`
	Timezone         string    `json:"timezone"`
	Status           string    `json:"status"`
	PlannerUserID    string    `json:"plannerUserId"`
	StartAt          time.Time `json:"startAt"`
	EndAt            time.Time `json:"endAt"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	RequiredCapacity int       `json:"requiredCapacity"`
	Version          int64     `json:"version"`
}
type ShiftAssignment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	ShiftID        string     `json:"shiftId"`
	MembershipID   string     `json:"membershipId"`
	AssignmentType string     `json:"assignmentType"`
	Status         string     `json:"status"`
	CheckInSource  string     `json:"checkInSource"`
	CheckOutSource string     `json:"checkOutSource"`
	ExpectedStart  time.Time  `json:"expectedStart"`
	ExpectedEnd    time.Time  `json:"expectedEnd"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	CheckedInAt    *time.Time `json:"checkedInAt"`
	CheckedOutAt   *time.Time `json:"checkedOutAt"`
	Version        int64      `json:"version"`
}
type Leave struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	MembershipID   string     `json:"membershipId"`
	LeaveType      string     `json:"leaveType"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ApprovedBy     string     `json:"approvedBy"`
	StartAt        time.Time  `json:"startAt"`
	EndAt          time.Time  `json:"endAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	ApprovedAt     *time.Time `json:"approvedAt"`
	Version        int64      `json:"version"`
}
type Qualification struct {
	ID                    string    `json:"id"`
	OrganizationID        string    `json:"organizationId"`
	Code                  string    `json:"code"`
	Name                  string    `json:"name"`
	Status                string    `json:"status"`
	ApplicableRoles       []string  `json:"applicableRoles"`
	ApplicableServices    []string  `json:"applicableServices"`
	ApplicableAssetTypes  []string  `json:"applicableAssetTypes"`
	ValidityPeriodSeconds *int64    `json:"validityPeriodSeconds"`
	EvidenceRequired      bool      `json:"evidenceRequired"`
	Version               int64     `json:"version"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}
type ProviderQualification struct {
	ID                    string     `json:"id"`
	OrganizationID        string     `json:"organizationId"`
	MembershipID          string     `json:"membershipId"`
	QualificationID       string     `json:"qualificationId"`
	EvidenceMediaID       string     `json:"evidenceMediaId"`
	VerificationReference string     `json:"verificationReference"`
	Status                string     `json:"status"`
	VerifiedBy            string     `json:"verifiedBy"`
	IssuedAt              *time.Time `json:"issuedAt"`
	ExpiresAt             *time.Time `json:"expiresAt"`
	VerifiedAt            *time.Time `json:"verifiedAt"`
	Version               int64      `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// QualificationPatch is a partial qualification update: empty/nil fields
// keep their stored value.
type QualificationPatch struct {
	Name                 string   `json:"name"`
	ApplicableRoles      []string `json:"applicableRoles"`
	ApplicableServices   []string `json:"applicableServices"`
	ApplicableAssetTypes []string `json:"applicableAssetTypes"`
	EvidenceRequired     *bool    `json:"evidenceRequired"`
	Version              int64    `json:"version"`
}

// CoursePatch is a partial training-course update: empty/nil fields keep
// their stored value.
type CoursePatch struct {
	Title        string   `json:"title"`
	PassingScore *float64 `json:"passingScore"`
	Version      int64    `json:"version"`
}

type Course struct {
	ID                    string    `json:"id"`
	OrganizationID        string    `json:"organizationId"`
	Code                  string    `json:"code"`
	Title                 string    `json:"title"`
	ProviderReference     string    `json:"providerReference"`
	Status                string    `json:"status"`
	RequiredRoles         []string  `json:"requiredRoles"`
	RequiredServices      []string  `json:"requiredServices"`
	ValidityPeriodSeconds *int64    `json:"validityPeriodSeconds"`
	PassingScore          float64   `json:"passingScore"`
	Version               int64     `json:"version"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}
type Completion struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organizationId"`
	CourseID           string     `json:"courseId"`
	MembershipID       string     `json:"membershipId"`
	Status             string     `json:"status"`
	CertificateMediaID string     `json:"certificateMediaId"`
	VerifiedBy         string     `json:"verifiedBy"`
	Score              float64    `json:"score"`
	CompletedAt        time.Time  `json:"completedAt"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
type PerformanceSnapshot struct {
	ID                string          `json:"id"`
	OrganizationID    string          `json:"organizationId"`
	MembershipID      string          `json:"membershipId"`
	GenerationStatus  string          `json:"generationStatus"`
	FormulaVersion    string          `json:"formulaVersion"`
	PeriodStart       time.Time       `json:"periodStart"`
	PeriodEnd         time.Time       `json:"periodEnd"`
	GeneratedAt       time.Time       `json:"generatedAt"`
	MetricInputs      json.RawMessage `json:"metricInputs"`
	CalculatedMetrics json.RawMessage `json:"calculatedMetrics"`
	Explanation       json.RawMessage `json:"explanation"`
	SourceVersions    json.RawMessage `json:"sourceVersions"`
	Version           int64           `json:"version"`
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
