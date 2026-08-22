package food

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Preparation profiles
// ---------------------------------------------------------------------------
//
// Vendor Service owns how long the kitchen needs. It does not own travel time,
// driver arrival or any customer-facing ETA. This package therefore produces a
// preparation RANGE and never a delivery promise: combining the two is another
// service's decision, and making it here would put a number in front of a
// customer that no one in this codebase is entitled to.

type PrepProfile struct {
	ItemID         string
	BusinessID     string
	PrepMinMinutes int
	PrepMaxMinutes int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
}

type PrepProfileView struct {
	ItemID         string    `json:"item_id"`
	PrepMinMinutes int       `json:"prep_min_minutes"`
	PrepMaxMinutes int       `json:"prep_max_minutes"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int64     `json:"version"`
}

func PrepProfileViewOf(p PrepProfile) PrepProfileView {
	return PrepProfileView{ItemID: p.ItemID, PrepMinMinutes: p.PrepMinMinutes,
		PrepMaxMinutes: p.PrepMaxMinutes, UpdatedAt: p.UpdatedAt, Version: p.Version}
}

// ResolvedPrepProfile is the effective range and where it came from.
type ResolvedPrepProfile struct {
	PrepMinMinutes int    `json:"prep_min_minutes"`
	PrepMaxMinutes int    `json:"prep_max_minutes"`
	Source         string `json:"source"`
	Configured     bool   `json:"configured"`
}

// Prep sources. BRANCH_OVERRIDE wins over ITEM_DEFAULT; NOT_CONFIGURED is
// reported honestly rather than substituting an invented default range.
const (
	PrepSourceItem      = "ITEM_DEFAULT"
	PrepSourceBranch    = "BRANCH_OVERRIDE"
	PrepSourceUnset     = "NOT_CONFIGURED"
	logisticsUnset      = "NOT_CONFIGURED"
	logisticsConfigured = "CONFIGURED"
)

const prepCols = `item_id,business_id,prep_min_minutes,prep_max_minutes,created_at,updated_at,version`

func scanPrep(row rowScanner) (PrepProfile, error) {
	var p PrepProfile
	err := row.Scan(&p.ItemID, &p.BusinessID, &p.PrepMinMinutes, &p.PrepMaxMinutes,
		&p.CreatedAt, &p.UpdatedAt, &p.Version)
	return p, err
}

func loadPrep(ctx context.Context, q querier, item Item, lock bool) (PrepProfile, bool, error) {
	sql := `SELECT ` + prepCols + ` FROM food_item_prep_profiles WHERE business_id=$1 AND item_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	profile, err := scanPrep(q.QueryRow(ctx, sql, item.BusinessID, item.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PrepProfile{ItemID: item.ID, BusinessID: item.BusinessID, Version: 0}, false, nil
	}
	if err != nil {
		return PrepProfile{}, false, mapDB(err, "Preparation profile", CodePrepProfileNotFound)
	}
	return profile, true, nil
}

// GetPrepProfile returns the item's own default, if one is configured.
func (s *Service) GetPrepProfile(ctx context.Context, actor Actor, scope Scope, itemID string) (PrepProfile, bool, error) {
	item, err := s.scopedItem(ctx, actor, scope, itemID)
	if err != nil {
		return PrepProfile{}, false, err
	}
	return loadPrep(ctx, s.db, item, false)
}

// ResolvePrepProfile applies the branch override on top of the item default.
func (s *Service) ResolvePrepProfile(ctx context.Context, q querier, item Item) (ResolvedPrepProfile, error) {
	override, err := loadOverride(ctx, q, item, false)
	if err != nil {
		return ResolvedPrepProfile{}, err
	}
	if override.PrepMinMinutes != nil && override.PrepMaxMinutes != nil {
		return ResolvedPrepProfile{PrepMinMinutes: *override.PrepMinMinutes,
			PrepMaxMinutes: *override.PrepMaxMinutes, Source: PrepSourceBranch, Configured: true}, nil
	}
	profile, found, err := loadPrep(ctx, q, item, false)
	if err != nil {
		return ResolvedPrepProfile{}, err
	}
	if !found {
		return ResolvedPrepProfile{Source: PrepSourceUnset, Configured: false}, nil
	}
	return ResolvedPrepProfile{PrepMinMinutes: profile.PrepMinMinutes,
		PrepMaxMinutes: profile.PrepMaxMinutes, Source: PrepSourceItem, Configured: true}, nil
}

type SetPrepProfileInput struct {
	PrepMinMinutes  int
	PrepMaxMinutes  int
	ExpectedVersion int64
}

// SetPrepProfile writes the item's default preparation range.
func (s *Service) SetPrepProfile(ctx context.Context, actor Actor, scope Scope, itemID string, in SetPrepProfileInput) (PrepProfile, error) {
	if err := s.requireWrites(); err != nil {
		return PrepProfile{}, err
	}
	if violation := validatePrepRange(in.PrepMinMinutes, in.PrepMaxMinutes); violation != nil {
		return PrepProfile{}, coded(apperrors.Validation("Check your details.",
			[]apperrors.FieldViolation{*violation}), CodeInvalidPrepProfile)
	}
	var updated PrepProfile
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		current, _, err := loadPrep(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}
		updated, err = scanPrep(tx.QueryRow(ctx, `
			INSERT INTO food_item_prep_profiles
				(item_id,business_id,prep_min_minutes,prep_max_minutes,version)
			VALUES ($1,$2,$3,$4,1)
			ON CONFLICT (item_id) DO UPDATE SET
				prep_min_minutes = EXCLUDED.prep_min_minutes,
				prep_max_minutes = EXCLUDED.prep_max_minutes,
				updated_at = now(), version = food_item_prep_profiles.version + 1
			RETURNING `+prepCols,
			item.ID, item.BusinessID, in.PrepMinMinutes, in.PrepMaxMinutes))
		if err != nil {
			return mapDB(err, "Preparation profile", CodePrepProfileNotFound)
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.prep_profile_changed", eventType: "food.item.prep_profile.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: updated.Version, before: PrepProfileViewOf(current), after: PrepProfileViewOf(updated),
			payload: map[string]any{"item_id": item.ID, "prep_profile": PrepProfileViewOf(updated),
				"catalogue_revision": revision},
		})
	})
	return updated, err
}

// ClearPrepProfile removes the item's default, returning it to unconfigured.
func (s *Service) ClearPrepProfile(ctx context.Context, actor Actor, scope Scope, itemID string, expectedVersion int64) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		current, found, err := loadPrep(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if !found {
			return errNotFound("Preparation profile", CodePrepProfileNotFound)
		}
		if current.Version != expectedVersion {
			return errStaleVersion()
		}
		if _, err = tx.Exec(ctx, `DELETE FROM food_item_prep_profiles WHERE business_id=$1 AND item_id=$2`,
			item.BusinessID, item.ID); err != nil {
			return err
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.prep_profile_cleared", eventType: "food.item.prep_profile.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: current.Version, before: PrepProfileViewOf(current),
			payload: map[string]any{"item_id": item.ID, "cleared": true, "catalogue_revision": revision},
		})
	})
}

// ---------------------------------------------------------------------------
// Logistics profiles
// ---------------------------------------------------------------------------
//
// Handling metadata for a future logistics decision. Vehicle entries are
// SUITABILITY HINTS: nothing here assigns a provider, chooses a rider, plans a
// route or prices a delivery, and no code path in this package calls Dispatch,
// Taxi or Hauling.

type LogisticsProfile struct {
	ItemID            string
	BusinessID        string
	TemperatureClass  string
	PackageSizeClass  string
	SpillRisk         bool
	Fragile           bool
	ApproxWeightGrams *int
	VehicleHints      []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Version           int64
}

type LogisticsProfileView struct {
	ItemID            string    `json:"item_id"`
	TemperatureClass  string    `json:"temperature_class"`
	PackageSizeClass  string    `json:"package_size_class"`
	SpillRisk         bool      `json:"spill_risk"`
	Fragile           bool      `json:"fragile"`
	ApproxWeightGrams *int      `json:"approx_weight_grams"`
	VehicleHints      []string  `json:"vehicle_hints"`
	UpdatedAt         time.Time `json:"updated_at"`
	Version           int64     `json:"version"`
}

func LogisticsProfileViewOf(l LogisticsProfile) LogisticsProfileView {
	hints := l.VehicleHints
	if hints == nil {
		hints = []string{}
	}
	return LogisticsProfileView{ItemID: l.ItemID, TemperatureClass: l.TemperatureClass,
		PackageSizeClass: l.PackageSizeClass, SpillRisk: l.SpillRisk, Fragile: l.Fragile,
		ApproxWeightGrams: l.ApproxWeightGrams, VehicleHints: hints,
		UpdatedAt: l.UpdatedAt, Version: l.Version}
}

const logisticsCols = `item_id,business_id,temperature_class,package_size_class,spill_risk,fragile,
	approx_weight_grams,vehicle_hints,created_at,updated_at,version`

func scanLogistics(row rowScanner) (LogisticsProfile, error) {
	var l LogisticsProfile
	err := row.Scan(&l.ItemID, &l.BusinessID, &l.TemperatureClass, &l.PackageSizeClass,
		&l.SpillRisk, &l.Fragile, &l.ApproxWeightGrams, &l.VehicleHints,
		&l.CreatedAt, &l.UpdatedAt, &l.Version)
	return l, err
}

func loadLogistics(ctx context.Context, q querier, item Item, lock bool) (LogisticsProfile, bool, error) {
	sql := `SELECT ` + logisticsCols + ` FROM food_item_logistics_profiles WHERE business_id=$1 AND item_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	profile, err := scanLogistics(q.QueryRow(ctx, sql, item.BusinessID, item.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return LogisticsProfile{ItemID: item.ID, BusinessID: item.BusinessID, Version: 0}, false, nil
	}
	if err != nil {
		return LogisticsProfile{}, false, mapDB(err, "Logistics profile", CodeLogisticsNotFound)
	}
	return profile, true, nil
}

// GetLogisticsProfile returns the item's handling metadata if configured.
func (s *Service) GetLogisticsProfile(ctx context.Context, actor Actor, scope Scope, itemID string) (LogisticsProfile, bool, error) {
	item, err := s.scopedItem(ctx, actor, scope, itemID)
	if err != nil {
		return LogisticsProfile{}, false, err
	}
	return loadLogistics(ctx, s.db, item, false)
}

type SetLogisticsProfileInput struct {
	TemperatureClass  string
	PackageSizeClass  string
	SpillRisk         bool
	Fragile           bool
	ApproxWeightGrams *int
	VehicleHints      []string
	ExpectedVersion   int64
}

func inVocabulary(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// SetLogisticsProfile writes the handling metadata.
func (s *Service) SetLogisticsProfile(ctx context.Context, actor Actor, scope Scope, itemID string, in SetLogisticsProfileInput) (LogisticsProfile, error) {
	if err := s.requireWrites(); err != nil {
		return LogisticsProfile{}, err
	}
	var violations []apperrors.FieldViolation
	temperature := strings.ToUpper(strings.TrimSpace(in.TemperatureClass))
	if temperature == "" {
		temperature = "AMBIENT"
	}
	if !inVocabulary(TemperatureClasses, temperature) {
		violations = append(violations, apperrors.FieldViolation{Field: "temperature_class",
			Message: "Temperature class must be AMBIENT, HOT, COLD or FROZEN."})
	}
	size := strings.ToUpper(strings.TrimSpace(in.PackageSizeClass))
	if size == "" {
		size = "SMALL"
	}
	if !inVocabulary(PackageSizeClasses, size) {
		violations = append(violations, apperrors.FieldViolation{Field: "package_size_class",
			Message: "Package size must be SMALL, MEDIUM or LARGE."})
	}
	if in.ApproxWeightGrams != nil {
		if *in.ApproxWeightGrams <= 0 || *in.ApproxWeightGrams > MaxApproxWeightGrams {
			violations = append(violations, apperrors.FieldViolation{Field: "approx_weight_grams",
				Message: "Approximate weight must be between 1 and " + itoa(MaxApproxWeightGrams) + " grams."})
		}
	}
	hints := make([]string, 0, len(in.VehicleHints))
	seen := map[string]bool{}
	for _, raw := range in.VehicleHints {
		hint := strings.ToUpper(strings.TrimSpace(raw))
		if !inVocabulary(VehicleHints, hint) {
			violations = append(violations, apperrors.FieldViolation{Field: "vehicle_hints",
				Message: "Vehicle hints must be MOTORCYCLE, CAR, VAN or TRUCK."})
			break
		}
		if seen[hint] {
			violations = append(violations, apperrors.FieldViolation{Field: "vehicle_hints",
				Message: "Duplicate vehicle hints are not allowed."})
			break
		}
		seen[hint] = true
		hints = append(hints, hint)
	}
	sort.Strings(hints)
	if len(violations) > 0 {
		return LogisticsProfile{}, coded(apperrors.Validation("Check your details.", violations),
			CodeInvalidLogisticsProfile)
	}

	var updated LogisticsProfile
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		current, _, err := loadLogistics(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}
		updated, err = scanLogistics(tx.QueryRow(ctx, `
			INSERT INTO food_item_logistics_profiles
				(item_id,business_id,temperature_class,package_size_class,spill_risk,fragile,
				 approx_weight_grams,vehicle_hints,version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1)
			ON CONFLICT (item_id) DO UPDATE SET
				temperature_class = EXCLUDED.temperature_class,
				package_size_class = EXCLUDED.package_size_class,
				spill_risk = EXCLUDED.spill_risk, fragile = EXCLUDED.fragile,
				approx_weight_grams = EXCLUDED.approx_weight_grams,
				vehicle_hints = EXCLUDED.vehicle_hints,
				updated_at = now(), version = food_item_logistics_profiles.version + 1
			RETURNING `+logisticsCols,
			item.ID, item.BusinessID, temperature, size, in.SpillRisk, in.Fragile,
			in.ApproxWeightGrams, hints))
		if err != nil {
			return mapDB(err, "Logistics profile", CodeLogisticsNotFound)
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.logistics_profile_changed", eventType: "food.item.logistics_profile.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: updated.Version, before: LogisticsProfileViewOf(current),
			after: LogisticsProfileViewOf(updated),
			payload: map[string]any{"item_id": item.ID, "logistics_profile": LogisticsProfileViewOf(updated),
				"catalogue_revision": revision},
		})
	})
	return updated, err
}

// ClearLogisticsProfile removes the handling metadata.
func (s *Service) ClearLogisticsProfile(ctx context.Context, actor Actor, scope Scope, itemID string, expectedVersion int64) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		current, found, err := loadLogistics(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if !found {
			return errNotFound("Logistics profile", CodeLogisticsNotFound)
		}
		if current.Version != expectedVersion {
			return errStaleVersion()
		}
		if _, err = tx.Exec(ctx, `DELETE FROM food_item_logistics_profiles WHERE business_id=$1 AND item_id=$2`,
			item.BusinessID, item.ID); err != nil {
			return err
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.logistics_profile_cleared", eventType: "food.item.logistics_profile.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: current.Version, before: LogisticsProfileViewOf(current),
			payload: map[string]any{"item_id": item.ID, "cleared": true, "catalogue_revision": revision},
		})
	})
}

// ---------------------------------------------------------------------------
// Availability source (the inventory seam)
// ---------------------------------------------------------------------------
//
// The SEAM and nothing else. There is no stock table, no quantity, no
// reservation and no decrement anywhere in this package, because Vendor Service
// is not the inventory authority and no such authority exists yet.

type AvailabilitySource struct {
	ItemID             string
	BusinessID         string
	Mode               string
	InventoryRecipeRef *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int64
}

type AvailabilitySourceView struct {
	ItemID             string    `json:"item_id"`
	Mode               string    `json:"mode"`
	InventoryRecipeRef *string   `json:"inventory_recipe_ref"`
	SourceSupport      string    `json:"inventory_availability_support"`
	UpdatedAt          time.Time `json:"updated_at"`
	Version            int64     `json:"version"`
}

// AvailabilitySourceViewOf reports the platform's real inventory support state
// alongside the configuration, so an operator who links an item can see
// immediately that nothing is there to answer for it yet.
func AvailabilitySourceViewOf(s AvailabilitySource) AvailabilitySourceView {
	support := InventorySupportNotConfigured

	if s.Mode == SourceInventoryLinked {
		support = InventoryAvailabilitySupport
	}

	return AvailabilitySourceView{
		ItemID:             s.ItemID,
		Mode:               s.Mode,
		InventoryRecipeRef: s.InventoryRecipeRef,
		SourceSupport:      support,
		UpdatedAt:          s.UpdatedAt,
		Version:            s.Version,
	}
}

const sourceCols = `item_id,business_id,mode,inventory_recipe_ref,created_at,updated_at,version`

func scanSource(row rowScanner) (AvailabilitySource, error) {
	var s AvailabilitySource
	err := row.Scan(&s.ItemID, &s.BusinessID, &s.Mode, &s.InventoryRecipeRef,
		&s.CreatedAt, &s.UpdatedAt, &s.Version)
	return s, err
}

func loadSource(ctx context.Context, q querier, item Item, lock bool) (AvailabilitySource, error) {
	sql := `SELECT ` + sourceCols + ` FROM food_item_availability_sources WHERE business_id=$1 AND item_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	source, err := scanSource(q.QueryRow(ctx, sql, item.BusinessID, item.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		// Unconfigured means MANUAL: an item nobody linked to inventory is
		// controlled by its own operational state, which is the safe default.
		return AvailabilitySource{ItemID: item.ID, BusinessID: item.BusinessID,
			Mode: SourceManual, Version: 0}, nil
	}
	if err != nil {
		return AvailabilitySource{}, mapDB(err, "Availability source", CodeInvalidSource)
	}
	return source, nil
}

// GetAvailabilitySource reads the seam configuration.
func (s *Service) GetAvailabilitySource(ctx context.Context, actor Actor, scope Scope, itemID string) (AvailabilitySource, error) {
	item, err := s.scopedItem(ctx, actor, scope, itemID)
	if err != nil {
		return AvailabilitySource{}, err
	}
	return loadSource(ctx, s.db, item, false)
}

type SetAvailabilitySourceInput struct {
	Mode               string
	InventoryRecipeRef *string
	ExpectedVersion    int64
}

// SetAvailabilitySource configures how an item's availability is decided.
//
// INVENTORY_LINKED requires an opaque recipe reference and, until a real
// inventory authority exists, makes the item unsellable. That is deliberate:
// linking an item to inventory says "do not sell this unless inventory agrees",
// and a missing inventory service is not agreement. The write is allowed â€”
// operators must be able to configure ahead of the integration â€” but the
// consequence is visible in the resolver immediately.
func (s *Service) SetAvailabilitySource(ctx context.Context, actor Actor, scope Scope, itemID string, in SetAvailabilitySourceInput) (AvailabilitySource, error) {
	if err := s.requireWrites(); err != nil {
		return AvailabilitySource{}, err
	}
	mode := strings.ToUpper(strings.TrimSpace(in.Mode))
	var violations []apperrors.FieldViolation
	var reference *string
	switch mode {
	case SourceManual:
		if in.InventoryRecipeRef != nil && strings.TrimSpace(*in.InventoryRecipeRef) != "" {
			violations = append(violations, apperrors.FieldViolation{Field: "inventory_recipe_ref",
				Message: "A manual item carries no inventory reference."})
		}
	case SourceInventoryLinked:
		trimmed := ""
		if in.InventoryRecipeRef != nil {
			trimmed = strings.TrimSpace(*in.InventoryRecipeRef)
		}
		if trimmed == "" || len(trimmed) > MaxInventoryRefLen {
			violations = append(violations, apperrors.FieldViolation{Field: "inventory_recipe_ref",
				Message: "An inventory-linked item needs a reference of 1 to " + itoa(MaxInventoryRefLen) + " characters."})
		} else {
			reference = &trimmed
		}
	default:
		violations = append(violations, apperrors.FieldViolation{Field: "mode",
			Message: "Mode must be MANUAL or INVENTORY_LINKED."})
	}
	if len(violations) > 0 {
		return AvailabilitySource{}, coded(apperrors.Validation("Check your details.", violations),
			CodeInvalidSource)
	}

	var updated AvailabilitySource
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		current, err := loadSource(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}
		updated, err = scanSource(tx.QueryRow(ctx, `
			INSERT INTO food_item_availability_sources
				(item_id,business_id,mode,inventory_recipe_ref,version)
			VALUES ($1,$2,$3,$4,1)
			ON CONFLICT (item_id) DO UPDATE SET
				mode = EXCLUDED.mode, inventory_recipe_ref = EXCLUDED.inventory_recipe_ref,
				updated_at = now(), version = food_item_availability_sources.version + 1
			RETURNING `+sourceCols,
			item.ID, item.BusinessID, mode, reference))
		if err != nil {
			return mapDB(err, "Availability source", CodeInvalidSource)
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.availability_source_changed", eventType: "food.item.availability_source.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: updated.Version, before: AvailabilitySourceViewOf(current),
			after: AvailabilitySourceViewOf(updated),
			payload: map[string]any{"item_id": item.ID, "mode": updated.Mode,
				"inventory_availability_support": InventoryAvailabilitySupport,
				"catalogue_revision":             revision},
		})
	})
	return updated, err
}

// ClearAvailabilitySource returns an item to the MANUAL default.
func (s *Service) ClearAvailabilitySource(ctx context.Context, actor Actor, scope Scope, itemID string, expectedVersion int64) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		current, err := loadSource(ctx, tx, item, true)
		if err != nil {
			return err
		}
		if current.Version == 0 {
			return errNotFound("Availability source", CodeInvalidSource)
		}
		if current.Version != expectedVersion {
			return errStaleVersion()
		}
		if _, err = tx.Exec(ctx, `DELETE FROM food_item_availability_sources WHERE business_id=$1 AND item_id=$2`,
			item.BusinessID, item.ID); err != nil {
			return err
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.availability_source_cleared", eventType: "food.item.availability_source.changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: current.Version, before: AvailabilitySourceViewOf(current),
			payload: map[string]any{"item_id": item.ID, "mode": SourceManual,
				"cleared": true, "catalogue_revision": revision},
		})
	})
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// scopedItem loads an item the caller's scope actually reaches.
func (s *Service) scopedItem(ctx context.Context, actor Actor, scope Scope, itemID string) (Item, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return Item{}, err
	}
	if !scope.Covers(item.BranchID) {
		return Item{}, errNotFound("Food item", CodeItemNotFound)
	}
	return item, nil
}

// bumpForItem advances the revision of the scope an item belongs to.
func (s *Service) bumpForItem(ctx context.Context, tx pgx.Tx, item Item) (int64, error) {
	catalogue, err := scopeOfItem(ctx, tx, item)
	if err != nil {
		return 0, err
	}
	return bumpRevision(ctx, tx, catalogue)
}
