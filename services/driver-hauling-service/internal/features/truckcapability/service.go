package truckcapability

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/observability"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/logging"
)

// Service owns the capability, eligibility and policy workflows.
type Service struct {
	repository *Repository
	now        func() time.Time

	// certificationRequired lists the products that may not be served on the
	// strength of a truck type alone. A product absent from this set is open
	// to any physically-capable truck, which is the correct default: most
	// haulage is general freight and demanding a certificate for it would
	// ground the fleet.
	certificationRequired map[string]bool
}

// Options configure the service.
type Options struct {
	Repository *Repository
	Now        func() time.Time
	// CertificationRequiredProducts is a comma-separated product list.
	CertificationRequiredProducts string
}

// New builds the service.
func New(options Options) *Service {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	required := map[string]bool{}
	for _, product := range strings.Split(options.CertificationRequiredProducts, ",") {
		if trimmed := strings.ToUpper(strings.TrimSpace(product)); trimmed != "" {
			required[trimmed] = true
		}
	}
	return &Service{repository: options.Repository, now: now, certificationRequired: required}
}

// ── Provider-facing capability ───────────────────────────────────────────────

// GetCapability returns one truck's capability for its own provider.
func (s *Service) GetCapability(ctx context.Context, providerID, truckID string) (PublicRecord, error) {
	record, err := s.repository.GetByTruck(ctx, truckID)
	if errors.Is(err, ErrNotFound) {
		return PublicRecord{}, apperrors.NotFound("Truck not found.", err)
	}
	if err != nil {
		return PublicRecord{}, apperrors.Internal("Truck capability could not be read.", err)
	}
	// Ownership is checked here rather than in SQL so the answer is a 404
	// either way: telling a caller "that truck exists but is not yours" leaks
	// the existence of another provider's asset.
	if record.ProviderID != providerID {
		return PublicRecord{}, apperrors.NotFound("Truck not found.", nil)
	}
	return record.Public(), nil
}

// UpdateCapability applies a provider's declared capability edit.
//
// Certification-bearing facts are deliberately NOT self-declarable: a provider
// may state their truck's dimensions, but stating "certified for cold chain"
// has to come from the certifying authority. The input type simply has no
// field for it, so the restriction cannot be forgotten at a call site.
func (s *Service) UpdateCapability(ctx context.Context, input UpdateInput) (PublicRecord, error) {
	if err := validateUpdate(input); err != nil {
		return PublicRecord{}, err
	}
	record, err := s.repository.UpdateCapability(ctx, input, s.now())
	switch {
	case errors.Is(err, ErrVersionConflict):
		return PublicRecord{}, apperrors.Conflict(
			"This truck was updated by someone else. Reload and try again.", err)
	case errors.Is(err, ErrNotFound):
		return PublicRecord{}, apperrors.NotFound("Truck not found.", err)
	case err != nil:
		return PublicRecord{}, apperrors.Internal("Truck capability could not be saved.", err)
	}
	return record.Public(), nil
}

func validateUpdate(input UpdateInput) error {
	var fields []apperrors.FieldViolation

	// A temperature band whose minimum exceeds its maximum describes no
	// achievable setpoint and would silently match nothing.
	if input.TemperatureMinC.Value != nil && input.TemperatureMaxC.Value != nil &&
		*input.TemperatureMinC.Value > *input.TemperatureMaxC.Value {
		fields = append(fields, apperrors.FieldViolation{
			Field:   "temperature_min_c",
			Message: "Minimum temperature cannot be above the maximum.",
		})
	}

	positives := map[string]*int64{
		"max_payload_grams":  input.MaxPayloadGrams.Value,
		"internal_length_mm": input.InternalLengthMm.Value,
		"internal_width_mm":  input.InternalWidthMm.Value,
		"internal_height_mm": input.InternalHeightMm.Value,
		"usable_volume_ml":   input.UsableVolumeMl.Value,
		"opening_width_mm":   input.OpeningWidthMm.Value,
		"opening_height_mm":  input.OpeningHeightMm.Value,
	}
	for field, value := range positives {
		if value != nil && *value <= 0 {
			fields = append(fields, apperrors.FieldViolation{
				Field:   field,
				Message: "Must be greater than zero. Leave it out if you do not know it.",
			})
		}
	}

	// An opening wider than the body it opens into is a measurement error, and
	// storing it would make the fit engine accept loads that cannot actually
	// be carried once inside.
	if input.OpeningWidthMm.Value != nil && input.InternalWidthMm.Value != nil &&
		*input.OpeningWidthMm.Value > *input.InternalWidthMm.Value {
		fields = append(fields, apperrors.FieldViolation{
			Field:   "opening_width_mm",
			Message: "The opening cannot be wider than the cargo body.",
		})
	}
	if input.OpeningHeightMm.Value != nil && input.InternalHeightMm.Value != nil &&
		*input.OpeningHeightMm.Value > *input.InternalHeightMm.Value {
		fields = append(fields, apperrors.FieldViolation{
			Field:   "opening_height_mm",
			Message: "The opening cannot be taller than the cargo body.",
		})
	}

	if input.SupportedGoodsClasses != nil {
		fields = append(fields, validateCodes("supported_goods_classes", *input.SupportedGoodsClasses)...)
	}
	if input.ProhibitedGoodsClasses != nil {
		fields = append(fields, validateCodes("prohibited_goods_classes", *input.ProhibitedGoodsClasses)...)
	}
	if input.SupportedContainerSizes != nil {
		fields = append(fields, validateCodes("supported_container_sizes", *input.SupportedContainerSizes)...)
	}

	if len(fields) > 0 {
		return apperrors.Validation("Please check the truck capability details.", fields)
	}
	return nil
}

// maxCodeListLength bounds a declared vocabulary list. These are stored as
// Postgres arrays and read on every match, so an unbounded list is both a
// storage and a matching-latency problem.
const maxCodeListLength = 32

func validateCodes(field string, values []string) []apperrors.FieldViolation {
	if len(values) > maxCodeListLength {
		return []apperrors.FieldViolation{{Field: field, Message: "Too many entries."}}
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return []apperrors.FieldViolation{{Field: field, Message: "Entries cannot be blank."}}
		}
		if len(value) > 64 {
			return []apperrors.FieldViolation{{Field: field, Message: "Entries are too long."}}
		}
	}
	return nil
}

// ── Product eligibility ──────────────────────────────────────────────────────

// ListEligibility returns a truck's product admissions for its own provider.
func (s *Service) ListEligibility(ctx context.Context, providerID, truckID string) ([]PublicProductEligibility, error) {
	record, err := s.repository.GetByTruck(ctx, truckID)
	if errors.Is(err, ErrNotFound) || (err == nil && record.ProviderID != providerID) {
		return nil, apperrors.NotFound("Truck not found.", err)
	}
	if err != nil {
		return nil, apperrors.Internal("Truck eligibility could not be read.", err)
	}
	stored, err := s.repository.ListEligibility(ctx, truckID)
	if err != nil {
		return nil, apperrors.Internal("Truck eligibility could not be read.", err)
	}
	out := make([]PublicProductEligibility, 0, len(stored))
	for _, eligibility := range stored {
		out = append(out, eligibility.Public())
	}
	return out, nil
}

// CertificationEvent is a decision from the certifying authority.
type CertificationEvent struct {
	TruckID          string
	ProductCode      string
	Status           string
	PolicyVersion    string
	CertificationRef string
	EffectiveAt      *time.Time
	ExpiresAt        *time.Time
	ReasonCode       string
	SourceVersion    int64
}

// ApplyCertificationEvent records a certification decision.
//
// It returns the stored eligibility and whether the event actually changed
// anything. A no-op is a success, not an error: the caller is an at-least-once
// delivery pipeline, and turning a duplicate into a failure would make it
// retry forever.
func (s *Service) ApplyCertificationEvent(ctx context.Context, event CertificationEvent) (PublicProductEligibility, bool, error) {
	status, ok := ParseEligibilityStatus(event.Status)
	if !ok {
		return PublicProductEligibility{}, false, apperrors.Validation(
			"Unknown eligibility status.", []apperrors.FieldViolation{
				{Field: "status", Message: "Must be PENDING, ELIGIBLE, SUSPENDED, EXPIRED or REJECTED."},
			})
	}
	if strings.TrimSpace(event.ProductCode) == "" {
		return PublicProductEligibility{}, false, apperrors.Validation(
			"Product code is required.", []apperrors.FieldViolation{
				{Field: "product_code", Message: "Product code is required."},
			})
	}
	if event.SourceVersion <= 0 {
		// Without a monotonic version there is no way to order two events, so
		// a replay could overwrite a newer decision with an older one.
		return PublicProductEligibility{}, false, apperrors.Validation(
			"Source version is required.", []apperrors.FieldViolation{
				{Field: "source_version", Message: "Must be a positive, increasing version."},
			})
	}

	record, err := s.repository.GetByTruck(ctx, event.TruckID)
	if errors.Is(err, ErrNotFound) {
		return PublicProductEligibility{}, false, apperrors.NotFound("Truck not found.", err)
	}
	if err != nil {
		return PublicProductEligibility{}, false, apperrors.Internal("Truck could not be read.", err)
	}

	stored, changed, err := s.repository.UpsertEligibility(ctx, ProductEligibility{
		TruckID:          record.TruckID,
		ProviderID:       record.ProviderID,
		ProductCode:      strings.ToUpper(strings.TrimSpace(event.ProductCode)),
		Status:           status,
		PolicyVersion:    event.PolicyVersion,
		CertificationRef: event.CertificationRef,
		EffectiveAt:      event.EffectiveAt,
		ExpiresAt:        event.ExpiresAt,
		ReasonCode:       event.ReasonCode,
		SourceVersion:    event.SourceVersion,
	})
	if err != nil {
		return PublicProductEligibility{}, false, apperrors.Internal("Eligibility could not be saved.", err)
	}
	if changed {
		observability.ProductEligibilityChange(string(status))
	}
	return stored.Public(), changed, nil
}

// RequiresCertification reports whether a product may only be served by a
// formally admitted truck.
func (s *Service) RequiresCertification(productCode string) bool {
	if s == nil {
		return false
	}
	return s.certificationRequired[strings.ToUpper(strings.TrimSpace(productCode))]
}

// ── Matcher primitives ───────────────────────────────────────────────────────

// LoadProviderTruckFits returns every active truck, as fit-engine input, for
// each provider, with product eligibility already projected onto it.
//
// Projecting eligibility here rather than leaving it to the caller is what
// makes the certification gate unforgettable: the fit engine receives a truck
// that already knows whether it is admitted, so no call site can evaluate
// physical fit and quietly skip the certificate.
//
// An error means the store could not answer. The caller must turn that into
// DEPENDENCY_UNAVAILABLE rather than an empty fleet -- an empty map returned
// on failure is precisely the fail-open this work removes.
func (s *Service) LoadProviderTruckFits(
	ctx context.Context,
	providerIDs []string,
	requirement cargofit.Requirement,
) (map[string][]cargofit.TruckCapability, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("truck capability store is not configured")
	}
	records, err := s.repository.LoadProviderTrucksForMatching(ctx, providerIDs)
	if err != nil {
		return nil, err
	}

	truckIDs := make([]string, 0)
	for _, trucks := range records {
		for _, truck := range trucks {
			truckIDs = append(truckIDs, truck.TruckID)
		}
	}

	var eligibility map[string]ProductEligibility
	if requirement.CertificationRequired && len(truckIDs) > 0 {
		eligibility, err = s.repository.LoadEligibilityForProduct(ctx, truckIDs, requirement.ProductCode)
		if err != nil {
			// The eligibility store failing is a dependency failure for the
			// whole question, not a per-truck "uncertified": reporting it as
			// the latter would tell an operator their fleet lost its
			// certificates.
			return nil, err
		}
	}

	now := s.now()
	out := make(map[string][]cargofit.TruckCapability, len(records))
	for providerID, trucks := range records {
		converted := make([]cargofit.TruckCapability, 0, len(trucks))
		for _, truck := range trucks {
			fit := truck.ToFit()
			if requirement.CertificationRequired {
				fit.ProductCode = requirement.ProductCode
				fit.ProductEligibility, fit.ProductEligibilityNote =
					projectEligibility(eligibility[truck.TruckID], requirement.ProductCode, now)
			}
			converted = append(converted, fit)
		}
		out[providerID] = converted
	}
	return out, nil
}

// projectEligibility turns a stored admission into the fit engine's
// three-valued input plus a human note.
//
// A missing row and a lapsed one both come back as No, with different notes.
// They are the same decision -- this truck may not do this work -- and a
// different explanation, which is what an operator actually needs.
func projectEligibility(eligibility ProductEligibility, product string, now time.Time) (capabilityfit.Tri, string) {
	if eligibility.TruckID == "" {
		return capabilityfit.No, "truck holds no eligibility for product " + product
	}
	if eligibility.UsableAt(now) {
		return capabilityfit.Yes, ""
	}
	return capabilityfit.No, "eligibility for " + product + " is " + string(eligibility.Status)
}

// RecordMatchDecisions persists hard-fit evidence.
func (s *Service) RecordMatchDecisions(ctx context.Context, decisions []MatchDecision) error {
	if s == nil || s.repository == nil {
		return nil
	}
	return s.repository.RecordDecisions(ctx, decisions)
}

// CapabilityPolicyVersion returns the locally-snapshotted policy version
// matching was evaluated against.
func (s *Service) CapabilityPolicyVersion(ctx context.Context) string {
	if s == nil || s.repository == nil {
		return DefaultCapabilityPolicyVersion
	}
	return s.repository.LatestPolicyVersion(ctx, PolicyKindCapability, DefaultCapabilityPolicyVersion)
}

// ProductForRequirement derives the service product a requirement implies and
// whether that product demands a certificate in this deployment.
func (s *Service) ProductForRequirement(requirement cargofit.Requirement) (product string, certificationRequired bool) {
	product = productForRequirement(requirement)
	return product, s.RequiresCertification(product)
}

// ── Background maintenance ───────────────────────────────────────────────────

// RunMaintenanceWorker expires lapsed certifications, publishes the rollout
// metrics and enforces evidence retention.
//
// It is one worker rather than three because all three are cheap periodic
// sweeps over the same two tables, and three tickers would triple the
// scheduling noise for no operational benefit.
func (s *Service) RunMaintenanceWorker(ctx context.Context, interval, retention time.Duration) {
	if s == nil || s.repository == nil {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	s.sweep(ctx, retention)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sweep(ctx, retention)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) sweep(ctx context.Context, retention time.Duration) {
	now := s.now()

	if expired, err := s.repository.ExpireLapsedEligibility(ctx, now); err != nil {
		logging.Error("hauling-capability", "expire lapsed eligibility: %v", err)
	} else if expired > 0 {
		logging.Notice("hauling-capability", "expired %d lapsed truck certifications", expired)
		observability.ProductEligibilityChange(string(EligibilityExpired))
	}

	if purged, err := s.repository.PurgeDecisionsBefore(ctx, now.Add(-retention)); err != nil {
		logging.Error("hauling-capability", "purge match decisions: %v", err)
	} else if purged > 0 {
		logging.Notice("hauling-capability", "purged %d expired match decisions", purged)
	}

	if surveyed, total, err := s.repository.FleetCompleteness(ctx); err == nil && total > 0 {
		observability.FleetCapabilityCompleteness(float64(surveyed) * 100 / float64(total))
	}

	for _, kind := range []string{PolicyKindCapability, PolicyKindProduct} {
		if lag, ok := s.repository.PolicyLagSeconds(ctx, kind, now); ok {
			observability.PolicySnapshotLag(kind, lag)
		}
	}
}

// ── Fit helper shared by preview and matching ────────────────────────────────

// EvaluateTruck runs the fit engine against one stored truck, resolving the
// three-way lookup outcome from the repository's own error.
//
// Preview and recovery both go through here, so neither can reintroduce the
// "error means skip the check" shortcut the old matcher had.
func (s *Service) EvaluateTruck(ctx context.Context, truckID string, requirement cargofit.Requirement) *capabilityfit.Decision {
	record, err := s.repository.GetByTruck(ctx, truckID)
	switch {
	case errors.Is(err, ErrNotFound):
		return cargofit.Evaluate(cargofit.ResolutionMissing, cargofit.TruckCapability{}, requirement)
	case err != nil:
		return cargofit.Evaluate(cargofit.ResolutionFailed, cargofit.TruckCapability{}, requirement)
	}

	fit := record.ToFit()
	if requirement.CertificationRequired {
		eligibilities, err := s.repository.LoadEligibilityForProduct(ctx, []string{truckID}, requirement.ProductCode)
		if err != nil {
			return cargofit.Evaluate(cargofit.ResolutionFailed, cargofit.TruckCapability{}, requirement)
		}
		fit.ProductCode = requirement.ProductCode
		fit.ProductEligibility, fit.ProductEligibilityNote =
			projectEligibility(eligibilities[truckID], requirement.ProductCode, s.now())
	}
	return cargofit.Evaluate(cargofit.ResolutionFound, fit, requirement)
}

// productForRequirement derives the service product a requirement implies.
//
// It is intentionally minimal: only cold chain currently distinguishes a
// certified product from general haulage, and inventing further product codes
// here would create a vocabulary nothing else in the platform shares.
func productForRequirement(requirement cargofit.Requirement) string {
	if requirement.NeedsRefrigeration() {
		return ProductColdChain
	}
	return ProductGeneralHaulage
}

// Product codes Hauling serves.
const (
	// ProductGeneralHaulage is ordinary freight. It requires no certification
	// by default.
	ProductGeneralHaulage = "HAUL_GENERAL"
	// ProductColdChain is temperature-controlled haulage, which a deployment
	// will normally add to HAULING_CERTIFICATION_REQUIRED_PRODUCTS.
	ProductColdChain = "HAUL_COLD_CHAIN"
)
