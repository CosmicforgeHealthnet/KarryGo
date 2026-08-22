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
	base_fee_minor, per_km_fee_minor, minimum_order_minor,
	free_delivery_threshold_minor, max_delivery_radius_meters,
	max_package_weight_grams, estimated_minutes_base, estimated_minutes_per_km,
	created_at, updated_at, version`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row rowScanner) (Policy, error) {
	var policy Policy

	err := row.Scan(
		&policy.ID,
		&policy.BusinessID,
		&policy.BranchID,
		&policy.Enabled,
		&policy.Currency,
		&policy.BaseFeeMinor,
		&policy.PerKmFeeMinor,
		&policy.MinimumOrderMinor,
		&policy.FreeDeliveryThresholdMinor,
		&policy.MaxDeliveryRadiusMeters,
		&policy.MaxPackageWeightGrams,
		&policy.EstimatedMinutesBase,
		&policy.EstimatedMinutesPerKm,
		&policy.CreatedAt,
		&policy.UpdatedAt,
		&policy.Version,
	)
	if err != nil {
		return Policy{}, err
	}

	policy.Currency = strings.ToUpper(strings.TrimSpace(policy.Currency))

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

	var after Policy

	if existed {
		after, err = scanPolicy(tx.QueryRow(ctx, `
			UPDATE vendor_delivery_policies
			   SET enabled = $3,
			       currency = $4,
			       base_fee_minor = $5,
			       per_km_fee_minor = $6,
			       minimum_order_minor = $7,
			       free_delivery_threshold_minor = $8,
			       max_delivery_radius_meters = $9,
			       max_package_weight_grams = $10,
			       estimated_minutes_base = $11,
			       estimated_minutes_per_km = $12,
			       updated_at = now(),
			       version = version + 1
			 WHERE business_id = $1 AND branch_id = $2
			RETURNING `+policyColumns,
			actor.VendorID, branchID,
			in.Enabled, currency,
			in.BaseFeeMinor, in.PerKmFeeMinor, in.MinimumOrderMinor,
			in.FreeDeliveryThresholdMinor, in.MaxDeliveryRadiusMeters,
			in.MaxPackageWeightGrams,
			in.EstimatedMinutesBase, in.EstimatedMinutesPerKm,
		))
	} else {
		after, err = scanPolicy(tx.QueryRow(ctx, `
			INSERT INTO vendor_delivery_policies (
				id, business_id, branch_id, enabled, currency,
				base_fee_minor, per_km_fee_minor, minimum_order_minor,
				free_delivery_threshold_minor, max_delivery_radius_meters,
				max_package_weight_grams,
				estimated_minutes_base, estimated_minutes_per_km
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING `+policyColumns,
			"vdp_"+uuid.NewString(), actor.VendorID, branchID,
			in.Enabled, currency,
			in.BaseFeeMinor, in.PerKmFeeMinor, in.MinimumOrderMinor,
			in.FreeDeliveryThresholdMinor, in.MaxDeliveryRadiusMeters,
			in.MaxPackageWeightGrams,
			in.EstimatedMinutesBase, in.EstimatedMinutesPerKm,
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
