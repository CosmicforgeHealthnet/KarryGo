package privateexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type Service struct {
	repo                *Repository
	verifier            AcceptanceVerifier
	enabled, killSwitch bool
	restrictions        interface {
		Restricted(context.Context, string, string, string) (bool, error)
	}
}

func (s *Service) WithRestrictionGate(gate interface {
	Restricted(context.Context, string, string, string) (bool, error)
}) *Service {
	s.restrictions = gate
	return s
}

func NewService(repo *Repository, verifier AcceptanceVerifier, enabled, killSwitch bool) *Service {
	return &Service{repo: repo, verifier: verifier, enabled: enabled, killSwitch: killSwitch}
}

func fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validate(in CreateRequest) error {
	for _, v := range []string{in.OrganizationID, in.SettlementOrganizationID, in.ExecutingProviderID, in.OrganizationMembershipID, in.PrivateOrderID, in.ShipmentID, in.PrivateAssignmentID, in.ProviderResponseID, in.FleetOperationID, in.TruckID, in.IdempotencyKey} {
		if strings.TrimSpace(v) == "" {
			return errors.New("required private execution reference is missing")
		}
	}
	if in.ServiceCategory != "HAULING" {
		return errors.New("serviceCategory must be HAULING")
	}
	if in.Route.PickupAddress == "" || in.Route.DropoffAddress == "" {
		return errors.New("route snapshot is required")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, in CreateRequest) (View, error) {
	if !s.enabled || s.killSwitch {
		return View{}, ErrDisabled
	}
	if err := validate(in); err != nil {
		return View{}, err
	}
	if s.restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", in.OrganizationID}, {"PROVIDER", in.ExecutingProviderID}, {"ASSET", in.TruckID}} {
			restricted, err := s.restrictions.Restricted(ctx, in.OrganizationID, subject[0], subject[1])
			if err != nil {
				return View{}, err
			}
			if restricted {
				return View{}, errors.New("private execution subject is restricted")
			}
		}
	}
	copy := in
	copy.IdempotencyKey = ""
	copy.CorrelationID = ""
	fp := fingerprint(copy)
	if prior, err := s.repo.GetKey(ctx, in.IdempotencyKey); err == nil {
		if prior.RequestFingerprint != fp {
			return View{}, ErrConflict
		}
		return prior.View, nil
	} else if !errors.Is(err, ErrNotFound) {
		return View{}, err
	}
	if s.verifier == nil {
		return View{}, ErrDisabled
	}
	acceptance, err := s.verifier.Verify(ctx, in.PrivateAssignmentID)
	if err != nil {
		return View{}, err
	}
	if acceptance.AssignmentID != in.PrivateAssignmentID || acceptance.OrganizationID != in.OrganizationID || acceptance.ProviderID != in.ExecutingProviderID || acceptance.ProviderService != "HAULING" || acceptance.OrganizationMembershipID != in.OrganizationMembershipID || acceptance.ProviderResponseID != in.ProviderResponseID || acceptance.ResponseStatus != "ACCEPTED" {
		return View{}, errors.New("provider acceptance does not match private execution")
	}
	record, err := s.repo.Create(ctx, in, fp)
	if errors.Is(err, ErrConflict) {
		prior, getErr := s.repo.GetKey(ctx, in.IdempotencyKey)
		if getErr != nil {
			return View{}, getErr
		}
		if prior.RequestFingerprint != fp {
			return View{}, ErrConflict
		}
		return prior.View, nil
	}
	return record.View, err
}
func (s *Service) Read(ctx context.Context, id string) (View, error) {
	if !s.enabled || s.killSwitch {
		return View{}, ErrDisabled
	}
	r, err := s.repo.GetOperation(ctx, id)
	return r.View, err
}
func (s *Service) ReadByTrip(ctx context.Context, id string) (View, error) {
	if !s.enabled || s.killSwitch {
		return View{}, ErrDisabled
	}
	r, err := s.repo.GetTrip(ctx, id)
	return r.View, err
}
func (s *Service) Cancel(ctx context.Context, id string, in CancelRequest) (CancelView, error) {
	if !s.enabled || s.killSwitch {
		return CancelView{}, ErrDisabled
	}
	if in.IdempotencyKey == "" {
		return CancelView{}, errors.New("idempotency key is required")
	}
	fp := fingerprint(map[string]string{"operationId": id, "reason": in.Reason})
	return s.repo.Cancel(ctx, id, in.IdempotencyKey, fp, in.Reason)
}
func (s *Service) ReadCancellation(ctx context.Context, id string) (CancelView, error) {
	r, err := s.repo.GetOperation(ctx, id)
	if err != nil {
		return CancelView{}, err
	}
	status := r.CancellationStatus
	if status == "CONFIRMED" {
		status = "CANCELLED"
	}
	return CancelView{status, r.SourceVersion}, nil
}
