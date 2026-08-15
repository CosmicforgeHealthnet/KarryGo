package crm

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Loyalty tiers are a vendor-configured, ordered set of thresholds over the
// loyalty ledger the Section 8D engine already maintains.
//
// Three decisions shape everything below.
//
// First, a tier is DERIVED, never stored on the customer. There is no column
// anywhere holding "this customer is Gold". Membership is computed from the
// account's lifetime credited points against the current definitions, so a
// reversal, a merge or a threshold change is reflected the next time anyone
// asks, and there is no second copy of the truth to drift. This costs one
// cheap join and removes an entire class of "the tier says Gold but the ledger
// says otherwise" defects.
//
// Second, the input is LIFETIME CREDITED POINTS NET OF REVERSED CREDIT — not
// the current balance. The original specification allowed either and required
// that one be chosen and documented, and this is the choice: status recognises
// what a customer has done over the relationship, so spending points must not
// demote them. Section 8D already maintains lifetime_credited_points and
// decrements it when a credit is reversed, which is exactly the quantity this
// needs; expiry and ordinary redemption move current_points and deliberately
// do not touch tier standing.
//
// Third, a tier confers RECOGNITION, NOT BENEFITS. There is no discount,
// entitlement, multiplier or monetary field here and there is no benefits
// engine. Points are not money — payment-wallet-service is the only monetary
// authority on this platform — and a tier that silently granted value would
// have made this CRM feature a commerce feature.

// MaxLoyaltyTiers bounds a definition set. Eight is generous for the product
// and small enough that the whole ladder is legible at a glance; an unbounded
// set would let a vendor build a per-customer ladder, which is a segment.
const MaxLoyaltyTiers = 8

// LoyaltyTierDefinition is one rung of a business's tier ladder.
type LoyaltyTierDefinition struct {
	ID                    string    `json:"id"`
	TierKey               string    `json:"tier_key"`
	DisplayName           string    `json:"display_name"`
	MinimumLifetimePoints int64     `json:"minimum_lifetime_points"`
	Rank                  int       `json:"rank"`
	Status                string    `json:"status"`
	Version               int64     `json:"version"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// LoyaltyTierSet is the whole ladder for one business, which is the only unit
// worth reading or writing: a single rung is meaningless without its neighbours
// and changing one in isolation is how ladders end up non-monotonic.
type LoyaltyTierSet struct {
	Tiers     []LoyaltyTierDefinition `json:"tiers"`
	Version   int64                   `json:"version"`
	UpdatedAt *time.Time              `json:"updated_at,omitempty"`
}

// LoyaltyTierInput is one requested rung.
type LoyaltyTierInput struct {
	TierKey               string `json:"tier_key"`
	DisplayName           string `json:"display_name"`
	MinimumLifetimePoints int64  `json:"minimum_lifetime_points"`
}

// CustomerLoyaltyTier is a relationship's derived standing.
//
// State is three-valued for the reason that governs this whole section:
// NOT_AVAILABLE (loyalty disabled, or no ladder configured) is a different
// statement from NO_TIER (a configured ladder this customer has not yet
// climbed), and neither may be presented as the bottom rung.
type CustomerLoyaltyTier struct {
	State                string `json:"state"`
	TierKey              string `json:"tier_key,omitempty"`
	DisplayName          string `json:"display_name,omitempty"`
	Rank                 int    `json:"rank,omitempty"`
	LifetimePoints       *int64 `json:"lifetime_points,omitempty"`
	NextTierKey          string `json:"next_tier_key,omitempty"`
	PointsToNextTier     *int64 `json:"points_to_next_tier,omitempty"`
	ConfiguredTierCount  int    `json:"configured_tier_count"`
	DefinitionSetVersion int64  `json:"definition_set_version,omitempty"`
}

// Loyalty tier availability states.
const (
	LoyaltyTierNotAvailable = "NOT_AVAILABLE"
	LoyaltyTierNone         = "NO_TIER"
	LoyaltyTierCurrent      = "CURRENT"
)

// Section 8 completion error codes. These reuse the existing CRM envelope and
// are deliberately few: a caller needs to distinguish "the source has never
// reported this" from "you asked for something invalid", and little else.
const (
	CodeLoyaltyTierConfigInvalid       = "CRM_LOYALTY_TIER_CONFIG_INVALID"
	CodeCustomerValueNotAvailable      = "CRM_CUSTOMER_VALUE_NOT_AVAILABLE"
	CodeRelationshipSignalNotAvailable = "CRM_RELATIONSHIP_SIGNAL_NOT_AVAILABLE"
	CodeCRMSourceNotAvailable          = "CRM_SOURCE_NOT_AVAILABLE"
)

var tierKeyPattern = func(s string) bool {
	if len(s) < 2 || len(s) > 40 {
		return false
	}
	if s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

// ValidateLoyaltyTierInputs enforces the shape of a ladder before any of it
// reaches the database.
//
// The database also enforces uniqueness of key, rank and threshold, but a
// constraint violation surfacing as a 500 is a poor contract, and monotonicity
// specifically cannot be expressed as a per-row constraint at all — it is a
// property of the set. Doing it here means the caller gets a field-level
// explanation of which rung is wrong.
func ValidateLoyaltyTierInputs(inputs []LoyaltyTierInput) ([]LoyaltyTierInput, error) {
	if len(inputs) > MaxLoyaltyTiers {
		return nil, crmError(apperrors.Validation(fmt.Sprintf(
			"A business may define at most %d loyalty tiers.", MaxLoyaltyTiers), nil),
			CodeLoyaltyTierConfigInvalid)
	}

	cleaned := make([]LoyaltyTierInput, 0, len(inputs))
	seenKeys := map[string]bool{}
	seenThresholds := map[int64]bool{}

	for i, in := range inputs {
		key := strings.ToUpper(strings.TrimSpace(in.TierKey))
		name := strings.TrimSpace(in.DisplayName)

		if !tierKeyPattern(key) {
			return nil, crmError(apperrors.Validation(fmt.Sprintf(
				"Tier %d: key must be 2-40 characters of A-Z, 0-9 and underscore, starting with a letter.", i+1), nil),
				CodeLoyaltyTierConfigInvalid)
		}
		if name == "" || len(name) > 80 {
			return nil, crmError(apperrors.Validation(fmt.Sprintf(
				"Tier %d: display name is required and must be at most 80 characters.", i+1), nil),
				CodeLoyaltyTierConfigInvalid)
		}
		if in.MinimumLifetimePoints < 0 {
			return nil, crmError(apperrors.Validation(fmt.Sprintf(
				"Tier %d: minimum lifetime points cannot be negative.", i+1), nil),
				CodeLoyaltyTierConfigInvalid)
		}
		if seenKeys[key] {
			return nil, crmError(apperrors.Validation(fmt.Sprintf(
				"Tier key %q appears more than once.", key), nil), CodeLoyaltyTierConfigInvalid)
		}
		// Two rungs at the same threshold make membership ambiguous, and an
		// ambiguous customer-facing status is worse than having no tiers.
		if seenThresholds[in.MinimumLifetimePoints] {
			return nil, crmError(apperrors.Validation(fmt.Sprintf(
				"Two tiers share the threshold %d. Each tier needs its own entry point.",
				in.MinimumLifetimePoints), nil), CodeLoyaltyTierConfigInvalid)
		}
		seenKeys[key] = true
		seenThresholds[in.MinimumLifetimePoints] = true
		cleaned = append(cleaned, LoyaltyTierInput{
			TierKey:               key,
			DisplayName:           name,
			MinimumLifetimePoints: in.MinimumLifetimePoints,
		})
	}

	// Rank is derived from the thresholds rather than accepted from the caller.
	// A caller-supplied rank could disagree with its own threshold ordering,
	// and then the ladder would have two answers to "which tier is higher".
	sort.Slice(cleaned, func(a, b int) bool {
		return cleaned[a].MinimumLifetimePoints < cleaned[b].MinimumLifetimePoints
	})
	return cleaned, nil
}

// GetLoyaltyTiers returns the active ladder for a business.
func (s *Service) GetLoyaltyTiers(ctx context.Context, businessID string) (LoyaltyTierSet, error) {
	rows, err := s.db.Query(ctx, `
        SELECT id, tier_key, display_name, minimum_lifetime_points, rank, status, version, updated_at
          FROM crm_loyalty_tier_definitions
         WHERE business_id = $1 AND status = 'ACTIVE'
         ORDER BY rank`, businessID)
	if err != nil {
		return LoyaltyTierSet{}, fmt.Errorf("list loyalty tiers: %w", err)
	}
	defer rows.Close()

	set := LoyaltyTierSet{Tiers: []LoyaltyTierDefinition{}}
	for rows.Next() {
		var t LoyaltyTierDefinition
		if err := rows.Scan(&t.ID, &t.TierKey, &t.DisplayName, &t.MinimumLifetimePoints,
			&t.Rank, &t.Status, &t.Version, &t.UpdatedAt); err != nil {
			return LoyaltyTierSet{}, fmt.Errorf("scan loyalty tier: %w", err)
		}
		set.Tiers = append(set.Tiers, t)
		// The set's version is the highest rung version, so a caller that read
		// the ladder and writes it back is checked against every rung it saw.
		if t.Version > set.Version {
			set.Version = t.Version
		}
		if set.UpdatedAt == nil || t.UpdatedAt.After(*set.UpdatedAt) {
			at := t.UpdatedAt
			set.UpdatedAt = &at
		}
	}
	return set, rows.Err()
}

// ReplaceLoyaltyTiers atomically replaces the whole ladder.
//
// Replacement rather than per-rung editing is deliberate. A ladder is only
// valid as a set — thresholds must stay distinct and ordered — so an API that
// let a caller move one rung would have to either reject most edits or accept
// a temporarily invalid ladder. Replacing the set makes every write a
// transition between two valid states.
//
// Previous rungs are ARCHIVED, not deleted: a task or audit record that
// referenced a tier keeps something to point at.
func (s *Service) ReplaceLoyaltyTiers(
	ctx context.Context,
	actor Actor,
	inputs []LoyaltyTierInput,
	expectedVersion *int64,
) (LoyaltyTierSet, error) {
	cleaned, err := ValidateLoyaltyTierInputs(inputs)
	if err != nil {
		return LoyaltyTierSet{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LoyaltyTierSet{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialise concurrent ladder writes for this business. Without it two
	// replacements could each archive the other's rungs and leave a ladder
	// that is the union of both.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1 || ':loyalty_tiers'))`,
		actor.VendorID); err != nil {
		return LoyaltyTierSet{}, fmt.Errorf("lock tier set: %w", err)
	}

	var currentVersion int64
	var currentCount int
	if err := tx.QueryRow(ctx, `
        SELECT COALESCE(MAX(version), 0), COUNT(*)
          FROM crm_loyalty_tier_definitions
         WHERE business_id = $1 AND status = 'ACTIVE'`,
		actor.VendorID).Scan(&currentVersion, &currentCount); err != nil {
		return LoyaltyTierSet{}, fmt.Errorf("read current tier version: %w", err)
	}

	if expectedVersion != nil && *expectedVersion != currentVersion {
		return LoyaltyTierSet{}, crmError(apperrors.Conflict(
			"The loyalty tiers changed since you loaded them. Reload and try again.", nil),
			CodeLoyaltyTierConfigInvalid)
	}

	if _, err := tx.Exec(ctx, `
        UPDATE crm_loyalty_tier_definitions
           SET status = 'ARCHIVED', updated_at = now()
         WHERE business_id = $1 AND status = 'ACTIVE'`, actor.VendorID); err != nil {
		return LoyaltyTierSet{}, fmt.Errorf("archive previous tiers: %w", err)
	}

	newVersion := currentVersion + 1
	for i, in := range cleaned {
		rank := i + 1
		id := newID("crmtier_")
		if _, err := tx.Exec(ctx, `
            INSERT INTO crm_loyalty_tier_definitions
                (id, business_id, tier_key, display_name, minimum_lifetime_points,
                 rank, status, version, created_at, updated_at)
            VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE',$7, now(), now())`,
			id, actor.VendorID, in.TierKey, in.DisplayName,
			in.MinimumLifetimePoints, rank, newVersion); err != nil {
			return LoyaltyTierSet{}, fmt.Errorf("insert loyalty tier: %w", err)
		}
	}

	// A threshold change moves customers between rungs, so anything derived
	// from tier standing has to be recomputed. Tier membership itself is
	// derived at read time and needs nothing; what does need telling is the
	// segment engine, whose materialised memberships were computed against the
	// old ladder.
	if err := s.invalidateSegmentsForDependency(ctx, tx, actor.VendorID, DependencyLoyaltyTier); err != nil {
		return LoyaltyTierSet{}, err
	}

	if err := s.auditTx(ctx, tx, actor, "vendor.crm.loyalty.tiers.updated",
		"crm_loyalty_tier_definitions", actor.VendorID, "", nil,
		map[string]any{
			"tier_count":       len(cleaned),
			"previous_count":   currentCount,
			"version":          newVersion,
			"previous_version": currentVersion,
		}, ""); err != nil {
		return LoyaltyTierSet{}, err
	}

	if err := s.publishTx(ctx, tx, actor, "vendor.crm.loyalty.tiers.updated",
		"crm_loyalty_tier_definitions", actor.VendorID, newVersion,
		map[string]any{
			"business_ref": actor.VendorID,
			"tier_count":   len(cleaned),
			"version":      newVersion,
		}); err != nil {
		return LoyaltyTierSet{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return LoyaltyTierSet{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetLoyaltyTiers(ctx, actor.VendorID)
}

// ResolveCustomerLoyaltyTier derives a relationship's standing from the ledger.
//
// Nothing is stored. The account's lifetime credited points are read and
// compared against the current ladder, which is what makes a credit, a
// reversal, a merge or a threshold change all correct immediately without a
// recalculation job.
func (s *Service) ResolveCustomerLoyaltyTier(
	ctx context.Context,
	businessID string,
	profileID string,
) (CustomerLoyaltyTier, error) {
	set, err := s.GetLoyaltyTiers(ctx, businessID)
	if err != nil {
		return CustomerLoyaltyTier{}, err
	}
	// No ladder configured is not the bottom rung. It is the absence of the
	// whole concept for this business, and presenting it as "Bronze" would
	// invent a status the vendor never defined.
	if len(set.Tiers) == 0 {
		return CustomerLoyaltyTier{State: LoyaltyTierNotAvailable}, nil
	}

	// Standing is the sum of credit-direction entries that have NOT been
	// reversed — not the account's lifetime_credited_points column.
	//
	// The distinction is load-bearing and a test caught it: Section 8D's ledger
	// is immutable, so reversing a credit posts a compensating REVERSAL entry
	// rather than decrementing the lifetime counter. Reading that counter would
	// therefore have let a credit posted in error confer permanent status, which
	// is precisely what a reversal exists to undo.
	//
	// Computing it from the entries instead gets all three cases right at once:
	// redemptions and expiries are debits and never appear, so spending points
	// cannot demote; a reversed credit is excluded, so an erroneous credit
	// confers nothing; and MERGE_IN is a credit, so a relationship that absorbed
	// another keeps the standing it legitimately earned.
	var lifetime int64
	var status string
	err = s.db.QueryRow(ctx, `
        SELECT COALESCE((
                   SELECT SUM(e.delta_points)
                     FROM crm_loyalty_entries e
                    WHERE e.business_id = a.business_id
                      AND e.loyalty_account_id = a.id
                      AND e.entry_type IN ('EARN', 'ADJUSTMENT_CREDIT', 'MERGE_IN')
                      AND NOT EXISTS (
                          SELECT 1 FROM crm_loyalty_entries r
                           WHERE r.business_id = e.business_id
                             AND r.reverses_entry_id = e.id)), 0),
               a.status
          FROM crm_loyalty_accounts a
         WHERE a.business_id = $1 AND a.customer_profile_id = $2`,
		businessID, profileID).Scan(&lifetime, &status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// A ladder exists but this relationship has no loyalty account, so it
		// has earned nothing rather than being unrankable. LifetimePoints stays
		// absent — the customer is not enrolled, which is a different statement
		// from being enrolled with zero — but the first rung is still reported,
		// because "what would it take to get on the ladder" is answerable and
		// is exactly what the CRM user is looking at the record to find out.
		out := CustomerLoyaltyTier{
			State:                LoyaltyTierNone,
			ConfiguredTierCount:  len(set.Tiers),
			DefinitionSetVersion: set.Version,
		}
		if first := set.Tiers[0]; first.MinimumLifetimePoints > 0 {
			remaining := first.MinimumLifetimePoints
			out.NextTierKey = first.TierKey
			out.PointsToNextTier = &remaining
		} else {
			// A ladder whose first rung is free is reached by enrolling, so
			// there is nothing left to earn toward it.
			out.NextTierKey = first.TierKey
		}
		return out, nil
	case err != nil:
		return CustomerLoyaltyTier{}, fmt.Errorf("read loyalty account: %w", err)
	}

	// A redacted or merged-away account must not present a status: Section 8D
	// keeps the ledger for lawful reasons, and standing is presentation.
	if status != "ACTIVE" {
		return CustomerLoyaltyTier{State: LoyaltyTierNotAvailable}, nil
	}

	out := CustomerLoyaltyTier{
		State:                LoyaltyTierNone,
		LifetimePoints:       &lifetime,
		ConfiguredTierCount:  len(set.Tiers),
		DefinitionSetVersion: set.Version,
	}

	// Tiers are ordered by ascending threshold, so the last one reached is the
	// customer's rung and the one after it is what they are climbing toward.
	for i, t := range set.Tiers {
		if lifetime >= t.MinimumLifetimePoints {
			out.State = LoyaltyTierCurrent
			out.TierKey = t.TierKey
			out.DisplayName = t.DisplayName
			out.Rank = t.Rank
			out.NextTierKey = ""
			out.PointsToNextTier = nil
			if i+1 < len(set.Tiers) {
				next := set.Tiers[i+1]
				remaining := next.MinimumLifetimePoints - lifetime
				out.NextTierKey = next.TierKey
				out.PointsToNextTier = &remaining
			}
			continue
		}
		if out.State == LoyaltyTierNone {
			remaining := t.MinimumLifetimePoints - lifetime
			out.NextTierKey = t.TierKey
			out.PointsToNextTier = &remaining
		}
		break
	}
	return out, nil
}
