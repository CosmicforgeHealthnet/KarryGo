package shop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Actor is the authenticated vendor staff member behind a request. VendorID
// comes from the server-resolved session and never from the path or body.
type Actor struct {
	StaffID       string
	VendorID      string
	SessionID     string
	CorrelationID string
	CausationID   string
	// CommandRef is the request's Idempotency-Key, recorded on price history so
	// a duplicate-looking pair of versions can be told apart from a genuine
	// repeat of the same command.
	CommandRef string
}

// Scope is the actor's authorised reach for one permission.
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

// AuditRecorder writes an audit entry outside a transaction. Section 12A's own
// mutations use the transactional path instead, so evidence and state commit
// together; this port exists for the shape the rest of the service uses.
type AuditRecorder interface {
	Record(context.Context, AuditEntry) error
}

// EventPublisher appends a domain event onto the transactional outbox inside
// the caller's transaction.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string,
		aggregateVersion int64, vendorID, correlationID, causationID string, payload any) error
}

// MediaObject is the Media File Service's answer about one reference.
type MediaObject struct {
	BindingType    string
	BindingID      string
	Visibility     string
	OwnerService   string
	ID             string
	OwnerReference string
	UploadPurpose  string
	UploadStatus   string
	SafetyStatus   string
	DeletionState  string
	SourceVersion  int64
}

// MediaAuthority is the existing Media private-API ownership contract. Vendor
// Service stores an opaque reference and never the bytes.
type MediaAuthority interface {
	GetObject(ctx context.Context, businessID, mediaRef string) (MediaObject, error)
}

// Flags is the Section 12A slice of the service feature-flag surface.
type Flags struct {
	// ShopEnabled gates the whole feature (COMMERCE_SHOP_ENABLED). With it
	// false no Shop route is registered at all, so a disabled catalogue has no
	// attack surface rather than routes that answer 503 and confirm it exists.
	ShopEnabled bool
	// WritesEnabled gates every mutation; reads stay available so a rollout can
	// disable risky writes without blinding operators.
	WritesEnabled bool
	// CommerceKillSwitch is the platform-wide commerce stop. It overrides
	// WritesEnabled and can only ever withhold a mutation, never permit one.
	CommerceKillSwitch bool
}

func (f Flags) writesAllowed() bool { return f.WritesEnabled && !f.CommerceKillSwitch }

// Service is the Section 12A application layer. Every ownership, capability
// and lifecycle decision lives here or in the database, never in a handler.
type Service struct {
	db     *pgxpool.Pool
	scopes ScopeResolver
	audit  AuditRecorder
	events EventPublisher
	media  MediaAuthority
	flags  Flags
	now    func() time.Time
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
	// an unverified reference.
	Media MediaAuthority
	// Now is injectable so archival timestamps are deterministic in tests.
	Now func() time.Time
}

// NewService builds the Section 12A service.
func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: opts.DB, scopes: opts.Scopes, audit: opts.Audit, events: opts.Events,
		media: opts.Media, flags: opts.Flags, now: now}
}

// FlagSet exposes the configured flags for route registration and tests.
func (s *Service) FlagSet() Flags { return s.flags }

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------
//
// Six authorities, not one. Catalogue shape, retail price, wholesale terms,
// measurement vocabulary, branch listing and fresh configuration are separate
// grants because they are separate jobs: the person who corrects a product
// name is not automatically the person who may reprice a 50 kg bag.

const (
	PermRead             = "vendor.shop.read"
	PermCatalogueManage  = "vendor.shop.catalogue.manage"
	PermPricingManage    = "vendor.shop.pricing.manage"
	PermWholesaleManage  = "vendor.shop.wholesale.manage"
	PermMeasurementsMngt = "vendor.shop.measurements.manage"
	PermListingsManage   = "vendor.shop.listings.manage"
	PermFreshManage      = "vendor.shop.fresh.manage"
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
	if !s.flags.ShopEnabled {
		return apperrors.Unavailable("Shop catalogue management is not enabled.", nil)
	}
	if !s.flags.writesAllowed() {
		return apperrors.Unavailable("Shop catalogue changes are temporarily unavailable.", nil)
	}
	return nil
}

// requireBusinessWide gates the definitions that are shared across branches.
//
// A branch-scoped grant is deliberately not enough for a category, a product,
// a measure or a sell unit: every one of them is visible from every branch, so
// editing one from a single branch would silently change another branch's
// catalogue.
func requireBusinessWide(scope Scope, subject string) error {
	if !scope.Global {
		return apperrors.Forbidden(subject+" are shared across the business and need business-wide access.", nil)
	}
	return nil
}

// requirePricingAuthority is the gate on retail money, checked per field
// rather than per route: one endpoint that edits a name and a price must not
// hand price authority to whoever may edit the name.
//
// branchID empty means the price is business-level (a variant base or a sell
// unit), which only a business-wide grant covers.
func (s *Service) requirePricingAuthority(ctx context.Context, actor Actor, branchID string) error {
	return s.requireScopedAuthority(ctx, actor, PermPricingManage, branchID,
		"Catalogue-wide prices need business-wide pricing access.")
}

// requireWholesaleAuthority is the same gate for wholesale terms: base
// wholesale prices, quantity breaks and order minimums. It is separate from
// retail pricing on purpose — a branch manager may reprice a tin of milk
// without gaining authority over a distributor's tier table.
func (s *Service) requireWholesaleAuthority(ctx context.Context, actor Actor, branchID string) error {
	return s.requireScopedAuthority(ctx, actor, PermWholesaleManage, branchID,
		"Catalogue-wide wholesale terms need business-wide wholesale access.")
}

func (s *Service) requireScopedAuthority(ctx context.Context, actor Actor, permission, branchID, businessWideMessage string) error {
	scope, err := s.Authorize(ctx, actor, permission)
	if err != nil {
		return err
	}
	if branchID == "" {
		if !scope.Global {
			return apperrors.Forbidden(businessWideMessage, nil)
		}
		return nil
	}
	if !scope.Covers(branchID) {
		return errForbidden()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------
//
// Stable Section 12A codes travel in the error details alongside the platform
// envelope. "Not found" is deliberately indistinguishable from "exists but is
// not yours": anything else would let a caller enumerate another tenant's
// catalogue.

const (
	CodeCapabilityNotSupported = "SHOP_CAPABILITY_NOT_SUPPORTED"
	CodeOperatingModelInvalid  = "SHOP_OPERATING_MODEL_INVALID"
	CodeCategoryNotFound       = "SHOP_CATEGORY_NOT_FOUND"
	CodeCategoryCycle          = "SHOP_CATEGORY_CYCLE"
	CodeCategoryTooDeep        = "SHOP_CATEGORY_TOO_DEEP"
	CodeProductNotFound        = "SHOP_PRODUCT_NOT_FOUND"
	CodeVariantNotFound        = "SHOP_VARIANT_NOT_FOUND"
	CodeMeasureNotFound        = "SHOP_MEASURE_NOT_FOUND"
	CodeCalibrationNotFound    = "SHOP_CALIBRATION_NOT_FOUND"
	CodeSellUnitNotFound       = "SHOP_SELL_UNIT_NOT_FOUND"
	CodeIdentifierNotFound     = "SHOP_IDENTIFIER_NOT_FOUND"
	CodePackageNotFound        = "SHOP_PACKAGE_RELATION_NOT_FOUND"
	CodePackageCycle           = "SHOP_PACKAGE_CYCLE"
	CodeListingNotFound        = "SHOP_LISTING_NOT_FOUND"
	CodeQuantityBreakNotFound  = "SHOP_QUANTITY_BREAK_NOT_FOUND"
	CodeQuantityBreakOverlap   = "SHOP_QUANTITY_BREAK_OVERLAP"
	CodeFreshAttributeNotFound = "SHOP_FRESH_ATTRIBUTE_NOT_FOUND"
	CodeFreshAttributeInvalid  = "SHOP_FRESH_ATTRIBUTE_NOT_APPLICABLE"
	CodeStaleVersion           = "SHOP_STALE_VERSION"
	CodeArchived               = "SHOP_ENTITY_ARCHIVED"
	CodeDuplicate              = "SHOP_DUPLICATE"
	CodeMediaUnavailable       = "SHOP_MEDIA_UNAVAILABLE"
	CodeConversionInvalid      = "SHOP_CONVERSION_INVALID"
	CodeApproximateNotExact    = "SHOP_APPROXIMATE_CONVERSION"
	CodeIdempotencyConflict    = "SHOP_IDEMPOTENCY_CONFLICT"
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

func validation(violations ...apperrors.FieldViolation) error {
	return apperrors.Validation("Check your details.", violations)
}

func fieldError(field, message string) error {
	return validation(apperrors.FieldViolation{Field: field, Message: message})
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
			return coded(apperrors.Conflict(label+" with these details already exists.", nil), CodeDuplicate)
		case "23503":
			return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "reference", Message: "Referenced resource does not belong to this business."}}), CodeDuplicate)
		case "23514":
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "request", Message: "A supplied value is outside the supported range."}})
		case "23P01":
			// The quantity-break overlap trigger. A pricing ambiguity is a
			// commercial defect, so the database refuses it even if a service
			// check is ever bypassed.
			return coded(apperrors.Conflict("This quantity band overlaps an existing active band.", nil),
				CodeQuantityBreakOverlap)
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

// auditTx writes the audit row inside the mutation's own transaction, which is
// what makes the evidence as durable as the change.
func (s *Service) auditTx(ctx context.Context, tx pgx.Tx, actor Actor, ev evidence) error {
	beforeJSON, _ := json.Marshal(ev.before)
	afterJSON, _ := json.Marshal(ev.after)
	actorType := "vendor_staff"
	if actor.StaffID == "" {
		actorType = "system"
	}
	_, err := tx.Exec(ctx, `INSERT INTO vendor_audit_log
		(id,vendor_id,actor_type,actor_id,action,aggregate_type,aggregate_id,branch_id,correlation_id,before_data,after_data,reason)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,NULLIF($12,''))`,
		"val_"+uuid.NewString(), actor.VendorID, actorType, actor.StaffID, ev.action, ev.aggregateType,
		ev.aggregateID, ev.branchID, actor.CorrelationID,
		nilJSON(ev.before, beforeJSON), nilJSON(ev.after, afterJSON), ev.reason)
	return err
}

func (s *Service) publishTx(ctx context.Context, tx pgx.Tx, actor Actor, ev evidence) error {
	if s.events == nil {
		return nil
	}
	return s.events.Publish(ctx, tx, ev.eventType, ev.aggregateType, ev.aggregateID, ev.version,
		actor.VendorID, actor.CorrelationID, actor.CausationID, ev.payload)
}

// record writes audit and outbox together for one mutation. Every command in
// this package goes through it, so "audited" and "published" are not decisions
// a new handler can forget to make.
func (s *Service) record(ctx context.Context, tx pgx.Tx, actor Actor, ev evidence) error {
	if err := s.auditTx(ctx, tx, actor, ev); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, ev)
}

func nilJSON(v any, raw []byte) any {
	if v == nil {
		return nil
	}
	return raw
}

func newID(prefix string) string { return prefix + uuid.NewString() }

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type rowScanner interface{ Scan(dest ...any) error }

// ---------------------------------------------------------------------------
// Capability and branch scope
// ---------------------------------------------------------------------------

// requireShopBranch proves the branch exists in this business and holds at
// least one Shop capability, reading Section 6/7 state rather than
// re-deciding it.
//
// This is the Shop/Food isolation gate at its root: a RESTAURANT-only branch
// has no Shop capability, so it can hold no Shop catalogue, no matter what
// other branches of the same business are licensed to do.
func (s *Service) requireShopBranch(ctx context.Context, q querier, businessID, branchID string) ([]string, error) {
	var branchStatus string
	err := q.QueryRow(ctx, `
		SELECT b.status FROM vendor_branches b
		WHERE b.business_id=$1 AND b.id=$2`, businessID, strings.TrimSpace(branchID)).Scan(&branchStatus)
	if err != nil {
		return nil, mapDB(err, "Branch", CodeCapabilityNotSupported)
	}
	rows, err := q.Query(ctx, `
		SELECT capability_type FROM vendor_branch_capabilities
		WHERE business_id=$1 AND branch_id=$2 AND capability_type = ANY($3) AND status IN ('ACTIVE','DRAFT','PAUSED')
		ORDER BY capability_type`, businessID, strings.TrimSpace(branchID), ShopCapabilities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var capabilities []string
	for rows.Next() {
		var capability string
		if err = rows.Scan(&capability); err != nil {
			return nil, err
		}
		capabilities = append(capabilities, capability)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(capabilities) == 0 {
		return nil, coded(apperrors.Conflict("This branch holds no Shop, Market or Wholesale capability.", nil),
			CodeCapabilityNotSupported)
	}
	return capabilities, nil
}

// admitOperatingModel picks the capability that admits a model at this branch,
// or refuses. A FOODSTUFF_DEALER may be admitted by SHOP_GROCERY or WHOLESALE;
// a MARKET_TRADER only by MARKET.
func admitOperatingModel(model string, capabilities []string) (string, error) {
	eligible := operatingModelCapabilities[model]
	if len(eligible) == 0 {
		return "", coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "operating_model", Message: "Unknown operating model."}}), CodeOperatingModelInvalid)
	}
	for _, candidate := range eligible {
		if contains(capabilities, candidate) {
			return candidate, nil
		}
	}
	return "", coded(apperrors.Conflict(
		"This branch does not hold a capability that admits the "+model+" operating model.", nil),
		CodeCapabilityNotSupported)
}

// requireBranchScope resolves a branch the caller must both own and reach.
func (s *Service) requireBranchScope(scope Scope, branchID string) error {
	if strings.TrimSpace(branchID) == "" {
		return fieldError("branch_id", "Branch is required.")
	}
	if !scope.Covers(strings.TrimSpace(branchID)) {
		return errForbidden()
	}
	return nil
}

// businessCurrency reads the one authoritative currency. Catalogue rows do not
// store it, so there is nothing that can disagree with Section 6.
func (s *Service) businessCurrency(ctx context.Context, q querier, businessID string) (string, error) {
	var currency string
	err := q.QueryRow(ctx, `SELECT currency FROM vendor_businesses WHERE id=$1`, businessID).Scan(&currency)
	if err != nil {
		return "", mapDB(err, "Business", CodeProductNotFound)
	}
	return currency, nil
}

// ---------------------------------------------------------------------------
// Catalogue revision
// ---------------------------------------------------------------------------

// bumpBranchRevision advances one branch's Shop catalogue revision inside the
// caller's transaction.
func bumpBranchRevision(ctx context.Context, tx pgx.Tx, businessID, branchID string) (int64, error) {
	if branchID == "" {
		return 0, nil
	}
	var revision int64
	err := tx.QueryRow(ctx, `
		INSERT INTO shop_catalogue_revisions (business_id, branch_id, domain, revision, updated_at)
		VALUES ($1,$2,'SHOP',1,now())
		ON CONFLICT (business_id, branch_id, domain)
		DO UPDATE SET revision = shop_catalogue_revisions.revision + 1, updated_at = now()
		RETURNING revision`, businessID, branchID).Scan(&revision)
	return revision, err
}

// bumpRevisionsForVariant advances every branch that lists a variant.
//
// A product, sell unit or price change is business-level but is SEEN per
// branch, so the fan-out is over the branches that actually list it rather
// than over every branch the business owns.
func bumpRevisionsForVariant(ctx context.Context, tx pgx.Tx, businessID, variantID string) ([]int64, error) {
	return bumpRevisionsForQuery(ctx, tx, businessID, `
		SELECT DISTINCT branch_id FROM shop_branch_listings
		WHERE business_id=$1 AND variant_id=$2 ORDER BY 1`, variantID)
}

// bumpRevisionsForProduct advances every branch that lists any variant of a
// product.
func bumpRevisionsForProduct(ctx context.Context, tx pgx.Tx, businessID, productID string) ([]int64, error) {
	return bumpRevisionsForQuery(ctx, tx, businessID, `
		SELECT DISTINCT branch_id FROM shop_branch_listings
		WHERE business_id=$1 AND product_id=$2 ORDER BY 1`, productID)
}

// bumpRevisionsForCategory advances every branch listing a product in the
// category, so a category rename or archive is visible to a Part B reader.
func bumpRevisionsForCategory(ctx context.Context, tx pgx.Tx, businessID, categoryID string) ([]int64, error) {
	return bumpRevisionsForQuery(ctx, tx, businessID, `
		SELECT DISTINCT l.branch_id FROM shop_branch_listings l
		JOIN shop_products p ON p.id = l.product_id AND p.business_id = l.business_id
		WHERE l.business_id=$1 AND p.category_id=$2 ORDER BY 1`, categoryID)
}

// bumpRevisionsForMeasure advances branches whose listed variants expose a
// sell unit in this measure. A regional alias or display-name change is part
// of what catalogue clients render, so it cannot leave their revision stale.
func bumpRevisionsForMeasure(ctx context.Context, tx pgx.Tx, businessID, measureID string) ([]int64, error) {
	return bumpRevisionsForQuery(ctx, tx, businessID, `
		SELECT DISTINCT l.branch_id FROM shop_branch_listings l
		JOIN shop_sell_units u ON u.business_id=l.business_id AND u.variant_id=l.variant_id
		WHERE l.business_id=$1 AND u.measure_definition_id=$2 ORDER BY 1`, measureID)
}

func bumpRevisionsForQuery(ctx context.Context, tx pgx.Tx, businessID, sql, argument string) ([]int64, error) {
	rows, err := tx.Query(ctx, sql, businessID, argument)
	if err != nil {
		return nil, err
	}
	var branches []string
	for rows.Next() {
		var branchID string
		if err = rows.Scan(&branchID); err != nil {
			rows.Close()
			return nil, err
		}
		branches = append(branches, branchID)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	revisions := make([]int64, 0, len(branches))
	for _, branchID := range branches {
		revision, bumpErr := bumpBranchRevision(ctx, tx, businessID, branchID)
		if bumpErr != nil {
			return nil, bumpErr
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

// CatalogueRevision reads one branch's current Shop revision.
func (s *Service) CatalogueRevision(ctx context.Context, actor Actor, scope Scope, branchID string) (int64, time.Time, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return 0, time.Time{}, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return 0, time.Time{}, err
	}
	var revision int64
	var updated time.Time
	err := s.db.QueryRow(ctx, `
		SELECT revision, updated_at FROM shop_catalogue_revisions
		WHERE business_id=$1 AND branch_id=$2 AND domain='SHOP'`,
		actor.VendorID, strings.TrimSpace(branchID)).Scan(&revision, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, s.now(), nil
	}
	return revision, updated, err
}

// ---------------------------------------------------------------------------
// Price history
// ---------------------------------------------------------------------------

// priceChange is one append-only price record to write inside a mutation.
type priceChange struct {
	subjectType    string
	subjectRef     string
	branchID       string
	productRef     string
	variantRef     string
	previousAmount *int64
	amount         *int64
	reason         string
	subjectVersion int64
}

// appendPriceVersion writes one immutable price record, allocating the next
// per-subject revision inside the transaction.
//
// Nothing overwrites commercial history here: a future Order snapshots
// (subject_type, subject_ref, revision) and never recomputes a historical
// total from today's catalogue.
func (s *Service) appendPriceVersion(ctx context.Context, tx pgx.Tx, actor Actor, currency string, change priceChange) error {
	var revision int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(revision),0)+1 FROM shop_price_versions
		WHERE subject_type=$1 AND subject_ref=$2`, change.subjectType, change.subjectRef).Scan(&revision); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO shop_price_versions
			(id,business_id,branch_id,subject_type,subject_ref,product_ref,variant_ref,currency,
			 previous_amount_minor,amount_minor,previous_inherited,inherited,change_reason,
			 subject_version,revision,actor_type,actor_id,correlation_id,command_ref)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,NULLIF($17,''),NULLIF($18,''),NULLIF($19,''))`,
		newID("spv_"), actor.VendorID, change.branchID, change.subjectType, change.subjectRef,
		change.productRef, change.variantRef, currency,
		change.previousAmount, change.amount, change.previousAmount == nil, change.amount == nil,
		change.reason, change.subjectVersion, revision,
		actorTypeOf(actor), actor.StaffID, actor.CorrelationID, actor.CommandRef)
	return err
}

func actorTypeOf(actor Actor) string {
	if actor.StaffID == "" {
		return "system"
	}
	return "vendor_staff"
}

// priceReason names the change so history is readable without diffing.
func priceReason(previous, next *int64) string {
	switch {
	case previous == nil && next != nil:
		return ReasonCreated
	case previous != nil && next == nil:
		return ReasonOverrideCleared
	case previous == nil && next == nil:
		return ReasonOverrideCleared
	default:
		return ReasonPriceChanged
	}
}

func moneyChanged(previous, next *int64) bool {
	switch {
	case previous == nil && next == nil:
		return false
	case previous == nil || next == nil:
		return true
	default:
		return *previous != *next
	}
}

// ---------------------------------------------------------------------------
// Shared query helpers
// ---------------------------------------------------------------------------

func boundedPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > MaxPageSize {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func expectedVersionMatches(current, expected int64) error {
	if expected != 0 && expected != current {
		return errStaleVersion()
	}
	return nil
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
