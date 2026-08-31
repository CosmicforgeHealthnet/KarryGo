package inventory

import "strings"

// BranchScope represents business-wide or exact-branch Inventory authority.
//
// BranchID == nil means business-wide authority.
//
// BranchID != nil means exact-branch authority.
type BranchScope struct {
	BusinessID string
	BranchID   *string
}

// BusinessScope creates Inventory authority for one business.
func BusinessScope(
	businessID string,
) (BranchScope, error) {
	scope := BranchScope{
		BusinessID: strings.TrimSpace(businessID),
	}

	if err := scope.Validate(); err != nil {
		return BranchScope{}, err
	}

	return scope, nil
}

// ExactBranchScope creates Inventory authority for one exact branch.
func ExactBranchScope(
	businessID string,
	branchID string,
) (BranchScope, error) {
	businessID = strings.TrimSpace(businessID)
	branchID = strings.TrimSpace(branchID)

	if businessID == "" || branchID == "" {
		return BranchScope{}, ErrInvalidScope
	}

	scope := BranchScope{
		BusinessID: businessID,
		BranchID:   &branchID,
	}

	if err := scope.Validate(); err != nil {
		return BranchScope{}, err
	}

	return scope, nil
}

// Validate rejects missing business scope or an empty exact branch.
func (s BranchScope) Validate() error {
	if strings.TrimSpace(s.BusinessID) == "" {
		return ErrInvalidScope
	}

	if s.BranchID != nil &&
		strings.TrimSpace(*s.BranchID) == "" {
		return ErrInvalidScope
	}

	return nil
}

// BusinessWide reports whether the scope is business-wide.
func (s BranchScope) BusinessWide() bool {
	return s.BranchID == nil
}

// Covers determines whether this scope covers the requested branch.
//
// A business-wide scope still never crosses BusinessID.
func (s BranchScope) Covers(
	businessID string,
	branchID string,
) bool {
	if s.Validate() != nil {
		return false
	}

	businessID = strings.TrimSpace(businessID)
	branchID = strings.TrimSpace(branchID)

	if businessID == "" ||
		branchID == "" ||
		businessID != strings.TrimSpace(s.BusinessID) {
		return false
	}

	if s.BranchID == nil {
		return true
	}

	return strings.TrimSpace(*s.BranchID) == branchID
}
