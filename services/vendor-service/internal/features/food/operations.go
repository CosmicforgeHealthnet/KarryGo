package food

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Section 9B vocabularies
// ---------------------------------------------------------------------------

// Operational availability. This is NOT the Section 9A administrative
// lifecycle: an item can be operationally AVAILABLE and still be unsellable
// because it is administratively DRAFT, PAUSED or ARCHIVED. Administrative
// safety always wins, and the resolver enforces that ordering rather than
// trusting either state on its own.
const (
	OperationalAvailable = "AVAILABLE"
	OperationalSoldOut   = "SOLD_OUT"
	OperationalPaused    = "PAUSED"
)

// Schedule labels are descriptive. Availability is decided by the configured
// days and clock times, never by the label — there is no hard-coded idea of
// when "breakfast" is, because that is the vendor's decision.
const (
	ScheduleBreakfast = "BREAKFAST"
	ScheduleLunch     = "LUNCH"
	ScheduleDinner    = "DINNER"
	ScheduleSeasonal  = "SEASONAL"
	ScheduleCustom    = "CUSTOM"
)

// Availability-source modes. INVENTORY_LINKED fails closed while no inventory
// authority exists, which is the point of the mode rather than a limitation of
// it: an operator who links an item to inventory is saying "do not sell this
// unless inventory agrees", and a missing inventory service is not agreement.
const (
	SourceManual          = "MANUAL"
	SourceInventoryLinked = "INVENTORY_LINKED"
)

// InventoryAvailabilitySupport reports whether any authoritative inventory
// source exists for this flow. It is a constant because the honest answer is
// currently a constant; no code path fabricates availability from it.
const InventoryAvailabilitySupport = "NOT_PRESENT"

// Price-history subjects and change reasons.
const (
	PriceSubjectItem           = "ITEM"
	PriceSubjectVariant        = "VARIANT"
	PriceSubjectModifier       = "MODIFIER"
	PriceSubjectBranchOverride = "BRANCH_OVERRIDE"

	PriceChangeBaseline        = "BASELINE_IMPORT"
	PriceChangeCreated         = "CREATED"
	PriceChanged               = "PRICE_CHANGED"
	PriceChangeOverrideSet     = "OVERRIDE_SET"
	PriceChangeOverrideCleared = "OVERRIDE_CLEARED"
)

// Logistics vocabularies. Vehicle entries are suitability HINTS: nothing in
// this service assigns a provider, chooses a rider or prices a delivery.
var (
	TemperatureClasses = []string{"AMBIENT", "HOT", "COLD", "FROZEN"}
	PackageSizeClasses = []string{"SMALL", "MEDIUM", "LARGE"}
	VehicleHints       = []string{"MOTORCYCLE", "CAR", "VAN", "TRUCK"}
)

// Bounds. Each is also a database CHECK, so a direct writer cannot bypass them.
const (
	MaxPrepMinutes       = 1440
	MaxApproxWeightGrams = 200000
	MaxAvailabilityText  = 280
	MaxInventoryRefLen   = 200
)

// Section 9B error codes, carried in error.details.code beside the platform
// envelope so a client branches on a code rather than on prose.
const (
	CodeScheduleNotFound        = "FOOD_MENU_SCHEDULE_NOT_FOUND"
	CodeInvalidSchedule         = "FOOD_INVALID_MENU_SCHEDULE"
	CodeInvalidAvailability     = "FOOD_INVALID_AVAILABILITY_STATE"
	CodeInvalidPriceOverride    = "FOOD_INVALID_PRICE_OVERRIDE"
	CodeInvalidModifierSelect   = "FOOD_INVALID_MODIFIER_SELECTION"
	CodeInvalidPrepProfile      = "FOOD_INVALID_PREP_PROFILE"
	CodePrepProfileNotFound     = "FOOD_PREP_PROFILE_NOT_FOUND"
	CodeInvalidLogisticsProfile = "FOOD_INVALID_LOGISTICS_PROFILE"
	CodeLogisticsNotFound       = "FOOD_LOGISTICS_PROFILE_NOT_FOUND"
	CodeInvalidSource           = "FOOD_INVALID_AVAILABILITY_SOURCE"
	CodeOverrideNotFound        = "FOOD_BRANCH_OVERRIDE_NOT_FOUND"
	CodePriceOverflow           = "FOOD_PRICE_OVERFLOW"
)

// Sellability reason codes. A closed vocabulary, because a client must be able
// to act on the answer without parsing English.
const (
	ReasonSellable            = "SELLABLE"
	ReasonBusinessInactive    = "BUSINESS_INACTIVE"
	ReasonBranchInactive      = "BRANCH_INACTIVE"
	ReasonCapabilityInactive  = "CAPABILITY_INACTIVE"
	ReasonMenuNotActive       = "MENU_NOT_ACTIVE"
	ReasonOutsideSchedule     = "OUTSIDE_MENU_SCHEDULE"
	ReasonSectionInactive     = "SECTION_INACTIVE"
	ReasonItemNotActive       = "ITEM_NOT_ACTIVE"
	ReasonItemSoldOut         = "ITEM_SOLD_OUT"
	ReasonItemPaused          = "ITEM_PAUSED"
	ReasonInventorySourceGone = "INVENTORY_SOURCE_UNAVAILABLE"
	ReasonInventoryEmpty      = "INVENTORY_NOT_AVAILABLE"
)

func errScheduleNotFound() error {
	return errNotFound("Menu schedule", CodeScheduleNotFound)
}

// ---------------------------------------------------------------------------
// Catalogue scope and revisions
// ---------------------------------------------------------------------------

// catalogueScope is the (business, branch, capability) triple a revision is
// counted against. Every discovery-visible change belongs to exactly one.
type catalogueScope struct {
	BusinessID string
	BranchID   string
	Capability string
}

// scopeOfMenu reads the revision scope a menu belongs to.
func scopeOfMenu(menu Menu) catalogueScope {
	return catalogueScope{BusinessID: menu.BusinessID, BranchID: menu.BranchID, Capability: menu.CapabilityType}
}

// scopeOfItem resolves an item's revision scope. The item already carries its
// business and branch; only the capability has to come from its menu, and it
// is one indexed lookup rather than a join on every caller.
func scopeOfItem(ctx context.Context, q querier, item Item) (catalogueScope, error) {
	var capability string
	err := q.QueryRow(ctx, `SELECT capability_type FROM food_menus WHERE business_id=$1 AND id=$2`,
		item.BusinessID, item.MenuID).Scan(&capability)
	if err != nil {
		return catalogueScope{}, mapDB(err, "Menu", CodeMenuNotFound)
	}
	return catalogueScope{BusinessID: item.BusinessID, BranchID: item.BranchID, Capability: capability}, nil
}

// bumpRevision advances the catalogue revision for one scope atomically.
//
// The increment happens in SQL, inside the caller's transaction, so N
// concurrent mutations produce N distinct monotonically increasing revisions
// rather than a lost update. There is deliberately no read-modify-write in Go:
// that pattern is exactly how two simultaneous sold-out toggles would end up
// claiming the same revision and letting a stale reader look current.
func bumpRevision(ctx context.Context, tx pgx.Tx, scope catalogueScope) (int64, error) {
	if scope.BranchID == "" || scope.Capability == "" {
		// A modifier group is business-level and belongs to no single branch;
		// its callers fan the bump out over the affected scopes instead.
		return 0, nil
	}
	var revision int64
	err := tx.QueryRow(ctx, `
		INSERT INTO food_catalogue_revisions (business_id, branch_id, capability_type, revision, updated_at)
		VALUES ($1,$2,$3,1,now())
		ON CONFLICT (business_id, branch_id, capability_type)
		DO UPDATE SET revision = food_catalogue_revisions.revision + 1, updated_at = now()
		RETURNING revision`, scope.BusinessID, scope.BranchID, scope.Capability).Scan(&revision)
	if err != nil {
		return 0, err
	}
	return revision, nil
}

// bumpRevisionsForModifierGroup advances every scope that could see a change to
// a shared modifier group.
//
// A group is business-level, so a price change on one of its modifiers is
// visible everywhere it is attached. Fanning the bump out over exactly those
// scopes is the honest answer; bumping the whole business would tell branches
// that sell none of those items that their catalogue changed.
func bumpRevisionsForModifierGroup(ctx context.Context, tx pgx.Tx, businessID, groupID string) ([]int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT m.branch_id, menu.capability_type
		FROM food_item_modifier_groups link
		JOIN food_items m ON m.id = link.item_id AND m.business_id = link.business_id
		JOIN food_menus menu ON menu.id = m.menu_id AND menu.business_id = m.business_id
		WHERE link.business_id = $1 AND link.modifier_group_id = $2
		ORDER BY 1, 2`, businessID, groupID)
	if err != nil {
		return nil, err
	}
	type target struct{ branch, capability string }
	var targets []target
	for rows.Next() {
		var entry target
		if err = rows.Scan(&entry.branch, &entry.capability); err != nil {
			rows.Close()
			return nil, err
		}
		targets = append(targets, entry)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	revisions := make([]int64, 0, len(targets))
	for _, entry := range targets {
		revision, bumpErr := bumpRevision(ctx, tx, catalogueScope{
			BusinessID: businessID, BranchID: entry.branch, Capability: entry.capability})
		if bumpErr != nil {
			return nil, bumpErr
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

// CatalogueRevision reads the current revision for a scope, or 0 when the scope
// has never existed. Zero is honest: it means "no catalogue here yet", not
// "revision unknown".
func (s *Service) CatalogueRevision(ctx context.Context, q querier, scope catalogueScope) (int64, error) {
	var revision int64
	err := q.QueryRow(ctx, `SELECT revision FROM food_catalogue_revisions
		WHERE business_id=$1 AND branch_id=$2 AND capability_type=$3`,
		scope.BusinessID, scope.BranchID, scope.Capability).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}

// ---------------------------------------------------------------------------
// Immutable price history
// ---------------------------------------------------------------------------

// PriceVersion is one append-only record of what something cost.
type PriceVersion struct {
	ID                  string
	BusinessID          string
	BranchID            *string
	SubjectType         string
	SubjectRef          string
	ItemRef             *string
	Currency            string
	PreviousAmountMinor *int64
	AmountMinor         *int64
	PreviousInherited   bool
	Inherited           bool
	ChangeReason        string
	SubjectVersion      int64
	Revision            int64
	ActorType           string
	ActorID             *string
	CorrelationID       *string
	CommandRef          *string
	RecordedAt          time.Time
	EffectiveAt         time.Time
}

type PriceVersionView struct {
	ID                  string    `json:"id"`
	SubjectType         string    `json:"subject_type"`
	SubjectRef          string    `json:"subject_ref"`
	ItemRef             *string   `json:"item_ref,omitempty"`
	BranchID            *string   `json:"branch_id,omitempty"`
	Currency            string    `json:"currency"`
	PreviousAmountMinor *int64    `json:"previous_amount_minor"`
	AmountMinor         *int64    `json:"amount_minor"`
	PreviousInherited   bool      `json:"previous_inherited"`
	Inherited           bool      `json:"inherited"`
	ChangeReason        string    `json:"change_reason"`
	SubjectVersion      int64     `json:"subject_version"`
	Revision            int64     `json:"revision"`
	ActorType           string    `json:"actor_type"`
	ActorID             *string   `json:"actor_id,omitempty"`
	RecordedAt          time.Time `json:"recorded_at"`
	EffectiveAt         time.Time `json:"effective_at"`
}

func PriceVersionViewOf(v PriceVersion) PriceVersionView {
	return PriceVersionView{ID: v.ID, SubjectType: v.SubjectType, SubjectRef: v.SubjectRef,
		ItemRef: v.ItemRef, BranchID: v.BranchID, Currency: v.Currency,
		PreviousAmountMinor: v.PreviousAmountMinor, AmountMinor: v.AmountMinor,
		PreviousInherited: v.PreviousInherited, Inherited: v.Inherited,
		ChangeReason: v.ChangeReason, SubjectVersion: v.SubjectVersion, Revision: v.Revision,
		ActorType: v.ActorType, ActorID: v.ActorID,
		RecordedAt: v.RecordedAt, EffectiveAt: v.EffectiveAt}
}

// priceRecord is what a mutation knows about the price it just changed.
type priceRecord struct {
	scope        catalogueScope
	branchID     *string
	subjectType  string
	subjectRef   string
	itemRef      *string
	currency     string
	previous     *int64
	amount       *int64
	prevInherit  bool
	inherit      bool
	changeReason string
	version      int64
}

// recordPrice appends one immutable price version inside the caller's
// transaction and returns its id, which is the reference a future Order
// snapshot will carry.
//
// The per-subject revision is computed in the same statement under the row lock
// every price mutation already holds on its subject, so two concurrent changes
// cannot claim the same revision number.
func (s *Service) recordPrice(ctx context.Context, tx pgx.Tx, actor Actor, in priceRecord) (string, error) {
	id := newID("fpv_")
	var actorID, correlation, command *string
	if actor.StaffID != "" {
		value := actor.StaffID
		actorID = &value
	}
	if actor.CorrelationID != "" {
		value := actor.CorrelationID
		correlation = &value
	}
	if actor.CommandRef != "" {
		value := actor.CommandRef
		command = &value
	}
	actorType := "vendor_staff"
	if actor.StaffID == "" {
		actorType = "system"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO food_price_versions
			(id, business_id, branch_id, subject_type, subject_ref, item_ref, currency,
			 previous_amount_minor, amount_minor, previous_inherited, inherited,
			 change_reason, subject_version, revision, actor_type, actor_id,
			 correlation_id, command_ref, recorded_at, effective_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
			COALESCE((SELECT max(revision) FROM food_price_versions
				WHERE subject_type = $4 AND subject_ref = $5), 0) + 1,
			$14,$15,$16,$17, now(), now())`,
		id, in.scope.BusinessID, in.branchID, in.subjectType, in.subjectRef, in.itemRef, in.currency,
		in.previous, in.amount, in.prevInherit, in.inherit, in.changeReason, in.version,
		actorType, actorID, correlation, command)
	if err != nil {
		return "", err
	}
	return id, nil
}

const priceVersionCols = `id,business_id,branch_id,subject_type,subject_ref,item_ref,currency,
	previous_amount_minor,amount_minor,previous_inherited,inherited,change_reason,
	subject_version,revision,actor_type,actor_id,correlation_id,command_ref,recorded_at,effective_at`

func scanPriceVersion(row rowScanner) (PriceVersion, error) {
	var v PriceVersion
	err := row.Scan(&v.ID, &v.BusinessID, &v.BranchID, &v.SubjectType, &v.SubjectRef, &v.ItemRef,
		&v.Currency, &v.PreviousAmountMinor, &v.AmountMinor, &v.PreviousInherited, &v.Inherited,
		&v.ChangeReason, &v.SubjectVersion, &v.Revision, &v.ActorType, &v.ActorID,
		&v.CorrelationID, &v.CommandRef, &v.RecordedAt, &v.EffectiveAt)
	return v, err
}

// PriceHistoryOptions bounds the aggregate history read.
type PriceHistoryOptions struct {
	SubjectType string
	Limit       int
	Offset      int
}

// ListItemPriceHistory returns the pricing history relevant to one item: its
// own base price, its variants, its branch override, and the modifiers of the
// groups currently attached to it.
//
// It is one query rather than four round trips, and every branch of the union
// is constrained by the caller's business — history is not a cross-tenant query
// surface, and an item id from another business simply matches nothing.
func (s *Service) ListItemPriceHistory(ctx context.Context, actor Actor, scope Scope, itemID string, opts PriceHistoryOptions) ([]PriceVersion, bool, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return nil, false, err
	}
	if !scope.Covers(item.BranchID) {
		return nil, false, errNotFound("Food item", CodeItemNotFound)
	}
	subject := strings.ToUpper(strings.TrimSpace(opts.SubjectType))
	switch subject {
	case "", PriceSubjectItem, PriceSubjectVariant, PriceSubjectModifier, PriceSubjectBranchOverride:
	default:
		return nil, false, validation(apperrors.FieldViolation{
			Field: "subject_type", Message: "Unsupported price history subject."})
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(ctx, `
		SELECT `+priceVersionCols+` FROM food_price_versions
		WHERE business_id = $1
		  AND ($2 = '' OR subject_type = $2)
		  AND (
			(subject_type = 'ITEM' AND subject_ref = $3)
			OR (subject_type = 'VARIANT' AND subject_ref IN (
				SELECT id FROM food_item_variants WHERE business_id = $1 AND item_id = $3))
			OR (subject_type = 'BRANCH_OVERRIDE' AND subject_ref IN (
				SELECT id FROM food_branch_item_overrides WHERE business_id = $1 AND item_id = $3))
			OR (subject_type = 'MODIFIER' AND subject_ref IN (
				SELECT md.id FROM food_modifiers md
				JOIN food_item_modifier_groups link
				  ON link.modifier_group_id = md.modifier_group_id AND link.business_id = md.business_id
				WHERE md.business_id = $1 AND link.item_id = $3))
		  )
		ORDER BY recorded_at DESC, id DESC
		LIMIT $4 OFFSET $5`, actor.VendorID, subject, item.ID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	history := []PriceVersion{}
	for rows.Next() {
		version, scanErr := scanPriceVersion(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		history = append(history, version)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(history) > limit
	if hasMore {
		history = history[:limit]
	}
	return history, hasMore, nil
}

// ---------------------------------------------------------------------------
// Overflow-safe money arithmetic
// ---------------------------------------------------------------------------

// addMinor adds two minor-unit amounts, refusing rather than wrapping.
//
// The bounds make an overflow arithmetically impossible with the current
// limits, which is exactly why the check is here: a later widening of those
// limits must fail loudly instead of silently producing a negative price.
func addMinor(left, right int64) (int64, error) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, coded(apperrors.Validation("The resolved price is outside the supported range.",
			[]apperrors.FieldViolation{{Field: "price", Message: "The resolved price is too large."}}),
			CodePriceOverflow)
	}
	if right < 0 && left < math.MinInt64-right {
		return 0, coded(apperrors.Validation("The resolved price is outside the supported range.",
			[]apperrors.FieldViolation{{Field: "price", Message: "The resolved price is too small."}}),
			CodePriceOverflow)
	}
	total := left + right
	if total < 0 || total > MaxPriceMinor*64 {
		return 0, coded(apperrors.Validation("The resolved price is outside the supported range.",
			[]apperrors.FieldViolation{{Field: "price", Message: "The resolved price is outside the supported range."}}),
			CodePriceOverflow)
	}
	return total, nil
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
