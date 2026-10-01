package bikecapability

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/logging"
)

// Service owns the capability, eligibility and policy workflows.
type Service struct {
	repository *Repository
	now        func() time.Time

	// certificationRequired lists the products that may not be served on the
	// strength of a bike type alone. A product absent from this set is open to
	// any physically-capable bike, which is the right default: most dispatch
	// work is ordinary parcels and demanding a certificate for them would
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

// RequiresCertification reports whether a product demands formal admission.
func (s *Service) RequiresCertification(productCode string) bool {
	if s == nil {
		return false
	}
	return s.certificationRequired[strings.ToUpper(strings.TrimSpace(productCode))]
}

// ProductForRequirement derives the service product a requirement implies and
// whether it demands a certificate in this deployment.
//
// It is deliberately minimal: only cold chain currently distinguishes a
// certified product from ordinary delivery, and inventing further codes here
// would create a vocabulary nothing else in the platform shares.
func (s *Service) ProductForRequirement(requirement parcelfit.Requirement) (string, bool) {
	product := ProductStandardDelivery
	if requirement.NeedsThermal() {
		product = ProductColdChain
	}
	return product, s.RequiresCertification(product)
}

// ── Provider-facing ──────────────────────────────────────────────────────────

// GetCapability returns one bike's capability for its own provider.
func (s *Service) GetCapability(ctx context.Context, providerID, bikeID string) (PublicRecord, error) {
	record, err := s.repository.GetByBike(ctx, bikeID)
	if errors.Is(err, ErrNotFound) {
		return PublicRecord{}, apperrors.NotFound("Bike not found.", err)
	}
	if err != nil {
		return PublicRecord{}, apperrors.Internal("Bike capability could not be read.", err)
	}
	// Ownership is answered as a 404 either way: telling a caller "that bike
	// exists but is not yours" leaks another provider's asset.
	if record.ProviderID != providerID {
		return PublicRecord{}, apperrors.NotFound("Bike not found.", nil)
	}
	return record.Public(), nil
}

// UpdateCapability applies a rider's declared capability edit.
//
// Certification-bearing facts are deliberately not self-declarable: a rider
// may state their box size, but "admitted to cold chain" has to come from the
// certifying authority. UpdateInput simply has no field for it, so the
// restriction cannot be forgotten at a call site.
func (s *Service) UpdateCapability(ctx context.Context, input UpdateInput) (PublicRecord, error) {
	if err := validateUpdate(input); err != nil {
		return PublicRecord{}, err
	}
	record, err := s.repository.UpsertCapability(ctx, input, s.now())
	switch {
	case errors.Is(err, ErrVersionConflict):
		return PublicRecord{}, apperrors.Conflict(
			"This bike was updated by someone else. Reload and try again.", err)
	case errors.Is(err, ErrNotFound):
		return PublicRecord{}, apperrors.NotFound("Bike not found.", err)
	case err != nil:
		return PublicRecord{}, apperrors.Internal("Bike capability could not be saved.", err)
	}
	return record.Public(), nil
}

func validateUpdate(input UpdateInput) error {
	var fields []apperrors.FieldViolation

	positives := map[string]*int64{
		"max_payload_grams": input.MaxPayloadGrams.Value,
		"box_length_mm":     input.BoxLengthMm.Value,
		"box_width_mm":      input.BoxWidthMm.Value,
		"box_height_mm":     input.BoxHeightMm.Value,
	}
	for field, value := range positives {
		if value != nil && *value <= 0 {
			fields = append(fields, apperrors.FieldViolation{
				Field:   field,
				Message: "Must be greater than zero. Leave it out if you do not know it.",
			})
		}
	}

	// A temperature class only means something with equipment behind it. The
	// schema enforces this too; catching it here turns a 500 into a field
	// error the rider can act on.
	if input.TemperatureClass != nil && strings.TrimSpace(*input.TemperatureClass) != "" {
		if input.ThermalBag.Value == nil || !*input.ThermalBag.Value {
			fields = append(fields, apperrors.FieldViolation{
				Field:   "temperature_class",
				Message: "Set thermal_bag before declaring a temperature class.",
			})
		}
	}
	if input.TemperatureClass != nil {
		switch strings.ToUpper(strings.TrimSpace(*input.TemperatureClass)) {
		case "", "AMBIENT", "CHILLED", "FROZEN":
		default:
			fields = append(fields, apperrors.FieldViolation{
				Field:   "temperature_class",
				Message: "Must be AMBIENT, CHILLED or FROZEN.",
			})
		}
	}

	if len(fields) > 0 {
		return apperrors.Validation("Please check the bike capability details.", fields)
	}
	return nil
}

// ListEligibility returns a bike's product admissions for its own provider.
func (s *Service) ListEligibility(ctx context.Context, providerID, bikeID string) ([]PublicProductEligibility, error) {
	record, err := s.repository.GetByBike(ctx, bikeID)
	if errors.Is(err, ErrNotFound) || (err == nil && record.ProviderID != providerID) {
		return nil, apperrors.NotFound("Bike not found.", err)
	}
	if err != nil {
		return nil, apperrors.Internal("Bike eligibility could not be read.", err)
	}
	stored, err := s.repository.ListEligibility(ctx, bikeID)
	if err != nil {
		return nil, apperrors.Internal("Bike eligibility could not be read.", err)
	}
	out := make([]PublicProductEligibility, 0, len(stored))
	for _, eligibility := range stored {
		out = append(out, eligibility.Public())
	}
	return out, nil
}

// ── Certification ────────────────────────────────────────────────────────────

// CertificationEvent is a decision from the certifying authority.
type CertificationEvent struct {
	BikeID           string
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
// A duplicate is a success with applied=false, not an error: the caller is an
// at-least-once delivery pipeline, and turning a replay into a failure would
// make it retry the same message forever.
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

	record, err := s.repository.GetByBike(ctx, event.BikeID)
	if errors.Is(err, ErrNotFound) {
		return PublicProductEligibility{}, false, apperrors.NotFound("Bike not found.", err)
	}
	if err != nil {
		return PublicProductEligibility{}, false, apperrors.Internal("Bike could not be read.", err)
	}

	stored, changed, err := s.repository.UpsertEligibility(ctx, ProductEligibility{
		BikeID:           record.BikeID,
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

// ── Matcher primitives ───────────────────────────────────────────────────────

// LoadProviderBikeFits returns every active bike, as fit-engine input, for
// each provider, with product eligibility already projected onto it.
//
// Projecting eligibility here rather than leaving it to the caller is what
// makes the certification gate unforgettable: the fit engine receives a bike
// that already knows whether it is admitted, so no call site can evaluate
// physical fit and quietly skip the certificate.
//
// An error means the store could not answer. The caller must turn that into
// DEPENDENCY_UNAVAILABLE rather than an empty fleet.
func (s *Service) LoadProviderBikeFits(
	ctx context.Context,
	providerIDs []string,
	requirement parcelfit.Requirement,
) (map[string][]parcelfit.Capability, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("bike capability store is not configured")
	}
	records, err := s.repository.LoadProviderBikes(ctx, providerIDs)
	if err != nil {
		return nil, err
	}

	bikeIDs := make([]string, 0)
	for _, bikes := range records {
		for _, bike := range bikes {
			bikeIDs = append(bikeIDs, bike.BikeID)
		}
	}

	var eligibility map[string]ProductEligibility
	if requirement.CertificationRequired && len(bikeIDs) > 0 {
		eligibility, err = s.repository.LoadEligibilityForProduct(ctx, bikeIDs, requirement.ProductCode)
		if err != nil {
			// An eligibility-store failure is a dependency failure for the
			// whole question, not a per-bike "uncertified": reporting the
			// latter would tell an operator their fleet lost its certificates.
			return nil, err
		}
	}

	now := s.now()
	out := make(map[string][]parcelfit.Capability, len(records))
	for providerID, bikes := range records {
		converted := make([]parcelfit.Capability, 0, len(bikes))
		for _, bike := range bikes {
			fit := bike.ToFit()
			if requirement.CertificationRequired {
				fit.ProductCode = requirement.ProductCode
				fit.ProductEligibility, fit.ProductEligibilityNote =
					projectEligibility(eligibility[bike.BikeID], requirement.ProductCode, now)
			}
			converted = append(converted, fit)
		}
		out[providerID] = converted
	}
	return out, nil
}

// projectEligibility turns a stored admission into the engine's three-valued
// input plus a human note.
//
// A missing row and a lapsed one both come back as No, with different notes:
// the same decision, a different explanation, which is what an operator needs.
func projectEligibility(eligibility ProductEligibility, product string, now time.Time) (capabilityfit.Tri, string) {
	if eligibility.BikeID == "" {
		return capabilityfit.No, "bike holds no eligibility for product " + product
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

// CapabilityPolicyVersion returns the locally-snapshotted policy version.
func (s *Service) CapabilityPolicyVersion(ctx context.Context) string {
	if s == nil || s.repository == nil {
		return DefaultCapabilityPolicyVersion
	}
	return s.repository.LatestPolicyVersion(ctx, PolicyKindCapability, DefaultCapabilityPolicyVersion)
}

// EvaluateBike runs the fit engine against one stored bike, resolving the
// three-way lookup outcome from the repository's own error.
//
// Serviceability and recovery both go through here so neither can reintroduce
// the "error means skip the check" shortcut.
func (s *Service) EvaluateBike(ctx context.Context, bikeID string, requirement parcelfit.Requirement) *capabilityfit.Decision {
	record, err := s.repository.GetByBike(ctx, bikeID)
	switch {
	case errors.Is(err, ErrNotFound):
		return parcelfit.Evaluate(parcelfit.ResolutionMissing, parcelfit.Capability{}, requirement)
	case err != nil:
		return parcelfit.Evaluate(parcelfit.ResolutionFailed, parcelfit.Capability{}, requirement)
	}
	// A bike with a registration row but no capability row is "never
	// surveyed", which is CAPABILITY_UNKNOWN rather than a missing bike.
	if !record.Surveyed {
		return parcelfit.Evaluate(parcelfit.ResolutionMissing, parcelfit.Capability{}, requirement)
	}

	fit := record.ToFit()
	if requirement.CertificationRequired {
		eligibilities, err := s.repository.LoadEligibilityForProduct(ctx, []string{bikeID}, requirement.ProductCode)
		if err != nil {
			return parcelfit.Evaluate(parcelfit.ResolutionFailed, parcelfit.Capability{}, requirement)
		}
		fit.ProductCode = requirement.ProductCode
		fit.ProductEligibility, fit.ProductEligibilityNote =
			projectEligibility(eligibilities[bikeID], requirement.ProductCode, s.now())
	}
	return parcelfit.Evaluate(parcelfit.ResolutionFound, fit, requirement)
}

// ── Background maintenance ───────────────────────────────────────────────────

// RunMaintenanceWorker expires lapsed certifications, publishes the rollout
// metrics and enforces evidence retention.
//
// One worker rather than three: all three are cheap periodic sweeps over the
// same tables, and three tickers would triple the scheduling noise for no
// operational benefit.
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
		logging.Error("dispatch-capability", "expire lapsed eligibility: %v", err)
	} else if expired > 0 {
		logging.Notice("dispatch-capability", "expired %d lapsed bike certifications", expired)
		observability.ProductEligibilityChange(string(EligibilityExpired))
	}

	if purged, err := s.repository.PurgeDecisionsBefore(ctx, now.Add(-retention)); err != nil {
		logging.Error("dispatch-capability", "purge match decisions: %v", err)
	} else if purged > 0 {
		logging.Notice("dispatch-capability", "purged %d expired match decisions", purged)
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
