package food

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Operational availability
// ---------------------------------------------------------------------------
//
// Kept in its own row, and its own version fence, away from the branch price
// override. Marking a dish sold out happens many times a service and is
// kitchen work; changing its price is a rare commercial decision. Sharing a row
// would mean a cook's toggle and a manager's price edit fighting over one
// expected_version.

// ItemAvailability is the item's current operational state at its branch.
type ItemAvailability struct {
	ItemID         string
	BusinessID     string
	BranchID       string
	Status         string
	Reason         string
	EffectiveUntil *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
}

// Expired reports a temporary state whose own deadline has passed.
//
// The resolver treats an expired override as absent rather than waiting for a
// worker to clear it, so correctness never depends on a sweeper running on
// time. Clearing the row is housekeeping; this is the truth.
func (a ItemAvailability) Expired(at time.Time) bool {
	return a.EffectiveUntil != nil && !a.EffectiveUntil.After(at)
}

// Effective collapses status and expiry into the state that actually applies.
func (a ItemAvailability) Effective(at time.Time) string {
	if a.Status == OperationalAvailable || a.Expired(at) {
		return OperationalAvailable
	}
	return a.Status
}

type ItemAvailabilityView struct {
	ItemID          string     `json:"item_id"`
	BranchID        string     `json:"branch_id"`
	Status          string     `json:"status"`
	EffectiveStatus string     `json:"effective_status"`
	Reason          string     `json:"reason"`
	EffectiveUntil  *time.Time `json:"effective_until"`
	Expired         bool       `json:"expired"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Version         int64      `json:"version"`
}

func ItemAvailabilityViewOf(a ItemAvailability, at time.Time) ItemAvailabilityView {
	return ItemAvailabilityView{ItemID: a.ItemID, BranchID: a.BranchID, Status: a.Status,
		EffectiveStatus: a.Effective(at), Reason: a.Reason, EffectiveUntil: a.EffectiveUntil,
		Expired: a.Expired(at), UpdatedAt: a.UpdatedAt, Version: a.Version}
}

const availabilityCols = `item_id,business_id,branch_id,status,reason,effective_until,created_at,updated_at,version`

func scanAvailability(row rowScanner) (ItemAvailability, error) {
	var a ItemAvailability
	err := row.Scan(&a.ItemID, &a.BusinessID, &a.BranchID, &a.Status, &a.Reason,
		&a.EffectiveUntil, &a.CreatedAt, &a.UpdatedAt, &a.Version)
	return a, err
}

// loadAvailability returns the stored row, or the implicit AVAILABLE default at
// version 0 for an item nobody has ever touched.
//
// Version 0 is meaningful: it says "no override has ever been written", so a
// first write can be made with expected_version 0 without the caller having to
// guess whether a row exists.
func loadAvailability(ctx context.Context, q querier, item Item, lock bool) (ItemAvailability, error) {
	sql := `SELECT ` + availabilityCols + ` FROM food_item_availability WHERE business_id=$1 AND item_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	availability, err := scanAvailability(q.QueryRow(ctx, sql, item.BusinessID, item.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ItemAvailability{ItemID: item.ID, BusinessID: item.BusinessID, BranchID: item.BranchID,
			Status: OperationalAvailable, Version: 0}, nil
	}
	if err != nil {
		return ItemAvailability{}, mapDB(err, "Item availability", CodeItemNotFound)
	}
	return availability, nil
}

// GetItemAvailability reads the operational state.
func (s *Service) GetItemAvailability(ctx context.Context, actor Actor, scope Scope, itemID string) (ItemAvailability, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return ItemAvailability{}, err
	}
	if !scope.Covers(item.BranchID) {
		return ItemAvailability{}, errNotFound("Food item", CodeItemNotFound)
	}
	return loadAvailability(ctx, s.db, item, false)
}

// SetAvailabilityInput is one operational change.
type SetAvailabilityInput struct {
	Status          string
	Reason          string
	EffectiveUntil  *time.Time
	ExpectedVersion int64
}

// SetItemAvailability applies SOLD_OUT, PAUSED or a restore to AVAILABLE.
//
// The state, its audit record, its outbox event and the catalogue revision all
// commit in one transaction, so the moment the call returns the authoritative
// answer to "can this be sold" has already changed. Nothing here depends on a
// message being delivered somewhere else first.
func (s *Service) SetItemAvailability(ctx context.Context, actor Actor, scope Scope, itemID string, in SetAvailabilityInput) (ItemAvailability, error) {
	if err := s.requireWrites(); err != nil {
		return ItemAvailability{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(in.Status))
	switch status {
	case OperationalAvailable, OperationalSoldOut, OperationalPaused:
	default:
		return ItemAvailability{}, coded(apperrors.Validation("Check your details.",
			[]apperrors.FieldViolation{{Field: "status",
				Message: "Status must be AVAILABLE, SOLD_OUT or PAUSED."}}), CodeInvalidAvailability)
	}
	reason := strings.TrimSpace(in.Reason)
	if len(reason) > MaxAvailabilityText {
		return ItemAvailability{}, coded(apperrors.Validation("Check your details.",
			[]apperrors.FieldViolation{{Field: "reason", Message: "Reason is too long."}}),
			CodeInvalidAvailability)
	}
	until := in.EffectiveUntil
	if status == OperationalAvailable {
		// "Available until Tuesday" is a schedule, not an availability
		// override, and accepting one here would create a second scheduling
		// system that the resolver does not consult.
		if until != nil {
			return ItemAvailability{}, coded(apperrors.Validation("Check your details.",
				[]apperrors.FieldViolation{{Field: "effective_until",
					Message: "An available item carries no expiry. Use a menu schedule instead."}}),
				CodeInvalidAvailability)
		}
	} else if until != nil && !until.After(s.now()) {
		return ItemAvailability{}, coded(apperrors.Validation("Check your details.",
			[]apperrors.FieldViolation{{Field: "effective_until",
				Message: "The restore time must be in the future."}}), CodeInvalidAvailability)
	}

	var updated ItemAvailability
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		current, err := loadAvailability(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}
		updated, err = scanAvailability(tx.QueryRow(ctx, `
			INSERT INTO food_item_availability
				(item_id,business_id,branch_id,status,reason,effective_until,version)
			VALUES ($1,$2,$3,$4,$5,$6,1)
			ON CONFLICT (item_id) DO UPDATE SET
				status = EXCLUDED.status, reason = EXCLUDED.reason,
				effective_until = EXCLUDED.effective_until,
				updated_at = now(), version = food_item_availability.version + 1
			RETURNING `+availabilityCols,
			item.ID, item.BusinessID, item.BranchID, status, reason, until))
		if err != nil {
			return mapDB(err, "Item availability", CodeItemNotFound)
		}
		catalogue, err := scopeOfItem(ctx, tx, item)
		if err != nil {
			return err
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.availability_changed", eventType: "food.item.availability.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: updated.Version,
			before:  ItemAvailabilityViewOf(current, s.now()), after: ItemAvailabilityViewOf(updated, s.now()),
			payload: map[string]any{"item_id": item.ID, "status": updated.Status,
				"effective_until": updated.EffectiveUntil, "catalogue_revision": revision},
		})
	})
	return updated, err
}

// ---------------------------------------------------------------------------
// Branch item overrides
// ---------------------------------------------------------------------------
//
// Section 9A items are already bound to exactly one branch, so this row is that
// branch's commercial override for the item. It is not a second scope and the
// item was not redesigned into a business-level product to justify one.

// BranchItemOverride is the optional per-branch price and preparation layer.
type BranchItemOverride struct {
	ID                     string
	ItemID                 string
	BusinessID             string
	BranchID               string
	BasePriceOverrideMinor *int64
	PrepMinMinutes         *int
	PrepMaxMinutes         *int
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Version                int64
}

type BranchItemOverrideView struct {
	ItemID                 string    `json:"item_id"`
	BranchID               string    `json:"branch_id"`
	BasePriceOverrideMinor *int64    `json:"base_price_override_minor"`
	InheritsBasePrice      bool      `json:"inherits_base_price"`
	PrepMinMinutes         *int      `json:"prep_min_minutes"`
	PrepMaxMinutes         *int      `json:"prep_max_minutes"`
	InheritsPrepProfile    bool      `json:"inherits_prep_profile"`
	UpdatedAt              time.Time `json:"updated_at"`
	Version                int64     `json:"version"`
}

// BranchItemOverrideViewOf derives the two inheritance flags rather than
// storing them, so a null price and "inherits" can never disagree.
func BranchItemOverrideViewOf(o BranchItemOverride) BranchItemOverrideView {
	return BranchItemOverrideView{ItemID: o.ItemID, BranchID: o.BranchID,
		BasePriceOverrideMinor: o.BasePriceOverrideMinor, InheritsBasePrice: o.BasePriceOverrideMinor == nil,
		PrepMinMinutes: o.PrepMinMinutes, PrepMaxMinutes: o.PrepMaxMinutes,
		InheritsPrepProfile: o.PrepMinMinutes == nil,
		UpdatedAt:           o.UpdatedAt, Version: o.Version}
}

const overrideCols = `id,item_id,business_id,branch_id,base_price_override_minor,
	prep_min_minutes,prep_max_minutes,created_at,updated_at,version`

func scanOverride(row rowScanner) (BranchItemOverride, error) {
	var o BranchItemOverride
	err := row.Scan(&o.ID, &o.ItemID, &o.BusinessID, &o.BranchID, &o.BasePriceOverrideMinor,
		&o.PrepMinMinutes, &o.PrepMaxMinutes, &o.CreatedAt, &o.UpdatedAt, &o.Version)
	return o, err
}

// loadOverride returns the stored row, or an all-inherit placeholder at version
// 0 when no override has ever been written.
func loadOverride(ctx context.Context, q querier, item Item, lock bool) (BranchItemOverride, error) {
	sql := `SELECT ` + overrideCols + ` FROM food_branch_item_overrides WHERE business_id=$1 AND item_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	override, err := scanOverride(q.QueryRow(ctx, sql, item.BusinessID, item.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return BranchItemOverride{ItemID: item.ID, BusinessID: item.BusinessID,
			BranchID: item.BranchID, Version: 0}, nil
	}
	if err != nil {
		return BranchItemOverride{}, mapDB(err, "Branch override", CodeOverrideNotFound)
	}
	return override, nil
}

// GetBranchOverride reads the override layer.
func (s *Service) GetBranchOverride(ctx context.Context, actor Actor, scope Scope, itemID string) (BranchItemOverride, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return BranchItemOverride{}, err
	}
	if !scope.Covers(item.BranchID) {
		return BranchItemOverride{}, errNotFound("Food item", CodeItemNotFound)
	}
	return loadOverride(ctx, s.db, item, false)
}

// SetBranchOverrideInput distinguishes "leave it alone", "set it" and "clear
// it" for each dimension independently.
//
// Without the explicit clear flags a nil pointer would be ambiguous, and a
// branch could never return to inheriting — which is also why zero is never
// overloaded to mean "no override": a zero-priced item is a real decision.
type SetBranchOverrideInput struct {
	BasePriceOverrideMinor *int64
	ClearBasePrice         bool
	PrepMinMinutes         *int
	PrepMaxMinutes         *int
	ClearPrepProfile       bool
	ExpectedVersion        int64
}

// SetBranchOverride writes the branch layer, appending price history whenever
// the price dimension actually moves.
//
// Changing the price requires the pricing grant on top of the operations one;
// changing only the preparation bounds does not. The check is per field rather
// than per route, because one endpoint that edits both must not hand pricing
// authority to whoever holds the operational grant.
func (s *Service) SetBranchOverride(ctx context.Context, actor Actor, scope Scope, itemID string, in SetBranchOverrideInput, pricingAllowed func() error) (BranchItemOverride, error) {
	if err := s.requireWrites(); err != nil {
		return BranchItemOverride{}, err
	}
	var violations []apperrors.FieldViolation
	if in.BasePriceOverrideMinor != nil {
		if violation := validatePriceMinor("base_price_override_minor", *in.BasePriceOverrideMinor); violation != nil {
			violations = append(violations, *violation)
		}
	}
	if in.ClearPrepProfile && (in.PrepMinMinutes != nil || in.PrepMaxMinutes != nil) {
		violations = append(violations, apperrors.FieldViolation{Field: "clear_prep_profile",
			Message: "Clear the preparation override or set it, not both."})
	}
	if in.ClearBasePrice && in.BasePriceOverrideMinor != nil {
		violations = append(violations, apperrors.FieldViolation{Field: "clear_base_price",
			Message: "Clear the price override or set it, not both."})
	}
	if (in.PrepMinMinutes == nil) != (in.PrepMaxMinutes == nil) {
		violations = append(violations, apperrors.FieldViolation{Field: "prep_max_minutes",
			Message: "Provide both preparation bounds together."})
	}
	if in.PrepMinMinutes != nil {
		if violation := validatePrepRange(*in.PrepMinMinutes, *in.PrepMaxMinutes); violation != nil {
			violations = append(violations, *violation)
		}
	}
	if len(violations) > 0 {
		return BranchItemOverride{}, coded(apperrors.Validation("Check your details.", violations),
			CodeInvalidPriceOverride)
	}
	priceTouched := in.BasePriceOverrideMinor != nil || in.ClearBasePrice
	if priceTouched && pricingAllowed != nil {
		if err := pricingAllowed(); err != nil {
			return BranchItemOverride{}, err
		}
	}

	var updated BranchItemOverride
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		current, err := loadOverride(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		price := current.BasePriceOverrideMinor
		switch {
		case in.ClearBasePrice:
			price = nil
		case in.BasePriceOverrideMinor != nil:
			value := *in.BasePriceOverrideMinor
			price = &value
		}
		prepMin, prepMax := current.PrepMinMinutes, current.PrepMaxMinutes
		switch {
		case in.ClearPrepProfile:
			prepMin, prepMax = nil, nil
		case in.PrepMinMinutes != nil:
			minValue, maxValue := *in.PrepMinMinutes, *in.PrepMaxMinutes
			prepMin, prepMax = &minValue, &maxValue
		}

		overrideID := current.ID
		if overrideID == "" {
			overrideID = newID("fbo_")
		}
		updated, err = scanOverride(tx.QueryRow(ctx, `
			INSERT INTO food_branch_item_overrides
				(item_id,id,business_id,branch_id,base_price_override_minor,
				 prep_min_minutes,prep_max_minutes,version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,1)
			ON CONFLICT (item_id) DO UPDATE SET
				base_price_override_minor = EXCLUDED.base_price_override_minor,
				prep_min_minutes = EXCLUDED.prep_min_minutes,
				prep_max_minutes = EXCLUDED.prep_max_minutes,
				updated_at = now(), version = food_branch_item_overrides.version + 1
			RETURNING `+overrideCols,
			item.ID, overrideID, item.BusinessID, item.BranchID, price, prepMin, prepMax))
		if err != nil {
			return mapDB(err, "Branch override", CodeOverrideNotFound)
		}

		catalogue, err := scopeOfItem(ctx, tx, item)
		if err != nil {
			return err
		}
		// History only when the price dimension genuinely moved. A preparation
		// edit is not a price change and must not litter the pricing record.
		var priceVersionRef string
		if priceChanged(current.BasePriceOverrideMinor, updated.BasePriceOverrideMinor) {
			currency, curErr := s.businessCurrency(ctx, tx, item.BusinessID)
			if curErr != nil {
				return curErr
			}
			reason := PriceChangeOverrideSet
			if updated.BasePriceOverrideMinor == nil {
				reason = PriceChangeOverrideCleared
			}
			branch := item.BranchID
			itemRef := item.ID
			priceVersionRef, err = s.recordPrice(ctx, tx, actor, priceRecord{
				scope: catalogue, branchID: &branch, subjectType: PriceSubjectBranchOverride,
				subjectRef: updated.ID, itemRef: &itemRef, currency: currency,
				previous: current.BasePriceOverrideMinor, amount: updated.BasePriceOverrideMinor,
				prevInherit:  current.BasePriceOverrideMinor == nil,
				inherit:      updated.BasePriceOverrideMinor == nil,
				changeReason: reason, version: updated.Version,
			})
			if err != nil {
				return err
			}
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.branch_override_changed", eventType: "food.item.branch_override.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: updated.Version,
			before:  BranchItemOverrideViewOf(current), after: BranchItemOverrideViewOf(updated),
			payload: map[string]any{"item_id": item.ID, "override": BranchItemOverrideViewOf(updated),
				"price_version_ref": stringPtrOrNil(priceVersionRef), "catalogue_revision": revision},
		})
	})
	return updated, err
}

// priceChanged compares two optional prices, treating inherit-vs-set as a
// change even when no number moved.
func priceChanged(before, after *int64) bool {
	switch {
	case before == nil && after == nil:
		return false
	case before == nil || after == nil:
		return true
	default:
		return *before != *after
	}
}

// ClearBranchOverride removes the whole override layer.
//
// The row is deleted because it is pure configuration, but its pricing history
// is not: a final OVERRIDE_CLEARED version is appended first, so the record of
// what this branch used to charge survives the row that charged it.
func (s *Service) ClearBranchOverride(ctx context.Context, actor Actor, scope Scope, itemID string, expectedVersion int64, pricingAllowed func() error) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		current, err := loadOverride(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version == 0 {
			return errNotFound("Branch override", CodeOverrideNotFound)
		}
		if current.Version != expectedVersion {
			return errStaleVersion()
		}
		if current.BasePriceOverrideMinor != nil && pricingAllowed != nil {
			if err = pricingAllowed(); err != nil {
				return err
			}
		}
		catalogue, err := scopeOfItem(ctx, tx, item)
		if err != nil {
			return err
		}
		if current.BasePriceOverrideMinor != nil {
			currency, curErr := s.businessCurrency(ctx, tx, item.BusinessID)
			if curErr != nil {
				return curErr
			}
			branch := item.BranchID
			itemRef := item.ID
			if _, err = s.recordPrice(ctx, tx, actor, priceRecord{
				scope: catalogue, branchID: &branch, subjectType: PriceSubjectBranchOverride,
				subjectRef: current.ID, itemRef: &itemRef, currency: currency,
				previous: current.BasePriceOverrideMinor, amount: nil,
				prevInherit: false, inherit: true,
				changeReason: PriceChangeOverrideCleared, version: current.Version + 1,
			}); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `DELETE FROM food_branch_item_overrides WHERE business_id=$1 AND item_id=$2`,
			item.BusinessID, item.ID); err != nil {
			return err
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.branch_override_cleared", eventType: "food.item.branch_override.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: current.Version, before: BranchItemOverrideViewOf(current),
			payload: map[string]any{"item_id": item.ID, "cleared": true, "catalogue_revision": revision},
		})
	})
}

func validatePrepRange(minMinutes, maxMinutes int) *apperrors.FieldViolation {
	if minMinutes < 0 || minMinutes > MaxPrepMinutes {
		return &apperrors.FieldViolation{Field: "prep_min_minutes",
			Message: "Preparation minutes must be between 0 and " + itoa(MaxPrepMinutes) + "."}
	}
	if maxMinutes < 0 || maxMinutes > MaxPrepMinutes {
		return &apperrors.FieldViolation{Field: "prep_max_minutes",
			Message: "Preparation minutes must be between 0 and " + itoa(MaxPrepMinutes) + "."}
	}
	if maxMinutes < minMinutes {
		return &apperrors.FieldViolation{Field: "prep_max_minutes",
			Message: "The maximum must be at least the minimum."}
	}
	return nil
}
