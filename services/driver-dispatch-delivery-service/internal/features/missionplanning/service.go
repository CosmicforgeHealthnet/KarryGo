package missionplanning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/rollout"
	"cosmicforge/logistics/shared/go/phonenumber"
)

var (
	ErrNotFound      = errors.New("delivery plan not found")
	ErrConflict      = errors.New("delivery plan conflict")
	ErrReplanBlocked = errors.New("mission is too advanced to replan")
)

type Store interface {
	Create(context.Context, PlanRequest, Plan, string) (Plan, error)
	Replan(context.Context, string, PlanRequest, Plan, string) (Plan, error)
	Read(context.Context, string) (Plan, error)
	ReadBySource(context.Context, string, int64) (Plan, error)
	ActivateManifestCustody(context.Context, string, CustodyCommand, string) error
	ReleaseManifestCustody(context.Context, string, CustodyCommand, string) error
}
type Service struct {
	planner Planner
	store   Store
	enabled bool
	routes  RouteAuthority
	rollout rollout.Policy
}

func New(store Store, planner Planner, enabled bool) *Service {
	return &Service{store: store, planner: planner, enabled: enabled}
}
func (s *Service) WithRouteAuthority(routes RouteAuthority) *Service { s.routes = routes; return s }
func (s *Service) WithRolloutPolicy(policy rollout.Policy) *Service  { s.rollout = policy; return s }
func requestFingerprint(in PlanRequest) string {
	in.CorrelationID = ""
	in.RouteEvidence = nil
	in.Parcels = append([]Parcel(nil), in.Parcels...)
	for i := range in.Parcels {
		in.Parcels[i].KarryGoParcelRef = ""
		in.Parcels[i].ScannableCode = ""
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *Service) Create(ctx context.Context, in PlanRequest) (Plan, error) {
	if !s.enabled {
		return Plan{}, errors.New("multi-vendor planning is disabled")
	}
	if strings.TrimSpace(in.CustomerID) == "" || strings.TrimSpace(in.ReceiverName) == "" || strings.TrimSpace(in.ReceiverPhone) == "" || in.FareAmount <= 0 || strings.TrimSpace(in.Currency) == "" || strings.TrimSpace(in.PaymentRef) == "" {
		return Plan{}, errors.New("customer, receiver and accepted commercial snapshot are required")
	}
	normalizedPhone, normalizeErr := phonenumber.NormalizeNigerianPhoneNumber(in.ReceiverPhone)
	if normalizeErr != nil {
		return Plan{}, errors.New("receiver phone is invalid")
	}
	in.ReceiverPhone = normalizedPhone
	in.Protection = s.rollout.Protect(rollout.Context{CustomerID: in.CustomerID, SourceType: "VENDOR_COMMERCE", SourceRef: in.SourceRef, ServiceTier: in.ServiceClass})
	if in.Protection.Mode != rollout.ModeProtected {
		return Plan{}, errors.New("multi-vendor planning is not selected for the protected rollout cohort")
	}
	if s.routes == nil {
		return Plan{}, errors.New("route authority unavailable")
	}
	evidence, err := s.routes.Evaluate(ctx, in)
	if err != nil {
		return Plan{}, err
	}
	in.RouteEvidence = &evidence
	plan, err := s.planner.Build(in)
	if err != nil {
		return Plan{}, err
	}
	BindStableIdentities(&plan, &in)
	return s.store.Create(ctx, in, plan, requestFingerprint(in))
}
func (s *Service) Replan(ctx context.Context, groupRef string, in PlanRequest) (Plan, error) {
	if !s.enabled {
		return Plan{}, errors.New("multi-vendor planning is disabled")
	}
	if strings.TrimSpace(groupRef) == "" {
		return Plan{}, ErrNotFound
	}
	if strings.TrimSpace(in.CustomerID) == "" || strings.TrimSpace(in.ReceiverName) == "" || strings.TrimSpace(in.ReceiverPhone) == "" || in.FareAmount <= 0 || strings.TrimSpace(in.Currency) == "" || strings.TrimSpace(in.PaymentRef) == "" {
		return Plan{}, errors.New("customer, receiver and accepted commercial snapshot are required")
	}
	normalizedPhone, normalizeErr := phonenumber.NormalizeNigerianPhoneNumber(in.ReceiverPhone)
	if normalizeErr != nil {
		return Plan{}, errors.New("receiver phone is invalid")
	}
	in.ReceiverPhone = normalizedPhone
	in.Protection = s.rollout.Protect(rollout.Context{CustomerID: in.CustomerID, SourceType: "VENDOR_COMMERCE", SourceRef: in.SourceRef, ServiceTier: in.ServiceClass})
	if in.Protection.Mode != rollout.ModeProtected {
		return Plan{}, errors.New("multi-vendor replanning is not selected for the protected rollout cohort")
	}
	if strings.TrimSpace(in.OperationRef) == "" || in.ExpectedPlanVersion < 1 {
		return Plan{}, errors.New("operation and expected plan version are required")
	}
	if s.routes == nil {
		return Plan{}, errors.New("route authority unavailable")
	}
	evidence, err := s.routes.Evaluate(ctx, in)
	if err != nil {
		return Plan{}, err
	}
	in.RouteEvidence = &evidence
	plan, err := s.planner.Build(in)
	if err != nil {
		return Plan{}, err
	}
	plan.GroupRef = groupRef
	BindStableIdentities(&plan, &in)
	return s.store.Replan(ctx, groupRef, in, plan, requestFingerprint(in))
}
func (s *Service) Read(ctx context.Context, groupRef string) (Plan, error) {
	if !s.enabled {
		return Plan{}, errors.New("multi-vendor planning is disabled")
	}
	return s.store.Read(ctx, groupRef)
}

func (s *Service) ReadBySource(ctx context.Context, sourceRef string, sourceVersion int64) (Plan, error) {
	if !s.enabled {
		return Plan{}, errors.New("multi-vendor planning is disabled")
	}
	if strings.TrimSpace(sourceRef) == "" || sourceVersion < 1 {
		return Plan{}, errors.New("source_ref and source_version are required")
	}
	return s.store.ReadBySource(ctx, strings.TrimSpace(sourceRef), sourceVersion)
}
func (s *Service) ActivateCustody(ctx context.Context, manifestRef string, command CustodyCommand) error {
	if !s.enabled || strings.TrimSpace(command.OperationRef) == "" || command.ManifestVersion < 1 || command.SourceVersion < 1 {
		return errors.New("custody command is invalid")
	}
	return s.store.ActivateManifestCustody(ctx, manifestRef, command, requestFingerprint(PlanRequest{SourceRef: command.SourceRef, SourceVersion: command.SourceVersion, OperationRef: command.OperationRef}))
}
func (s *Service) ReleaseCustody(ctx context.Context, manifestRef string, command CustodyCommand) error {
	if !s.enabled || strings.TrimSpace(command.OperationRef) == "" || command.ManifestVersion < 1 || command.SourceVersion < 1 {
		return errors.New("custody command is invalid")
	}
	return s.store.ReleaseManifestCustody(ctx, manifestRef, command, requestFingerprint(PlanRequest{SourceRef: command.SourceRef, SourceVersion: command.SourceVersion, OperationRef: command.OperationRef}))
}
