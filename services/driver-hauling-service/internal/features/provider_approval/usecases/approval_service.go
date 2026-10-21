// Package providerapprovalusecases approves a truck driver after their
// documents have been checked. It is an operator action, reached only through
// a signed internal request (there is no admin console yet).
package providerapprovalusecases

import (
	"context"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

// Driver is the approval view of a truck driver.
type Driver struct {
	ID               string `json:"id"`
	Phone            string `json:"phone"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	Status           string `json:"status"`
	OnboardingStatus string `json:"onboarding_status"`
	ActiveTrucks     int    `json:"active_trucks"`
}

type Repository interface {
	FindByID(ctx context.Context, id string) (Driver, error)
	FindByPhone(ctx context.Context, phone string) (Driver, error)
	Approve(ctx context.Context, id string, activateTrucks bool) (Driver, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

type ApproveInput struct {
	ProviderID     string
	Phone          string
	ActivateTrucks bool
}

// Approve finds the driver by id or phone and completes their onboarding. A
// suspended account or one that has not finished its profile is refused.
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
	if d.Status != "" && d.Status != "active" {
		return Driver{}, apperrors.Conflict("This account is "+d.Status+"; approval does not lift that.", nil)
	}
	if d.OnboardingStatus == "profile_required" {
		return Driver{}, apperrors.Conflict("This driver has not finished setting up their account yet.", nil)
	}
	return s.repo.Approve(ctx, d.ID, in.ActivateTrucks)
}
