package models

import "time"

// Provider self-view projections (FLEET-17.15). Every field is provider-facing:
// no money and no staff identifier is ever carried.

// OwnShift is one of the provider's shift assignments.
type OwnShift struct {
	ShiftAssignmentID string     `json:"shiftAssignmentId"`
	ShiftID           string     `json:"shiftId"`
	ShiftName         string     `json:"shiftName"`
	Role              string     `json:"role"`
	BranchID          string     `json:"branchId"`
	BranchName        string     `json:"branchName"`
	Timezone          string     `json:"timezone"`
	AssignmentType    string     `json:"assignmentType"`
	Status            string     `json:"status"`
	ShiftStatus       string     `json:"shiftStatus"`
	ExpectedStart     time.Time  `json:"expectedStart"`
	ExpectedEnd       time.Time  `json:"expectedEnd"`
	CheckedInAt       *time.Time `json:"checkedInAt,omitempty"`
	CheckedOutAt      *time.Time `json:"checkedOutAt,omitempty"`
}

// OwnLeave is one of the provider's leave records.
type OwnLeave struct {
	LeaveID   string    `json:"leaveId"`
	LeaveType string    `json:"leaveType"`
	Status    string    `json:"status"`
	StartAt   time.Time `json:"startAt"`
	EndAt     time.Time `json:"endAt"`
}

// OwnSchedule is the provider's schedule over a window.
type OwnSchedule struct {
	MembershipID string     `json:"membershipId"`
	From         time.Time  `json:"from"`
	To           time.Time  `json:"to"`
	Shifts       []OwnShift `json:"shifts"`
	Leave        []OwnLeave `json:"leave"`
}

// OwnQualification is one qualification record of the provider, or a
// mandatory qualification they do not yet hold (Status MISSING).
type OwnQualification struct {
	ProviderQualificationID string     `json:"providerQualificationId,omitempty"`
	QualificationID         string     `json:"qualificationId"`
	Code                    string     `json:"code"`
	Name                    string     `json:"name"`
	Status                  string     `json:"status"`
	Mandatory               bool       `json:"mandatory"`
	EvidenceProvided        bool       `json:"evidenceProvided"`
	IssuedAt                *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt               *time.Time `json:"expiresAt,omitempty"`
	VerifiedAt              *time.Time `json:"verifiedAt,omitempty"`
	Expired                 bool       `json:"expired"`
}

// OwnTrainingEnrollment is one course assigned to the provider.
type OwnTrainingEnrollment struct {
	EnrollmentID  string     `json:"enrollmentId"`
	CourseID      string     `json:"courseId"`
	CourseCode    string     `json:"courseCode"`
	CourseTitle   string     `json:"courseTitle"`
	CourseVersion int64      `json:"courseVersion"`
	Status        string     `json:"status"`
	AssignedAt    time.Time  `json:"assignedAt"`
	DueAt         *time.Time `json:"dueAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

// OwnTrainingCompletion is one recorded course result of the provider.
type OwnTrainingCompletion struct {
	CompletionID       string     `json:"completionId"`
	CourseID           string     `json:"courseId"`
	CourseCode         string     `json:"courseCode"`
	CourseTitle        string     `json:"courseTitle"`
	Status             string     `json:"status"`
	Score              float64    `json:"score"`
	CertificateOnFile  bool       `json:"certificateOnFile"`
	CompletedAt        time.Time  `json:"completedAt"`
	CertificateExpires *time.Time `json:"certificateExpiresAt,omitempty"`
}

// OwnTraining is the provider's training record.
type OwnTraining struct {
	MembershipID string                  `json:"membershipId"`
	Enrollments  []OwnTrainingEnrollment `json:"enrollments"`
	Completions  []OwnTrainingCompletion `json:"completions"`
}
