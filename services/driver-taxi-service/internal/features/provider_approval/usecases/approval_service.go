// Package providerapprovalusecases approves a taxi driver after their
// documents have been checked. It is an operator action, reached only through
// a signed internal request (there is no admin console yet).
package providerapprovalusecases

import (
	"context"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

// Driver is the approval view of a taxi driver.
type Driver struct {
	ID                        string `json:"id"`
	Phone                     string `json:"phone"`
	FirstName                 string `json:"first_name"`
	LastName                  string `json:"last_name"`
	AccountStatus             string `json:"account_status"`
	OnboardingStatus          string `json:"onboarding_status"`
	IdentityStatus            string `json:"identity_verification_status"`
	DocumentStatus            string `json:"document_compliance_status"`
	VehicleVerificationStatus string `json:"vehicle_verification_status"`
	VehicleOperationalStatus  string `json:"vehicle_operational_status"`
}

type Repository interface {
	FindByID(ctx context.Context, id string) (Driver, error)
	FindByPhone(ctx context.Context, phone string) (Driver, error)
	Approve(ctx context.Context, id string) (Driver, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

type ApproveInput struct {
	ProviderID string
	Phone      string
}

// Approve finds the driver by id or phone and approves them. A suspended
// account or one that has not finished its profile is refused: approval must
// not undo a suspension or skip the driver's own details.
func (s *Service) Approve(ctx context.Context, in ApproveInput) (Driver, error) {
	id, phone := strings.TrimSpace(in.ProviderID), strings.TrimSpace(in.Phone)
	if (id == "") == (phone == "") {
		return Driver{}, apperrors.Validation("Check your request.", []apperrors.FieldViolation{
			{Field: "provider_id", Message: "Send exactly one of provider_id or phone."},
		})
	}
	var (
		d   Driver
		err error
	)
	if id != "" {
		d, err = s.repo.FindByID(ctx, id)
	} else {
		normalized, nerr := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if nerr != nil {
			return Driver{}, nerr
		}
		d, err = s.repo.FindByPhone(ctx, normalized)
	}
	if err != nil {
		return Driver{}, err
	}
	if d.AccountStatus != "active" {
		return Driver{}, apperrors.Conflict("This account is "+d.AccountStatus+"; approval does not lift that.", nil)
	}
	if d.OnboardingStatus == "profile_required" {
		return Driver{}, apperrors.Conflict("This driver has not finished setting up their account yet.", nil)
	}
	return s.repo.Approve(ctx, d.ID)
}
