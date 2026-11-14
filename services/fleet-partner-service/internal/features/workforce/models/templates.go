package models

import "time"

// Shift templates and training enrolments (spec Sections 17 and 46).

// ShiftTemplate is a weekly repeating shift pattern in a branch timezone.
type ShiftTemplate struct {
	ID               string         `json:"id"`
	OrganizationID   string         `json:"organizationId"`
	BranchID         string         `json:"branchId"`
	Name             string         `json:"name"`
	Role             string         `json:"role"`
	Timezone         string         `json:"timezone"`
	Weekdays         []int          `json:"weekdays"`
	StartLocalTime   string         `json:"startLocalTime"`
	EndLocalTime     string         `json:"endLocalTime"`
	RequiredCapacity int            `json:"requiredCapacity"`
	BreakRules       map[string]any `json:"breakRules"`
	Status           string         `json:"status"`
	Version          int64          `json:"version"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// GeneratedShifts reports what a template generation produced. Days whose
// local start or end does not exist (a DST gap) are skipped and listed:
// silently shifting a shift by an hour is the bug spec 46.2 warns about.
type GeneratedShifts struct {
	TemplateID   string   `json:"templateId"`
	Created      []Shift  `json:"created"`
	AlreadyExist []string `json:"alreadyExist"`
	SkippedDays  []string `json:"skippedDays"`
}

// Enrollment assigns a training course to a staff member or provider.
type Enrollment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	CourseID       string     `json:"courseId"`
	CourseVersion  int64      `json:"courseVersion"`
	MembershipID   string     `json:"membershipId"`
	Status         string     `json:"status"`
	AssignedBy     string     `json:"assignedBy"`
	AssignedAt     time.Time  `json:"assignedAt"`
	DueAt          *time.Time `json:"dueAt"`
	CompletedAt    *time.Time `json:"completedAt"`
	Score          *float64   `json:"score"`
	CompletionID   *string    `json:"completionId"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
