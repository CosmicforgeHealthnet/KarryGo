package delivery

// Vendor Own Delivery policy management.
//
// Every read and every mutation derives the tenant from the authenticated
// session. A caller-supplied business id is not authority here; it is not read
// at all. Branch reach is checked before any row is touched, so a refusal
// cannot be used to probe whether another branch's policy exists.
//
// No mutation here has any financial effect. Configuring a delivery charge
// records a commercial intention; it moves no money, settles nothing and
// writes to no ledger.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

// Service owns vendor-own delivery policy authorization and persistence.
type Service struct {
	db         *pgxpool.Pool
	production *production.Service

	// enabled is VENDOR_OWN_DELIVERY_ENABLED, the platform capability switch.
	// It refuses writes here and gates route registration in http.go.
	//
	// ResolvePolicy is deliberately NOT gated by it: planning passes the flag
	// into Quote explicitly, which refuses with its own distinct reason code,
	// so a disabled platform produces an explainable planning answer instead
	// of an indistinguishable "no policy".
	enabled bool
}

// NewService builds the Section 17C vendor-own delivery service.
func NewService(
	db *pgxpool.Pool,
	productionService *production.Service,
	enabled bool,
) *Service {
	return &Service{
		db:         db,
		production: productionService,
		enabled:    enabled,
	}
}

// Enabled reports the platform capability switch.
func (s *Service) Enabled() bool { return s != nil && s.enabled }

// Authorize delegates to the Section 5 evaluator, so delivery permissions are
// evaluated by the same IAM that governs everything else rather than by a
// private rule this section invented.
func (s *Service) Authorize(
	ctx context.Context,
	actor production.Actor,
	permission string,
) (production.Scope, error) {
	if s == nil || s.production == nil {
		return production.Scope{}, ErrForbidden
	}

	scope, err := s.production.Authorize(ctx, actor, permission)
	if err != nil {
		return production.Scope{}, ErrForbidden
	}

	return scope, nil
}

// AuthorizeBranch authorizes a permission and confirms the scope reaches the
// branch being acted on.
//
// This is the branch-escape guard, and it is why a branch manager holding
// vendor.delivery.policy.manage for branch A cannot rewrite branch B's
// delivery charge.
func (s *Service) AuthorizeBranch(
	ctx context.Context,
	actor production.Actor,
	permission, branchID string,
) (production.Scope, error) {
	scope, err := s.Authorize(ctx, actor, permission)
	if err != nil {
		return production.Scope{}, err
	}

	if scope.Empty() || strings.TrimSpace(branchID) == "" ||
		!scope.Covers(branchID) {
		return production.Scope{}, ErrForbidden
	}

	return scope, nil
}

const policyColumns = `id, business_id, branch_id, enabled, currency,
	delivery_mode, fee_rule, operating_timezone,
	base_fee_minor, per_km_fee_minor, minimum_order_minor,
	free_delivery_threshold_minor, max_delivery_radius_meters,
	max_package_weight_grams, max_package_count, max_volume_ml,
	estimated_minutes_base, estimated_minutes_per_km, rider_availability,
	service_zones, distance_tiers, operating_periods,
	supported_vehicle_classes, fallback_triggers,
	hybrid_fallback_funding_sponsor, hybrid_vendor_share_bps, hybrid_platform_approval_ref,
	created_at, updated_at, version`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row rowScanner) (Policy, error) {
	var policy Policy
	var zones, tiers, periods, vehicles, triggers []byte

	err := row.Scan(
		&policy.ID,
		&policy.BusinessID,
		&policy.BranchID,
		&policy.Enabled,
		&policy.Currency,
		&policy.Mode,
		&policy.FeeRule,
		&policy.OperatingTimezone,
		&policy.BaseFeeMinor,
		&policy.PerKmFeeMinor,
		&policy.MinimumOrderMinor,
		&policy.FreeDeliveryThresholdMinor,
		&policy.MaxDeliveryRadiusMeters,
		&policy.MaxPackageWeightGrams,
		&policy.MaxPackageCount,
		&policy.MaxVolumeMl,
		&policy.EstimatedMinutesBase,
		&policy.EstimatedMinutesPerKm,
		&policy.RiderAvailability,
		&zones,
		&tiers,
		&periods,
		&vehicles,
		&triggers,
		&policy.HybridFallbackFunding,
		&policy.HybridVendorShareBPS,
		&policy.HybridPlatformApprovalRef,
		&policy.CreatedAt,
		&policy.UpdatedAt,
		&policy.Version,
	)
	if err != nil {
		return Policy{}, err
	}

	policy.Currency = strings.ToUpper(strings.TrimSpace(policy.Currency))
	if policy.OperatingTimezone == "" {
		policy.OperatingTimezone = "UTC"
	}
	if policy.Mode == "" {
		policy.Mode = ModeVendor
	}
	if policy.FeeRule == "" {
		policy.FeeRule = FeeDistanceTier
	}
	if policy.RiderAvailability == "" {
		policy.RiderAvailability = RiderUnknown
	}
	if policy.HybridFallbackFunding == "" {
		policy.HybridFallbackFunding = FallbackFundingVendor
		policy.HybridVendorShareBPS = 10000
	}
	_ = json.Unmarshal(zones, &policy.ServiceZones)
	_ = json.Unmarshal(tiers, &policy.DistanceTiers)
	_ = json.Unmarshal(periods, &policy.OperatingPeriods)
	_ = json.Unmarshal(vehicles, &policy.SupportedVehicleClasses)
	_ = json.Unmarshal(triggers, &policy.FallbackTriggers)

	return policy, nil
}

// GetPolicy reads one branch's policy.
//
// A branch the actor cannot reach and a branch with no policy return the same
// ErrNotFound, so the API cannot be used to enumerate another branch.
func (s *Service) GetPolicy(
	ctx context.Context,
	actor production.Actor,
	branchID string,
) (Policy, error) {
	if s == nil || s.db == nil {
		return Policy{}, ErrNotFound
	}

	if _, err := s.AuthorizeBranch(
		ctx, actor, PermPolicyRead, branchID,
	); err != nil {
		return Policy{}, ErrNotFound
	}

	policy, err := scanPolicy(s.db.QueryRow(ctx, `
		SELECT `+policyColumns+`
		  FROM vendor_delivery_policies
		 WHERE business_id = $1 AND branch_id = $2`,
		actor.VendorID, branchID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}

	return policy, err
}

// ResolvePolicy reads a policy for planning, without a staff actor.
//
// Planning runs on behalf of a customer, not a vendor staff member, so there
// is no staff permission to evaluate. Tenant scoping is still explicit: the
// business id is the server-resolved Section 17B group evidence, never a
// customer-supplied value. An absent policy is "this vendor does not offer
// own delivery here", which is a normal planning answer rather than an error.
func (s *Service) ResolvePolicy(
	ctx context.Context,
	businessID, branchID string,
) (Policy, bool, error) {
	if s == nil || s.db == nil {
		return Policy{}, false, nil
	}

	businessID = strings.TrimSpace(businessID)
	branchID = strings.TrimSpace(branchID)

	if businessID == "" || branchID == "" {
		return Policy{}, false, nil
	}

	policy, err := scanPolicy(s.db.QueryRow(ctx, `
		SELECT `+policyColumns+`
		  FROM vendor_delivery_policies
		 WHERE business_id = $1 AND branch_id = $2`,
		businessID, branchID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, err
	}

	return policy, true, nil
}

func (in PolicyInput) validate() error {
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))

	if len(currency) != 3 {
		return ErrInvalid
	}

	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return ErrInvalid
		}
	}

	if in.BaseFeeMinor < 0 || in.PerKmFeeMinor < 0 ||
		in.MinimumOrderMinor < 0 {
		return ErrInvalid
	}

	if in.FreeDeliveryThresholdMinor != nil {
		if *in.FreeDeliveryThresholdMinor < 0 {
			return ErrInvalid
		}

		// A free-delivery threshold below the minimum order can never be
		// reached on its own terms.
		if in.MinimumOrderMinor > 0 &&
			*in.FreeDeliveryThresholdMinor < in.MinimumOrderMinor {
			return ErrInvalid
		}
	}

	if in.MaxDeliveryRadiusMeters != nil &&
		(*in.MaxDeliveryRadiusMeters < 1 ||
			*in.MaxDeliveryRadiusMeters > 200000) {
		return ErrInvalid
	}

	if in.MaxPackageWeightGrams != nil &&
		(*in.MaxPackageWeightGrams < 1 ||
			*in.MaxPackageWeightGrams > 100000000) {
		return ErrInvalid
	}

	if in.EstimatedMinutesBase < 0 || in.EstimatedMinutesBase > 1440 ||
		in.EstimatedMinutesPerKm < 0 || in.EstimatedMinutesPerKm > 1440 {
		return ErrInvalid
	}

	policy := Policy{
		Mode: in.Mode, FeeRule: in.FeeRule, Currency: currency,
		BaseFeeMinor: in.BaseFeeMinor, PerKmFeeMinor: in.PerKmFeeMinor,
		MinimumOrderMinor:          in.MinimumOrderMinor,
		FreeDeliveryThresholdMinor: in.FreeDeliveryThresholdMinor,
		MaxDeliveryRadiusMeters:    in.MaxDeliveryRadiusMeters,
		MaxPackageWeightGrams:      in.MaxPackageWeightGrams,
		MaxPackageCount:            in.MaxPackageCount, MaxVolumeMl: in.MaxVolumeMl,
		OperatingTimezone: in.OperatingTimezone, ServiceZones: in.ServiceZones,
		DistanceTiers: in.DistanceTiers, OperatingPeriods: in.OperatingPeriods,
		SupportedVehicleClasses:   in.SupportedVehicleClasses,
		FallbackTriggers:          in.FallbackTriggers,
		HybridFallbackFunding:     in.HybridFallbackFunding,
		HybridVendorShareBPS:      in.HybridVendorShareBPS,
		HybridPlatformApprovalRef: in.HybridPlatformApprovalRef,
		RiderAvailability:         in.RiderAvailability,
	}
	// Vendor IAM cannot authorize Platform spend. Platform/combined policies
	// may exist only as pre-authorized projections written by the platform
	// authority, never through this public Vendor command.
	if policy.HybridFallbackFunding != "" && policy.HybridFallbackFunding != FallbackFundingVendor {
		return ErrForbidden
	}
	if policy.HybridFallbackFunding == "" {
		policy.HybridFallbackFunding = FallbackFundingVendor
		policy.HybridVendorShareBPS = 10000
	}
	if policy.Mode == "" {
		policy.Mode = ModeVendor
	}
	if policy.FeeRule == "" {
		policy.FeeRule = FeeDistanceTier
	}
	if policy.OperatingTimezone == "" {
		policy.OperatingTimezone = "UTC"
	}
	if err := policy.Validate(); err != nil {
		return ErrInvalid
	}

	return nil
}

// UpsertPolicy creates or replaces one branch's policy.
//
// expectedVersion is required and is the whole point of the operation: a
// delivery charge is what a customer is asked to pay, so two staff members
// editing it at once must collide rather than silently overwrite each other.
// Zero means "I believe no policy exists yet"; a create therefore fails if one
// has appeared in the meantime.
func (s *Service) UpsertPolicy(
	ctx context.Context,
	actor production.Actor,
	branchID string,
	expectedVersion int64,
	in PolicyInput,
) (Policy, error) {
	if s == nil || s.db == nil {
		return Policy{}, ErrNotFound
	}

	if !s.enabled {
		return Policy{}, ErrDisabled
	}

	if err := in.validate(); err != nil {
		return Policy{}, err
	}

	if _, err := s.AuthorizeBranch(
		ctx, actor, PermPolicyManage, branchID,
	); err != nil {
		return Policy{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// The branch must belong to this tenant. Checked in SQL against the
	// tenant-scoped key rather than trusted from the path, so a branch id
	// borrowed from another business resolves to nothing.
	var branchExists bool

	err = tx.QueryRow(ctx, `
		SELECT true FROM vendor_branches
		 WHERE id = $1 AND business_id = $2`,
		branchID, actor.VendorID,
	).Scan(&branchExists)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, err
	}

	before, existed, err := lockExisting(ctx, tx, actor.VendorID, branchID)
	if err != nil {
		return Policy{}, err
	}

	if existed && before.Version != expectedVersion {
		return Policy{}, ErrVersionConflict
	}

	if !existed && expectedVersion != 0 {
		return Policy{}, ErrVersionConflict
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	mode := in.Mode
	if mode == "" {
		mode = ModeVendor
	}
	feeRule := in.FeeRule
	if feeRule == "" {
		feeRule = FeeDistanceTier
	}
	timezone := strings.TrimSpace(in.OperatingTimezone)
	if timezone == "" {
		timezone = "UTC"
	}
	zones, _ := json.Marshal(in.ServiceZones)
	tiers, _ := json.Marshal(in.DistanceTiers)
	periods, _ := json.Marshal(in.OperatingPeriods)
	vehicles, _ := json.Marshal(in.SupportedVehicleClasses)
	triggers, _ := json.Marshal(in.FallbackTriggers)
	rider := in.RiderAvailability
	if rider == "" {
		rider = RiderUnknown
	}
	hybridSponsor := in.HybridFallbackFunding
	hybridVendorShare := in.HybridVendorShareBPS
	if hybridSponsor == "" {
		hybridSponsor = FallbackFundingVendor
		hybridVendorShare = 10000
	}
	if hybridSponsor == FallbackFundingVendor && hybridVendorShare == 0 {
		hybridVendorShare = 10000
	}

	var after Policy

	if existed {
		after, err = scanPolicy(tx.QueryRow(ctx, `
		UPDATE vendor_delivery_policies
		   SET enabled = $3,
		       currency = $4,
		       delivery_mode = $5, fee_rule = $6, operating_timezone = $7,
		       base_fee_minor = $8, per_km_fee_minor = $9,
		       minimum_order_minor = $10,
		       free_delivery_threshold_minor = $11,
		       max_delivery_radius_meters = $12,
		       max_package_weight_grams = $13,
		       max_package_count = $14, max_volume_ml = $15,
		       estimated_minutes_base = $16,
		       estimated_minutes_per_km = $17,
		       rider_availability = $18,
		       service_zones = $19, distance_tiers = $20,
		       operating_periods = $21, supported_vehicle_classes = $22,
		       fallback_triggers = $23,
		       hybrid_fallback_funding_sponsor = $24,
		       hybrid_vendor_share_bps = $25,
		       hybrid_platform_approval_ref = $26,
		       updated_at = now(),
			       version = version + 1
			 WHERE business_id = $1 AND branch_id = $2
			RETURNING `+policyColumns,
			actor.VendorID, branchID,
			in.Enabled, currency, mode, feeRule, timezone,
			in.BaseFeeMinor, in.PerKmFeeMinor, in.MinimumOrderMinor,
			in.FreeDeliveryThresholdMinor, in.MaxDeliveryRadiusMeters,
			in.MaxPackageWeightGrams, in.MaxPackageCount, in.MaxVolumeMl,
			in.EstimatedMinutesBase, in.EstimatedMinutesPerKm, rider,
			zones, tiers, periods, vehicles, triggers,
			hybridSponsor, hybridVendorShare, "",
		))
	} else {
		after, err = scanPolicy(tx.QueryRow(ctx, `
			INSERT INTO vendor_delivery_policies (
				id, business_id, branch_id, enabled, currency,
				delivery_mode, fee_rule, operating_timezone,
				base_fee_minor, per_km_fee_minor, minimum_order_minor,
				free_delivery_threshold_minor, max_delivery_radius_meters,
				max_package_weight_grams, max_package_count, max_volume_ml,
				estimated_minutes_base, estimated_minutes_per_km, rider_availability,
				service_zones, distance_tiers, operating_periods,
				supported_vehicle_classes, fallback_triggers,
				hybrid_fallback_funding_sponsor, hybrid_vendor_share_bps, hybrid_platform_approval_ref
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
			RETURNING `+policyColumns,
			"vdp_"+uuid.NewString(), actor.VendorID, branchID,
			in.Enabled, currency, mode, feeRule, timezone,
			in.BaseFeeMinor, in.PerKmFeeMinor, in.MinimumOrderMinor,
			in.FreeDeliveryThresholdMinor, in.MaxDeliveryRadiusMeters,
			in.MaxPackageWeightGrams, in.MaxPackageCount, in.MaxVolumeMl,
			in.EstimatedMinutesBase, in.EstimatedMinutesPerKm, rider,
			zones, tiers, periods, vehicles, triggers,
			hybridSponsor, hybridVendorShare, "",
		))
	}
	if err != nil {
		return Policy{}, err
	}

	var beforeEvidence any

	if existed {
		beforeEvidence = before
	}

	// before/after are both recorded: a delivery-charge change must be
	// auditable as "from X to Y", not merely "changed".
	if err = s.production.RecordMutation(
		ctx, tx, actor,
		AuditPolicyUpserted, EventPolicyChanged,
		AggregateType, after.ID, after.BranchID, after.Version,
		beforeEvidence, after, after, in.Reason,
	); err != nil {
		return Policy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Policy{}, err
	}

	return after, nil
}

func lockExisting(
	ctx context.Context,
	tx pgx.Tx,
	businessID, branchID string,
) (Policy, bool, error) {
	policy, err := scanPolicy(tx.QueryRow(ctx, `
		SELECT `+policyColumns+`
		  FROM vendor_delivery_policies
		 WHERE business_id = $1 AND branch_id = $2
		   FOR UPDATE`,
		businessID, branchID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, err
	}

	return policy, true, nil
}
