package staffhttp

type inviteStaffRequest struct {
	Phone    string  `json:"phone"`
	Email    string  `json:"email"`
	RoleID   string  `json:"role_id"`
	BranchID *string `json:"branch_id,omitempty"`
}

type staffView struct {
	ID              string  `json:"id"`
	VendorID        string  `json:"vendor_id"`
	Status          string  `json:"status"`
	SuspendedReason *string `json:"suspended_reason,omitempty"`
	Version         int64   `json:"version"`
	CreatedAt       string  `json:"created_at"`
}

type suspendStaffRequest struct {
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_version"`
}

type reactivateStaffRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type assignmentView struct {
	ID         string  `json:"id"`
	StaffID    string  `json:"staff_id"`
	RoleID     string  `json:"role_id"`
	BranchID   *string `json:"branch_id,omitempty"`
	AssignedBy *string `json:"assigned_by,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type permissionView struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Category    string `json:"category"`
	IsHighRisk  bool   `json:"is_high_risk"`
}

type roleView struct {
	ID              string   `json:"id"`
	VendorID        *string  `json:"vendor_id,omitempty"`
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	IsSystem        bool     `json:"is_system"`
	Version         int64    `json:"version"`
	PermissionCodes []string `json:"permission_codes,omitempty"`
}

type createRoleRequest struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permission_codes"`
	StepUpGrantID   string   `json:"step_up_grant_id"`
}

type setRolePermissionsRequest struct {
	PermissionCodes []string `json:"permission_codes"`
	ExpectedVersion int64    `json:"expected_version"`
	StepUpGrantID   string   `json:"step_up_grant_id"`
}

type assignRoleRequest struct {
	StaffID       string  `json:"staff_id"`
	RoleID        string  `json:"role_id"`
	BranchID      *string `json:"branch_id,omitempty"`
	StepUpGrantID string  `json:"step_up_grant_id"`
}

type provisionOwnerRequest struct {
	VendorID string `json:"vendor_id"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
}
