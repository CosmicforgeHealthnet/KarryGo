package staffusecases

import (
	"context"

	"cosmicforge/logistics/shared/go/apperrors"
)

func validateBranchScope(ctx context.Context, validator BranchScopeValidator, vendorID string, branchID *string) error {
	if validator == nil || branchID == nil {
		return nil
	}
	ok, err := validator.BranchBelongsToVendor(ctx, vendorID, *branchID)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "branch_id", Message: "Branch does not belong to this business."}})
	}
	return nil
}
