package food

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Actor is the authenticated vendor staff member behind a request. VendorID
// comes from the server-resolved session and never from the path or body,
// which is what makes tenant spoofing impossible rather than merely unlikely.
type Actor struct {
	StaffID       string
	VendorID      string
	SessionID     string
	CorrelationID string
	CausationID   string
	// CommandRef is the request's Idempotency-Key. Section 9B records it on
	// price history so a duplicate-looking pair of versions can be told apart
	// from a genuine repeat of the same command.
	CommandRef string
}

// Scope is the actor's authorised reach for one permission: a vendor-wide
// grant, or the exact branches their assignment covers.
type Scope struct {
	Global    bool
	BranchIDs []string
}

// Empty reports a scope that authorises nothing.
func (s Scope) Empty() bool { return !s.Global && len(s.BranchIDs) == 0 }

// Covers reports whether the scope reaches a branch.
func (s Scope) Covers(branchID string) bool {
	if s.Global {
		return true
	}
	for _, id := range s.BranchIDs {
		if id == branchID {
			return true
		}
	}
	return false
}

// ScopeResolver answers "at which scopes does this staff member hold this
// permission". A port, so this feature never imports the staff repositories.
type ScopeResolver interface {
	ResolveScope(ctx context.Context, staffID, permission string) (Scope, error)
}

// AuditEntry mirrors the service-wide audit record shape.
type AuditEntry struct {
	ActorID, Action, AggregateType, AggregateID, VendorID, BranchID, Reason string
	Before, After                                                           any
}

// AuditRecorder writes an audit entry. Section 9A does not use it for its own
// mutations ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â those are written inside the mutation's transaction (see
// auditTx) so state and evidence commit together ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â it exists for the same
// port shape the rest of the service uses.
type AuditRecorder interface {
	Record(context.Context, AuditEntry) error
}

// EventPublisher appends a domain event onto the transactional outbox inside
// the caller's transaction.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string,
		aggregateVersion int64, vendorID, correlationID, causationID string, payload any) error
}

// MediaObject is the Media File Service's answer about one reference. Vendor
// Service stores none of it beyond the opaque id.
type MediaObject struct {
	ID             string
	OwnerReference string
	UploadPurpose  string
	UploadStatus   string
	SafetyStatus   string
	DeletionState  string
	SourceVersion  int64
}

// MediaAuthority is the existing Media private-API ownership contract Section
// 7 already uses for onboarding documents and brand media. Section 9A reuses
// it rather than inventing a second way to trust a media reference.
type MediaAuthority interface {
	GetObject(ctx context.Context, businessID, mediaRef string) (MediaObject, error)
}

// Flags is the Section 9A slice of the service feature-flag surface.
type Flags struct {
	// CatalogueEnabled gates the whole feature (COMMERCE_FOOD_ENABLED). With it
	// false no food route is registered at all, so a disabled catalogue has no
	// attack surface rather than routes that answer 503 and thereby confirm the
	// feature exists.
	CatalogueEnabled bool
	// WritesEnabled gates every mutation. Reads stay available so a rollout can
	// disable risky writes without blinding operators.
	WritesEnabled bool
	// CommerceKillSwitch is the platform-wide commerce stop. It overrides
	// WritesEnabled and can only ever withhold a mutation, never permit one.
	CommerceKillSwitch bool
}

func (f Flags) writesAllowed() bool { return f.WritesEnabled && !f.CommerceKillSwitch }

// Service is the Section 9A application layer. Every ownership, capability and
// lifecycle decision lives here or in the database, never in a handler.
type Service struct {
	db        *pgxpool.Pool
	scopes    ScopeResolver
	audit     AuditRecorder
	events    EventPublisher
	media     MediaAuthority
	inventory InventoryAvailabilityAuthority
	flags     Flags
	now       func() time.Time
}

// Options configures a Service.
type Options struct {
	DB     *pgxpool.Pool
	Scopes ScopeResolver
	Audit  AuditRecorder
	Events EventPublisher
	Flags  Flags
	// Media proves a media reference is an active, completed object owned by
	// this business. Nil refuses media attachment outright rather than storing
	// an unverified reference ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â an unchecked reference is worse than none.
	Media     MediaAuthority
	Inventory InventoryAvailabilityAuthority
	// Now is injectable so archival timestamps are deterministic in tests.
	Now func() time.Time
}

func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: opts.DB, scopes: opts.Scopes, audit: opts.Audit, events: opts.Events,
		media: opts.Media, inventory: opts.Inventory, flags: opts.Flags, now: now}
}

// FlagSet exposes the configured flags for route registration and tests.
func (s *Service) FlagSet() Flags { return s.flags }

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

const (
	PermCatalogueRead    = "vendor.food.catalogue.read"
	PermCatalogueManage  = "vendor.food.catalogue.manage"
	PermCataloguePublish = "vendor.food.catalogue.publish"

	// Section 9B splits two authorities Section 9A had merged into
	// catalogue.manage. Pricing is separated because the kitchen needs to mark
	// a dish sold out during service and should not need business-wide price
	// authority to do it; operations is separated so that grant can exist
	// without carrying pricing with it.
	PermPricingManage    = "vendor.food.pricing.manage"
	PermOperationsManage = "vendor.food.operations.manage"
)

// Authorize resolves the actor's scope for a permission, denying outright when
// they hold no covering grant.
func (s *Service) Authorize(ctx context.Context, actor Actor, permission string) (Scope, error) {
	if actor.StaffID == "" || actor.VendorID == "" {
		return Scope{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	if s.scopes == nil {
		return Scope{}, errForbidden()
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, permission)
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() {
		return Scope{}, errForbidden()
	}
	return scope, nil
}

// requireWrites is checked on every mutation, after authorization, so a
// disabled rollout never looks like a permission problem.
func (s *Service) requireWrites() error {
	if !s.flags.CatalogueEnabled {
		return apperrors.Unavailable("Food catalogue management is not enabled.", nil)
	}
	if !s.flags.writesAllowed() {
		return apperrors.Unavailable("Food catalogue changes are temporarily unavailable.", nil)
	}
	return nil
}

// requirePricingAuthority is the Section 9B gate on money.
//
// Section 9A let one grant edit both a menu and its prices. That merged two
// different authorities, and the merge became untenable once the kitchen needed
// catalogue.manage to mark a dish sold out: nobody should have to be handed
// price authority to do that. So any mutation that establishes a price
// (including an explicit zero) or changes an existing one additionally
// requires vendor.food.pricing.manage,
// checked per field rather than per route ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â one endpoint that edits a name and
// a price must not hand pricing power to whoever may edit the name.
func (s *Service) requirePricingAuthority(ctx context.Context, actor Actor, branchID string) error {
	scope, err := s.Authorize(ctx, actor, PermPricingManage)
	if err != nil {
		return err
	}
	// A branch-scoped pricing grant reaches only its own branches. Modifier
	// prices are business-level and pass an empty branch, which only a
	// business-wide grant covers.
	if branchID == "" {
		if !scope.Global {
			return apperrors.Forbidden("Modifier prices are shared across the business and need business-wide pricing access.", nil)
		}
		return nil
	}
	if !scope.Covers(branchID) {
		return errForbidden()
	}
	return nil
}

// requireBusinessWide gates the business-level modifier-group definitions.
//
// A branch-scoped grant is deliberately not enough: a modifier group is shared
// by every item that attaches it, so editing one from a single branch would
// silently change other branches' menus. Branch-scoped staff can still attach,
// detach and reorder groups on their own branch's items.
func requireBusinessWide(scope Scope) error {
	if !scope.Global {
		return apperrors.Forbidden("Modifier groups are shared across the business and need business-wide access.", nil)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------
//
// Stable Section 9A codes travel in the error details alongside the platform
// envelope, so a client can branch on a code without parsing prose. "Not
// found" is deliberately indistinguishable from "exists but is not yours":
// anything else would let a caller enumerate another tenant's catalogue.

const (
	CodeCapabilityNotSupported = "FOOD_CAPABILITY_NOT_SUPPORTED"
	CodeMenuNotFound           = "FOOD_MENU_NOT_FOUND"
	CodeMenuNotEditable        = "FOOD_MENU_NOT_EDITABLE"
	CodeSectionNotFound        = "FOOD_SECTION_NOT_FOUND"
	CodeItemNotFound           = "FOOD_ITEM_NOT_FOUND"
	CodeVariantNotFound        = "FOOD_VARIANT_NOT_FOUND"
	CodeModifierGroupNotFound  = "FOOD_MODIFIER_GROUP_NOT_FOUND"
	CodeModifierNotFound       = "FOOD_MODIFIER_NOT_FOUND"
	CodeStaleVersion           = "FOOD_STALE_VERSION"
	CodeInvalidReorder         = "FOOD_INVALID_REORDER"
	CodeDuplicateRelationship  = "FOOD_DUPLICATE_RELATIONSHIP"
	CodeArchived               = "FOOD_ENTITY_ARCHIVED"
	CodeNotPublishable         = "FOOD_MENU_NOT_PUBLISHABLE"
	CodeDuplicateName          = "FOOD_DUPLICATE_NAME"
	CodeMediaUnavailable       = "FOOD_MEDIA_UNAVAILABLE"
)

func coded(err *apperrors.Error, code string) error {
	if err.Details == nil {
		err.Details = map[string]any{}
	}
	err.Details["code"] = code
	return err
}

func errForbidden() error {
	return apperrors.Forbidden("You do not have permission to do this.", nil)
}
func errNotFound(label, code string) error {
	return coded(apperrors.NotFound(label+" not found.", nil), code)
}
func errStaleVersion() error {
	return coded(apperrors.Conflict("This record changed since you loaded it. Reload and try again.", nil), CodeStaleVersion)
}
func errArchived(label string) error {
	return coded(apperrors.Conflict(label+" has been archived and can no longer be changed.", nil), CodeArchived)
}
func errInvalidReorder(message string) error {
	return coded(apperrors.Validation(message, []apperrors.FieldViolation{
		{Field: "ordered_ids", Message: message}}), CodeInvalidReorder)
}

// mapDB turns a database failure into the service's own vocabulary. A raw SQL
// error never reaches a response body.
func mapDB(err error, label, code string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound(label, code)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return coded(apperrors.Conflict(label+" with these details already exists.", nil), CodeDuplicateName)
		case "23503":
			return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "reference", Message: "Referenced resource does not belong to this business."}}), CodeInvalidReorder)
		case "23514":
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "request", Message: "A supplied value is outside the supported range."}})
		}
	}
	return err
}

// ---------------------------------------------------------------------------
// Transaction and evidence helpers
// ---------------------------------------------------------------------------

// inTx runs fn in a transaction, rolling back on any error so an outbox row
// can never survive a mutation that did not commit.
func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// auditTx writes the audit row inside the mutation's own transaction, which is
// what makes the evidence as durable as the change. New Section 9A code does
// not use the append-after-commit shape.
func (s *Service) auditTx(ctx context.Context, tx pgx.Tx, actor Actor, action, aggregateType, aggregateID, branchID string, before, after any, reason string) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	actorType := "vendor_staff"
	if actor.StaffID == "" {
		actorType = "system"
	}
	_, err := tx.Exec(ctx, `INSERT INTO vendor_audit_log
		(id,vendor_id,actor_type,actor_id,action,aggregate_type,aggregate_id,branch_id,correlation_id,before_data,after_data,reason)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,NULLIF($12,''))`,
		"val_"+uuid.NewString(), actor.VendorID, actorType, actor.StaffID, action, aggregateType, aggregateID,
		branchID, actor.CorrelationID, nilJSON(before, beforeJSON), nilJSON(after, afterJSON), reason)
	return err
}

func (s *Service) publishTx(ctx context.Context, tx pgx.Tx, actor Actor, eventType, aggregateType, aggregateID string, aggregateVersion int64, payload any) error {
	if s.events == nil {
		return nil
	}
	return s.events.Publish(ctx, tx, eventType, aggregateType, aggregateID, aggregateVersion,
		actor.VendorID, actor.CorrelationID, actor.CausationID, payload)
}

// record writes audit and outbox together for one mutation. Every command in
// this package goes through it, so "audited" and "published" are not decisions
// a new handler can forget to make.
func (s *Service) record(ctx context.Context, tx pgx.Tx, actor Actor, ev evidence) error {
	if err := s.auditTx(ctx, tx, actor, ev.action, ev.aggregateType, ev.aggregateID, ev.branchID,
		ev.before, ev.after, ev.reason); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, ev.eventType, ev.aggregateType, ev.aggregateID, ev.version, ev.payload)
}

type evidence struct {
	action        string
	eventType     string
	aggregateType string
	aggregateID   string
	branchID      string
	version       int64
	before, after any
	payload       any
	reason        string
}

func nilJSON(v any, raw []byte) any {
	if v == nil {
		return nil
	}
	return raw
}

func newID(prefix string) string { return prefix + uuid.NewString() }

// ---------------------------------------------------------------------------
// Ownership loaders
// ---------------------------------------------------------------------------
//
// Every loader is tenant-scoped in the WHERE clause rather than by a check on
// the returned row, and every nested loader also constrains the parent. That
// is what closes the nested-IDOR cases: a section addressed through the wrong
// menu simply does not exist as far as the query is concerned.

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type rowScanner interface{ Scan(dest ...any) error }

const menuCols = `id,business_id,branch_id,capability_type,name,description,status,created_at,updated_at,archived_at,version`

func scanMenu(row rowScanner) (Menu, error) {
	var m Menu
	err := row.Scan(&m.ID, &m.BusinessID, &m.BranchID, &m.CapabilityType, &m.Name, &m.Description,
		&m.Status, &m.CreatedAt, &m.UpdatedAt, &m.ArchivedAt, &m.Version)
	return m, err
}

// loadMenu reads a menu within the tenant. lock takes a row lock for mutations
// that must serialise against a concurrent edit of the same menu.
func loadMenu(ctx context.Context, q querier, businessID, menuID string, lock bool) (Menu, error) {
	sql := `SELECT ` + menuCols + ` FROM food_menus WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	menu, err := scanMenu(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(menuID)))
	if err != nil {
		return Menu{}, mapDB(err, "Menu", CodeMenuNotFound)
	}
	return menu, nil
}

const sectionCols = `id,business_id,menu_id,name,description,position,status,created_at,updated_at,archived_at,version`

func scanSection(row rowScanner) (Section, error) {
	var s Section
	err := row.Scan(&s.ID, &s.BusinessID, &s.MenuID, &s.Name, &s.Description, &s.Position,
		&s.Status, &s.CreatedAt, &s.UpdatedAt, &s.ArchivedAt, &s.Version)
	return s, err
}

func loadSection(ctx context.Context, q querier, businessID, menuID, sectionID string, lock bool) (Section, error) {
	sql := `SELECT ` + sectionCols + ` FROM food_menu_sections WHERE business_id=$1 AND menu_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	section, err := scanSection(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(menuID), strings.TrimSpace(sectionID)))
	if err != nil {
		return Section{}, mapDB(err, "Menu section", CodeSectionNotFound)
	}
	return section, nil
}

const itemCols = `id,business_id,branch_id,menu_id,section_id,name,description,base_price_minor,
	dietary_codes,allergen_codes,status,position,created_at,updated_at,archived_at,version`

func scanItem(row rowScanner) (Item, error) {
	var i Item
	var dietary, allergens []byte
	err := row.Scan(&i.ID, &i.BusinessID, &i.BranchID, &i.MenuID, &i.SectionID, &i.Name, &i.Description,
		&i.BasePriceMinor, &dietary, &allergens, &i.Status, &i.Position, &i.CreatedAt, &i.UpdatedAt,
		&i.ArchivedAt, &i.Version)
	if err != nil {
		return Item{}, err
	}
	if err = json.Unmarshal(dietary, &i.DietaryCodes); err != nil {
		return Item{}, fmt.Errorf("food: decode dietary codes: %w", err)
	}
	if err = json.Unmarshal(allergens, &i.AllergenCodes); err != nil {
		return Item{}, fmt.Errorf("food: decode allergen codes: %w", err)
	}
	return i, nil
}

// loadItem reads an item by id within the tenant. The item carries its own
// menu, section and branch, so no caller has to trust a path parameter for
// those.
func loadItem(ctx context.Context, q querier, businessID, itemID string, lock bool) (Item, error) {
	sql := `SELECT ` + itemCols + ` FROM food_items WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	item, err := scanItem(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(itemID)))
	if err != nil {
		return Item{}, mapDB(err, "Food item", CodeItemNotFound)
	}
	return item, nil
}

// loadItemInSection additionally proves the item sits in the addressed section
// of the addressed menu.
func loadItemInSection(ctx context.Context, q querier, businessID, menuID, sectionID, itemID string, lock bool) (Item, error) {
	sql := `SELECT ` + itemCols + ` FROM food_items
		WHERE business_id=$1 AND menu_id=$2 AND section_id=$3 AND id=$4`
	if lock {
		sql += ` FOR UPDATE`
	}
	item, err := scanItem(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(menuID),
		strings.TrimSpace(sectionID), strings.TrimSpace(itemID)))
	if err != nil {
		return Item{}, mapDB(err, "Food item", CodeItemNotFound)
	}
	return item, nil
}

const variantCols = `id,business_id,item_id,name,price_override_minor,position,status,created_at,updated_at,archived_at,version`

func scanVariant(row rowScanner) (Variant, error) {
	var v Variant
	err := row.Scan(&v.ID, &v.BusinessID, &v.ItemID, &v.Name, &v.PriceOverrideMinor, &v.Position,
		&v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.Version)
	return v, err
}

func loadVariant(ctx context.Context, q querier, businessID, itemID, variantID string, lock bool) (Variant, error) {
	sql := `SELECT ` + variantCols + ` FROM food_item_variants WHERE business_id=$1 AND item_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	variant, err := scanVariant(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(itemID), strings.TrimSpace(variantID)))
	if err != nil {
		return Variant{}, mapDB(err, "Variant", CodeVariantNotFound)
	}
	return variant, nil
}

const groupCols = `id,business_id,name,description,min_select,max_select,status,created_at,updated_at,archived_at,version`

func scanGroup(row rowScanner) (ModifierGroup, error) {
	var g ModifierGroup
	err := row.Scan(&g.ID, &g.BusinessID, &g.Name, &g.Description, &g.MinSelect, &g.MaxSelect,
		&g.Status, &g.CreatedAt, &g.UpdatedAt, &g.ArchivedAt, &g.Version)
	return g, err
}

func loadGroup(ctx context.Context, q querier, businessID, groupID string, lock bool) (ModifierGroup, error) {
	sql := `SELECT ` + groupCols + ` FROM food_modifier_groups WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	group, err := scanGroup(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(groupID)))
	if err != nil {
		return ModifierGroup{}, mapDB(err, "Modifier group", CodeModifierGroupNotFound)
	}
	return group, nil
}

const modifierCols = `id,business_id,modifier_group_id,name,price_delta_minor,position,status,created_at,updated_at,archived_at,version`

func scanModifier(row rowScanner) (Modifier, error) {
	var m Modifier
	err := row.Scan(&m.ID, &m.BusinessID, &m.ModifierGroupID, &m.Name, &m.PriceDeltaMinor, &m.Position,
		&m.Status, &m.CreatedAt, &m.UpdatedAt, &m.ArchivedAt, &m.Version)
	return m, err
}

func loadModifier(ctx context.Context, q querier, businessID, groupID, modifierID string, lock bool) (Modifier, error) {
	sql := `SELECT ` + modifierCols + ` FROM food_modifiers WHERE business_id=$1 AND modifier_group_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	modifier, err := scanModifier(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(groupID), strings.TrimSpace(modifierID)))
	if err != nil {
		return Modifier{}, mapDB(err, "Modifier", CodeModifierNotFound)
	}
	return modifier, nil
}

// businessCurrency reads the one authoritative currency. Catalogue rows do not
// store it, so there is nothing that can disagree with Section 6.
func (s *Service) businessCurrency(ctx context.Context, q querier, businessID string) (string, error) {
	var currency string
	err := q.QueryRow(ctx, `SELECT currency FROM vendor_businesses WHERE id=$1`, businessID).Scan(&currency)
	if err != nil {
		return "", mapDB(err, "Business", CodeMenuNotFound)
	}
	return currency, nil
}

// ---------------------------------------------------------------------------
// Ordering
// ---------------------------------------------------------------------------

// orderedTable names one of the five ordered child collections. The set is
// closed and every SQL fragment below is a compile-time constant: no table or
// column name is ever built from request data.
type orderedTable struct {
	table     string
	parentCol string
	hasStatus bool
}

var (
	sectionsOrdering  = orderedTable{table: "food_menu_sections", parentCol: "menu_id", hasStatus: true}
	itemsOrdering     = orderedTable{table: "food_items", parentCol: "section_id", hasStatus: true}
	variantsOrdering  = orderedTable{table: "food_item_variants", parentCol: "item_id", hasStatus: true}
	modifiersOrdering = orderedTable{table: "food_modifiers", parentCol: "modifier_group_id", hasStatus: true}
	attachedOrdering  = orderedTable{table: "food_item_modifier_groups", parentCol: "item_id", hasStatus: false}
	mediaOrdering     = orderedTable{table: "food_item_media", parentCol: "item_id", hasStatus: false}
)

// resequence compacts a parent's children onto contiguous positions, live rows
// first and archived rows behind them.
//
// It is the single reason ordering is deterministic: create, archive and
// reorder all end here, so no path can leave a gap, a duplicate or an archived
// row wedged between two live ones. The deferred unique (parent, position)
// constraint means the intermediate states inside this statement are allowed
// while the committed state is still provably unique.
func resequence(ctx context.Context, tx pgx.Tx, spec orderedTable, parentID string) error {
	order := "position, id"
	if spec.hasStatus {
		order = "(status = 'ARCHIVED'), position, id"
	}
	_, err := tx.Exec(ctx, `
		WITH ordered AS (
			SELECT id, row_number() OVER (ORDER BY `+order+`) - 1 AS rn
			FROM `+spec.table+` WHERE `+spec.parentCol+` = $1
		)
		UPDATE `+spec.table+` t SET position = ordered.rn
		FROM ordered WHERE t.id = ordered.id AND t.position <> ordered.rn`, parentID)
	return err
}

// nextPosition is the count of existing rows, which is free because positions
// are always 0..n-1 after resequence. Callers hold the parent row lock, so two
// concurrent creates cannot both claim it.
func nextPosition(ctx context.Context, q querier, spec orderedTable, parentID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `SELECT count(*) FROM `+spec.table+` WHERE `+spec.parentCol+` = $1`, parentID).Scan(&count)
	return count, err
}

// applyReorder validates and applies a complete reordering of a parent's live
// children.
//
// The contract is deliberately "the complete set, exactly once": a partial
// reorder cannot express where the unnamed rows should go, so accepting one
// would mean inventing an answer. Duplicates, foreign ids and omissions are
// each refused with the reason, and the whole permutation is one statement
// inside the caller's transaction.
func applyReorder(ctx context.Context, tx pgx.Tx, spec orderedTable, parentID string, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return errInvalidReorder("Provide the complete ordered list of ids.")
	}
	if len(orderedIDs) > MaxReorderEntries {
		return errInvalidReorder("Too many ids were supplied.")
	}
	seen := make(map[string]bool, len(orderedIDs))
	cleaned := make([]string, 0, len(orderedIDs))
	for _, raw := range orderedIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			return errInvalidReorder("Ids must not be blank.")
		}
		if seen[id] {
			return errInvalidReorder("The same id was supplied more than once.")
		}
		seen[id] = true
		cleaned = append(cleaned, id)
	}

	live := `SELECT id FROM ` + spec.table + ` WHERE ` + spec.parentCol + ` = $1`
	if spec.hasStatus {
		live += ` AND status <> 'ARCHIVED'`
	}
	rows, err := tx.Query(ctx, live+` ORDER BY position, id FOR UPDATE`, parentID)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing[id] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}

	for _, id := range cleaned {
		if !existing[id] {
			// Same message whether the id belongs to another parent, another
			// tenant or nothing at all: a reorder must not become an oracle for
			// what exists elsewhere.
			return errInvalidReorder("One or more ids do not belong to this collection.")
		}
	}
	if len(existing) != len(cleaned) {
		return errInvalidReorder("Provide every id in this collection exactly once.")
	}

	positions := make([]int32, len(cleaned))
	for index := range cleaned {
		positions[index] = int32(index)
	}
	if _, err = tx.Exec(ctx, `
		UPDATE `+spec.table+` t SET position = requested.position
		FROM (SELECT unnest($2::text[]) AS id, unnest($3::int[]) AS position) requested
		WHERE t.`+spec.parentCol+` = $1 AND t.id = requested.id`, parentID, cleaned, positions); err != nil {
		return err
	}
	return resequence(ctx, tx, spec, parentID)
}
