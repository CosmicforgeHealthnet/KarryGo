package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/observability"
)

// Flags gate Section 7 passenger capability behaviour.
type Flags struct {
	ServiceProductsEnabled    bool // TAXI_SERVICE_PRODUCTS_ENABLED: product listing + shadow evaluation
	CapabilityMatchingEnabled bool // TAXI_CAPABILITY_MATCHING_ENABLED: enforce hard gates in matching
}

// Store is the persistence the service needs; *Repository implements it.
type Store interface {
	LatestPolicy(ctx context.Context, kind string, now time.Time) ([]byte, string, error)
	InsertPolicy(ctx context.Context, kind, version, checksum string, effectiveAt time.Time, payload []byte) (bool, error)
	GetCapability(ctx context.Context, vehicleID, providerID string) (capability.VehicleCapability, error)
	PutProviderCapability(ctx context.Context, providerID string, input capability.VehicleCapability, expectedVersion int64, sensitive []string) (capability.VehicleCapability, []string, error)
	ListEligibility(ctx context.Context, vehicleID string) ([]capability.Eligibility, error)
	ApplyCertification(ctx context.Context, event CertificationEvent) (string, error)
	PassengerCandidates(ctx context.Context, providerIDs []string) (map[string]capability.Candidate, error)
	RecordMatchEvidence(ctx context.Context, bookingID, instance, mode string, rows []EvidenceRow) error
	BookingRequirement(ctx context.Context, bookingID string) (string, []byte, error)
}

type Service struct {
	repo    Store
	flags   Flags
	metrics *observability.Metrics
	now     func() time.Time

	mu         sync.RWMutex
	lastPolicy *capability.Policy
}

func NewService(repo Store, flags Flags, metrics *observability.Metrics) *Service {
	return &Service{repo: repo, flags: flags, metrics: metrics, now: time.Now}
}

func (s *Service) Flags() Flags { return s.flags }

// CurrentPolicy returns the newest applied policy. When the database is
// unreachable, Taxi keeps using the last valid snapshot it applied.
func (s *Service) CurrentPolicy(ctx context.Context) (capability.Policy, error) {
	payload, _, err := s.repo.LatestPolicy(ctx, capability.PolicyKindServiceProducts, s.now().UTC())
	if err == nil {
		policy, _, parseErr := capability.ParsePolicy(payload)
		if parseErr == nil {
			s.mu.Lock()
			s.lastPolicy = &policy
			s.mu.Unlock()
			return policy, nil
		}
		err = parseErr
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastPolicy != nil {
		logging.Notice("taxi-policy", "using last valid policy %s: %v", s.lastPolicy.Version, err)
		return *s.lastPolicy, nil
	}
	return capability.Policy{}, err
}

// ApplyPolicy validates and stores an immutable control-plane snapshot.
func (s *Service) ApplyPolicy(ctx context.Context, raw []byte) (capability.Policy, bool, error) {
	policy, checksum, err := capability.ParsePolicy(raw)
	if err != nil {
		return capability.Policy{}, false, ErrInvalid
	}
	canonical, _ := json.Marshal(policy)
	created, err := s.repo.InsertPolicy(ctx, capability.PolicyKindServiceProducts, policy.Version, checksum, policy.EffectiveAt, canonical)
	return policy, created, err
}

func (s *Service) GetProviderCapability(ctx context.Context, providerID, vehicleID string) (capability.VehicleCapability, []capability.Eligibility, error) {
	if !validUUID(vehicleID) {
		return capability.VehicleCapability{}, nil, ErrNotFound
	}
	v, err := s.repo.GetCapability(ctx, vehicleID, providerID)
	if err != nil {
		return v, nil, err
	}
	eligibility, err := s.repo.ListEligibility(ctx, vehicleID)
	return v, eligibility, err
}

func (s *Service) GetInternalCapability(ctx context.Context, vehicleID string) (capability.VehicleCapability, []capability.Eligibility, error) {
	return s.GetProviderCapability(ctx, "", vehicleID)
}

// PutProviderCapability records provider-declared facts. Self-declaration
// never certifies anything: certification-sensitive edits send eligible
// products back to PENDING review.
func (s *Service) PutProviderCapability(ctx context.Context, providerID string, input capability.VehicleCapability, expectedVersion int64) (capability.VehicleCapability, []string, error) {
	if !validUUID(input.VehicleID) {
		return capability.VehicleCapability{}, nil, ErrNotFound
	}
	if err := validateCapability(input); err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	policy, err := s.CurrentPolicy(ctx)
	if err != nil {
		return capability.VehicleCapability{}, nil, err
	}
	return s.repo.PutProviderCapability(ctx, providerID, input, expectedVersion, policy.CertificationRequiredFields)
}

func validateCapability(v capability.VehicleCapability) error {
	if v.TotalSeats != nil && (*v.TotalSeats < 1 || *v.TotalSeats > 60) {
		return ErrInvalid
	}
	if v.PassengerCapacity != nil && (*v.PassengerCapacity < 1 || (v.TotalSeats != nil && *v.PassengerCapacity >= *v.TotalSeats)) {
		return ErrInvalid
	}
	if v.TemperatureMinC != nil && v.TemperatureMaxC != nil && *v.TemperatureMinC > *v.TemperatureMaxC {
		return ErrInvalid
	}
	for _, value := range []*int64{v.LuggageVolumeML, v.GoodsMaxWeightGrams, v.GoodsMaxVolumeML, v.GoodsMaxLengthMM, v.GoodsMaxWidthMM, v.GoodsMaxHeightMM, v.GoodsMaxPackageCount} {
		if value != nil && *value < 0 {
			return ErrInvalid
		}
	}
	return nil
}

func (s *Service) ApplyCertification(ctx context.Context, event CertificationEvent) (string, error) {
	event.EventID, event.ProductCode = strings.TrimSpace(event.EventID), strings.ToUpper(strings.TrimSpace(event.ProductCode))
	event.Decision, event.CertificationRef = strings.ToUpper(strings.TrimSpace(event.Decision)), strings.TrimSpace(event.CertificationRef)
	if event.EventID == "" || !validUUID(event.VehicleID) || event.ProductCode == "" || event.SourceVersion < 1 || event.CertificationRef == "" || strings.TrimSpace(event.PolicyVersion) == "" {
		return "", ErrInvalid
	}
	if event.Decision == "CERTIFIED" && event.ExpiresAt != nil && !event.ExpiresAt.After(s.now()) {
		return "", ErrInvalid
	}
	return s.repo.ApplyCertification(ctx, event)
}

// ProductView is one listed service product. It never carries a price.
type ProductView struct {
	ProductCode       string   `json:"product_code"`
	ProductVersion    int64    `json:"product_version"`
	Purpose           string   `json:"purpose"`
	Label             string   `json:"label"`
	PassengerCapacity int      `json:"passenger_capacity,omitempty"`
	MaxPickups        int      `json:"max_pickups,omitempty"`
	Features          []string `json:"features,omitempty"`
	Serviceability    string   `json:"serviceability"`
}

type ProductQuery struct {
	MarketID       string
	ZoneID         string
	PassengerCount int
	Purpose        string
	Requirements   []string
}

// ListProducts returns products active under the applied policy that can
// satisfy the requested passenger count and hard requirements.
func (s *Service) ListProducts(ctx context.Context, query ProductQuery) ([]ProductView, string, error) {
	if !s.flags.ServiceProductsEnabled {
		return nil, "", ErrDisabled
	}
	policy, err := s.CurrentPolicy(ctx)
	if err != nil {
		return nil, "", err
	}
	purpose := strings.ToUpper(strings.TrimSpace(query.Purpose))
	if purpose != "" && purpose != capability.PurposePassenger && purpose != capability.PurposeGoods {
		return nil, "", ErrInvalid
	}
	out := make([]ProductView, 0)
	for _, product := range policy.ActiveProducts(purpose, strings.TrimSpace(query.MarketID)) {
		if product.Purpose == capability.PurposePassenger && query.PassengerCount > product.MaxPassengers {
			continue
		}
		if !hasFeatures(product, query.Requirements) {
			continue
		}
		out = append(out, ProductView{ProductCode: product.Code, ProductVersion: product.Version, Purpose: product.Purpose, Label: product.Label,
			PassengerCapacity: product.MaxPassengers, MaxPickups: product.MaxPickups, Features: product.Features, Serviceability: "POLICY_ACTIVE"})
	}
	return out, policy.Version, nil
}

func hasFeatures(product capability.Product, requirements []string) bool {
	for _, requirement := range requirements {
		requirement = strings.ToUpper(strings.TrimSpace(requirement))
		if requirement == "" {
			continue
		}
		found := false
		for _, feature := range product.Features {
			if strings.EqualFold(feature, requirement) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// PassengerRequirementFor builds the frozen hard-requirement snapshot of a
// booking. Legacy bookings map their vehicle_class to a product code.
func PassengerRequirementFor(booking bookingmodels.Booking, productCode string, snapshot []byte) capability.PassengerRequirement {
	requirement := capability.PassengerRequirement{ProductCode: productCode, PassengerCount: booking.PassengerCount}
	if requirement.ProductCode == "" {
		requirement.ProductCode = capability.LegacyProductForClass(booking.VehicleClass)
	}
	if len(snapshot) > 0 {
		_ = json.Unmarshal(snapshot, &requirement)
		if productCode != "" {
			requirement.ProductCode = productCode
		}
	}
	if requirement.PassengerCount < 1 {
		requirement.PassengerCount = booking.PassengerCount
	}
	return requirement
}

// FilterPassengerCandidates evaluates hard gates BEFORE scoring. In shadow
// mode it records the decision and keeps the legacy candidate list; when
// enforcement is on, rejected candidates never reach ranking.
func (s *Service) FilterPassengerCandidates(ctx context.Context, booking bookingmodels.Booking, productCode string, snapshot []byte, providerIDs []string, instance string) ([]string, error) {
	if !s.flags.ServiceProductsEnabled && !s.flags.CapabilityMatchingEnabled {
		return providerIDs, nil
	}
	policy, err := s.CurrentPolicy(ctx)
	if err != nil {
		if s.flags.CapabilityMatchingEnabled {
			return nil, err
		}
		return providerIDs, nil
	}
	if productCode == "" && snapshot == nil {
		if code, stored, lookupErr := s.repo.BookingRequirement(ctx, booking.ID); lookupErr == nil {
			productCode, snapshot = code, stored
		}
	}
	requirement := PassengerRequirementFor(booking, productCode, snapshot)
	product, ok := policy.Product(requirement.ProductCode)
	candidates, err := s.repo.PassengerCandidates(ctx, providerIDs)
	if err != nil {
		if s.flags.CapabilityMatchingEnabled {
			return nil, err
		}
		return providerIDs, nil
	}
	mode := "SHADOW"
	if s.flags.CapabilityMatchingEnabled {
		mode = "ENFORCED"
	}
	now := s.now().UTC()
	var kept []string
	var evidence []EvidenceRow
	for _, id := range providerIDs {
		candidate, found := candidates[id]
		decision := capability.Decision{Reason: capability.ReasonVehicleNotOperational}
		if found && ok {
			decision = capability.EvaluatePassenger(candidate, requirement, product, now)
		} else if found {
			decision = capability.Decision{Reason: capability.ReasonProductNotEligible}
		}
		evidence = append(evidence, EvidenceRow{ProviderID: id, VehicleID: candidate.VehicleID, Decision: decision, LegacyEligible: true})
		if s.metrics != nil {
			s.metrics.IncCounter("taxi_match_candidates_total", map[string]string{"mode": mode}, 1)
			if !decision.Eligible {
				s.metrics.IncCounter("taxi_match_hard_rejections_total", map[string]string{"purpose": "PASSENGER", "reason": decision.Reason, "mode": mode}, 1)
			}
		}
		if decision.Eligible || mode == "SHADOW" {
			kept = append(kept, id)
		}
	}
	if err := s.repo.RecordMatchEvidence(ctx, booking.ID, instance, mode, evidence); err != nil {
		logging.Error("taxi-matcher", "record match evidence booking=%s: %v", booking.ID, err)
	}
	if len(kept) == 0 && s.metrics != nil {
		s.metrics.IncCounter("taxi_match_no_eligible_provider_total", nil, 1)
	}
	return kept, nil
}

var ErrDisabled = errors.New("capability feature is disabled")

func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}
