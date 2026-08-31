package purchasing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

type PGXTx = pgx.Tx

type DomainEventPublisher interface {
	Publish(
		ctx context.Context,
		tx pgx.Tx,
		eventType string,
		aggregateType string,
		aggregateID string,
		aggregateVersion int64,
		vendorID string,
		correlationID string,
		causationID string,
		payload any,
	) error
}

type Options struct {
	DB            *pgxpool.Pool
	Scopes        ScopeResolver
	Events        DomainEventPublisher
	Enabled       bool
	WritesEnabled bool
}

type Service struct {
	db            *pgxpool.Pool
	scopes        ScopeResolver
	events        DomainEventPublisher
	enabled       bool
	writesEnabled bool
}

func NewService(opts Options) *Service {
	return &Service{
		db:            opts.DB,
		scopes:        opts.Scopes,
		events:        opts.Events,
		enabled:       opts.Enabled,
		writesEnabled: opts.WritesEnabled,
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.enabled
}

func (s *Service) ResolveScope(
	ctx context.Context,
	actor Actor,
	permission string,
) (Scope, error) {
	if s == nil || !s.enabled || s.scopes == nil || !actor.Validate() {
		return Scope{}, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	scope, err := s.scopes.ResolveScope(
		ctx,
		actor.StaffID,
		permission,
	)
	if err != nil {
		return Scope{}, err
	}

	if !scope.HasAny() {
		return Scope{}, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	return normalizeScope(scope), nil
}

func normalizeScope(scope Scope) Scope {
	if scope.Global {
		return Scope{Global: true}
	}

	seen := map[string]bool{}
	out := Scope{}

	for _, branch := range scope.BranchIDs {
		branch = strings.TrimSpace(branch)
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		out.BranchIDs = append(out.BranchIDs, branch)
	}

	return out
}

func (s *Service) requireWrite() error {
	if s == nil || !s.enabled || !s.writesEnabled || s.db == nil {
		return apperrors.Forbidden(
			"Purchasing changes are currently disabled.",
			nil,
		)
	}
	return nil
}

func requireGlobal(scope Scope) error {
	if !scope.Global {
		return apperrors.Forbidden(
			"Vendor-wide purchasing authority is required.",
			nil,
		)
	}
	return nil
}

func requireBranch(scope Scope, branchID string) error {
	if !scope.Covers(branchID) {
		return apperrors.Forbidden(
			"Purchasing access is not permitted for this branch.",
			nil,
		)
	}
	return nil
}

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,79}$`)

func normalizedCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizedCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", validation(
			"currency_code",
			"Use a three-letter ISO currency code.",
		)
	}

	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return "", validation(
				"currency_code",
				"Use a three-letter ISO currency code.",
			)
		}
	}

	return value, nil
}

func validation(field, message string) error {
	return apperrors.Validation(
		"Check your details.",
		[]apperrors.FieldViolation{{
			Field:   field,
			Message: message,
		}},
	)
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return apperrors.Conflict(
				"A purchasing record with these details already exists.",
				err,
			)
		case "23503":
			return apperrors.Conflict(
				"A referenced purchasing or inventory record is no longer valid.",
				err,
			)
		case "23514":
			return apperrors.BadRequest(
				"The purchasing record violates a data rule.",
				err,
			)
		}
	}

	return err
}

func supplierStatus(value string, allowArchived bool) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = StatusActive
	}

	switch value {
	case StatusActive, StatusInactive, StatusSuspended:
		return value, nil
	case StatusArchived:
		if allowArchived {
			return value, nil
		}
	}

	return "", validation(
		"status",
		"Use ACTIVE, INACTIVE or SUSPENDED.",
	)
}

func contactRole(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = ContactRoleOther
	}

	switch value {
	case ContactRolePrimary,
		ContactRoleSales,
		ContactRoleOperations,
		ContactRoleFinance,
		ContactRoleOther:
		return value, nil
	default:
		return "", validation(
			"contact_role",
			"Choose a supported supplier contact role.",
		)
	}
}

func validateLeadTime(value int) error {
	if value < 0 || value > 8760 {
		return validation(
			"default_lead_time_hours",
			"Lead time must be between 0 and 8,760 hours.",
		)
	}
	return nil
}

func validateOptionalLeadTime(value *int) error {
	if value == nil {
		return nil
	}
	if *value < 0 || *value > 8760 {
		return validation(
			"lead_time_hours",
			"Lead time must be between 0 and 8,760 hours.",
		)
	}
	return nil
}

func validateCosts(value *int64) error {
	if value != nil && *value < 0 {
		return validation(
			"typical_cost_minor",
			"Cost cannot be negative.",
		)
	}
	return nil
}

func NormalizeExactConversion(
	mode string,
	factor int64,
) (string, int64, error) {
	mode = strings.ToUpper(strings.TrimSpace(mode))

	if factor <= 0 {
		return "", 0, validation(
			"inventory_units_per_purchase_unit",
			"Exact Inventory conversion must be a positive integer.",
		)
	}

	if mode == "" {
		if factor == 1 {
			mode = ConversionSameAsInventory
		} else {
			mode = ConversionExactInteger
		}
	}

	switch mode {
	case ConversionSameAsInventory:
		if factor != 1 {
			return "", 0, validation(
				"inventory_units_per_purchase_unit",
				"SAME_AS_INVENTORY requires exactly one Inventory unit per purchase unit.",
			)
		}

	case ConversionExactInteger:
		// Any positive integer is exact. No approximate or calibrated mode is
		// accepted by this domain.

	default:
		return "", 0, validation(
			"conversion_mode",
			"Use SAME_AS_INVENTORY or EXACT_INTEGER_MULTIPLE. Approximate conversion is not accepted.",
		)
	}

	return mode, factor, nil
}

func ConvertPurchaseQuantity(
	purchaseQuantity int64,
	inventoryUnitsPerPurchaseUnit int64,
) (int64, error) {
	if purchaseQuantity <= 0 ||
		inventoryUnitsPerPurchaseUnit <= 0 {
		return 0, validation(
			"quantity",
			"Purchase quantity and exact conversion factor must be positive integers.",
		)
	}

	if purchaseQuantity >
		math.MaxInt64/inventoryUnitsPerPurchaseUnit {
		return 0, validation(
			"quantity",
			"Converted Inventory quantity exceeds the supported range.",
		)
	}

	return purchaseQuantity * inventoryUnitsPerPurchaseUnit, nil
}

func begin(ctx context.Context, db *pgxpool.Pool) (pgx.Tx, error) {
	if db == nil {
		return nil, errors.New("purchasing database is unavailable")
	}
	return db.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
	)
}

func marshalJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func (s *Service) recordMutation(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	action string,
	eventType string,
	aggregateType string,
	aggregateID string,
	branchID string,
	version int64,
	before any,
	after any,
	eventPayload map[string]any,
) error {
	beforeJSON, err := marshalJSON(before)
	if err != nil {
		return err
	}

	afterJSON, err := marshalJSON(after)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO vendor_audit_log (
            id,
            vendor_id,
            actor_type,
            actor_id,
            action,
            aggregate_type,
            aggregate_id,
            branch_id,
            before_data,
            after_data,
            reason
        )
        VALUES (
            $1,$2,'vendor_staff',$3,$4,$5,$6,
            NULLIF($7,''),$8,$9,NULL
        )
        `,
		"val_"+uuid.NewString(),
		actor.VendorID,
		actor.StaffID,
		action,
		aggregateType,
		aggregateID,
		branchID,
		beforeJSON,
		afterJSON,
	)
	if err != nil {
		return err
	}

	if s.events == nil {
		return nil
	}

	if eventPayload == nil {
		eventPayload = map[string]any{}
	}

	eventPayload["business_id"] = actor.VendorID
	eventPayload["aggregate_id"] = aggregateID
	eventPayload["version"] = version
	if branchID != "" {
		eventPayload["branch_id"] = branchID
	}

	return s.events.Publish(
		ctx,
		tx,
		eventType,
		aggregateType,
		aggregateID,
		version,
		actor.VendorID,
		actor.CorrelationID,
		actor.CausationID,
		eventPayload,
	)
}

type rowScanner interface {
	Scan(...any) error
}

const categoryColumns = `
    id,
    business_id,
    code,
    name,
    description,
    status,
    version,
    created_at,
    updated_at
`

func scanCategory(row rowScanner) (SupplierCategory, error) {
	var out SupplierCategory
	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

const supplierColumns = `
    id,
    business_id,
    category_id,
    code,
    name,
    legal_name,
    default_lead_time_hours,
    status,
    notes,
    version,
    created_at,
    updated_at
`

func scanSupplier(row rowScanner) (Supplier, error) {
	var out Supplier
	var category sql.NullString

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&category,
		&out.Code,
		&out.Name,
		&out.LegalName,
		&out.DefaultLeadTimeHours,
		&out.Status,
		&out.Notes,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return Supplier{}, err
	}

	if category.Valid {
		value := category.String
		out.CategoryID = &value
	}

	return out, nil
}

const contactColumns = `
    id,
    business_id,
    supplier_id,
    contact_role,
    name,
    email,
    phone,
    is_primary,
    status,
    version,
    created_at,
    updated_at
`

func scanContact(row rowScanner) (SupplierContact, error) {
	var out SupplierContact
	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.SupplierID,
		&out.ContactRole,
		&out.Name,
		&out.Email,
		&out.Phone,
		&out.IsPrimary,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

const termsColumns = `
    supplier_id,
    business_id,
    payment_terms_days,
    currency_code,
    delivery_terms,
    version,
    created_at,
    updated_at
`

func scanTerms(row rowScanner) (SupplierTerms, error) {
	var out SupplierTerms
	err := row.Scan(
		&out.SupplierID,
		&out.BusinessID,
		&out.PaymentTermsDays,
		&out.CurrencyCode,
		&out.DeliveryTerms,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

const supplierProductColumns = `
    id,
    business_id,
    supplier_id,
    branch_id,
    inventory_item_id,
    supplier_sku,
    purchase_unit_code,
    purchase_unit_name,
    conversion_mode,
    inventory_units_per_purchase_unit,
    minimum_order_quantity,
    purchase_multiple,
    typical_cost_minor,
    last_cost_minor,
    currency_code,
    lead_time_hours,
    inventory_measure_ref,
    inventory_measure_owner_key,
    inventory_unit_kind,
    status,
    version,
    created_at,
    updated_at
`

func scanSupplierProduct(row rowScanner) (SupplierProduct, error) {
	var out SupplierProduct

	var typical sql.NullInt64
	var last sql.NullInt64
	var lead sql.NullInt64

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.SupplierID,
		&out.BranchID,
		&out.InventoryItemID,
		&out.SupplierSKU,
		&out.PurchaseUnitCode,
		&out.PurchaseUnitName,
		&out.ConversionMode,
		&out.InventoryUnitsPerPurchaseUnit,
		&out.MinimumOrderQuantity,
		&out.PurchaseMultiple,
		&typical,
		&last,
		&out.CurrencyCode,
		&lead,
		&out.InventoryMeasureRef,
		&out.InventoryMeasureOwnerKey,
		&out.InventoryUnitKind,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return SupplierProduct{}, err
	}

	if typical.Valid {
		value := typical.Int64
		out.TypicalCostMinor = &value
	}

	if last.Valid {
		value := last.Int64
		out.LastCostMinor = &value
	}

	if lead.Valid {
		value := int(lead.Int64)
		out.LeadTimeHours = &value
	}

	return out, nil
}

func notFound(entity string) error {
	return apperrors.NotFound(
		entity+" was not found.",
		nil,
	)
}

func versionConflict() error {
	return apperrors.Conflict(
		"The purchasing record changed since you loaded it.",
		nil,
	)
}

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// -----------------------------------------------------------------------------
// Supplier categories
// -----------------------------------------------------------------------------

func (s *Service) ListCategories(
	ctx context.Context,
	actor Actor,
	scope Scope,
	limit int,
	offset int,
) ([]SupplierCategory, error) {
	if s == nil || s.db == nil || !s.enabled || !actor.Validate() ||
		!scope.HasAny() {
		return nil, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	limit, offset = normalizePage(limit, offset)

	rows, err := s.db.Query(
		ctx,
		`SELECT `+categoryColumns+`
           FROM vendor_supplier_categories
          WHERE business_id=$1
          ORDER BY name,id
          LIMIT $2 OFFSET $3`,
		actor.VendorID,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SupplierCategory, 0)
	for rows.Next() {
		item, scanErr := scanCategory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) CreateCategory(
	ctx context.Context,
	actor Actor,
	scope Scope,
	input CreateCategoryInput,
) (SupplierCategory, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierCategory{}, err
	}
	if !actor.Validate() {
		return SupplierCategory{}, apperrors.Unauthorized(
			"Authentication is required.",
			nil,
		)
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierCategory{}, err
	}

	input.Code = normalizedCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)

	if !codePattern.MatchString(input.Code) {
		return SupplierCategory{}, validation(
			"code",
			"Use uppercase letters, digits, underscore or hyphen.",
		)
	}
	if input.Name == "" || len(input.Name) > 160 {
		return SupplierCategory{}, validation(
			"name",
			"Provide a supplier category name.",
		)
	}
	if len(input.Description) > 1000 {
		return SupplierCategory{}, validation(
			"description",
			"Description is too long.",
		)
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierCategory{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := scanCategory(
		tx.QueryRow(
			ctx,
			`INSERT INTO vendor_supplier_categories (
                id,business_id,code,name,description
             )
             VALUES ($1,$2,$3,$4,$5)
             RETURNING `+categoryColumns,
			"vscat_"+uuid.NewString(),
			actor.VendorID,
			input.Code,
			input.Name,
			input.Description,
		),
	)
	if err != nil {
		return SupplierCategory{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_category.created",
		"vendor.purchasing.supplier_category.created",
		"vendor_supplier_category",
		out.ID,
		"",
		out.Version,
		nil,
		out,
		map[string]any{
			"code":   out.Code,
			"status": out.Status,
		},
	); err != nil {
		return SupplierCategory{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierCategory{}, err
	}

	return out, nil
}

func (s *Service) UpdateCategory(
	ctx context.Context,
	actor Actor,
	scope Scope,
	categoryID string,
	input UpdateCategoryInput,
) (SupplierCategory, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierCategory{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierCategory{}, err
	}
	if input.ExpectedVersion <= 0 {
		return SupplierCategory{}, versionConflict()
	}

	categoryID = strings.TrimSpace(categoryID)
	input.Code = normalizedCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)

	if categoryID == "" {
		return SupplierCategory{}, notFound("Supplier category")
	}
	if !codePattern.MatchString(input.Code) {
		return SupplierCategory{}, validation(
			"code",
			"Use uppercase letters, digits, underscore or hyphen.",
		)
	}
	if input.Name == "" || len(input.Name) > 160 {
		return SupplierCategory{}, validation(
			"name",
			"Provide a supplier category name.",
		)
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierCategory{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanCategory(
		tx.QueryRow(
			ctx,
			`SELECT `+categoryColumns+`
               FROM vendor_supplier_categories
              WHERE business_id=$1
                AND id=$2
              FOR UPDATE`,
			actor.VendorID,
			categoryID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierCategory{}, notFound("Supplier category")
	}
	if err != nil {
		return SupplierCategory{}, err
	}
	if current.Status == StatusArchived {
		return SupplierCategory{}, apperrors.Conflict(
			"Archived supplier categories cannot be changed.",
			nil,
		)
	}
	if current.Version != input.ExpectedVersion {
		return SupplierCategory{}, versionConflict()
	}

	out, err := scanCategory(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_supplier_categories
                SET code=$3,
                    name=$4,
                    description=$5,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND id=$2
              RETURNING `+categoryColumns,
			actor.VendorID,
			categoryID,
			input.Code,
			input.Name,
			input.Description,
		),
	)
	if err != nil {
		return SupplierCategory{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_category.updated",
		"vendor.purchasing.supplier_category.updated",
		"vendor_supplier_category",
		out.ID,
		"",
		out.Version,
		current,
		out,
		map[string]any{
			"code":   out.Code,
			"status": out.Status,
		},
	); err != nil {
		return SupplierCategory{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierCategory{}, err
	}

	return out, nil
}

func (s *Service) ArchiveCategory(
	ctx context.Context,
	actor Actor,
	scope Scope,
	categoryID string,
	expectedVersion int64,
) (SupplierCategory, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierCategory{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierCategory{}, err
	}
	if expectedVersion <= 0 {
		return SupplierCategory{}, versionConflict()
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierCategory{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanCategory(
		tx.QueryRow(
			ctx,
			`SELECT `+categoryColumns+`
               FROM vendor_supplier_categories
              WHERE business_id=$1
                AND id=$2
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(categoryID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierCategory{}, notFound("Supplier category")
	}
	if err != nil {
		return SupplierCategory{}, err
	}
	if current.Version != expectedVersion {
		return SupplierCategory{}, versionConflict()
	}
	if current.Status == StatusArchived {
		return current, nil
	}

	out, err := scanCategory(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_supplier_categories
                SET status='ARCHIVED',
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND id=$2
              RETURNING `+categoryColumns,
			actor.VendorID,
			current.ID,
		),
	)
	if err != nil {
		return SupplierCategory{}, err
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_category.archived",
		"vendor.purchasing.supplier_category.archived",
		"vendor_supplier_category",
		out.ID,
		"",
		out.Version,
		current,
		out,
		map[string]any{
			"code":   out.Code,
			"status": out.Status,
		},
	); err != nil {
		return SupplierCategory{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierCategory{}, err
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Suppliers
// -----------------------------------------------------------------------------

func (s *Service) validateCategory(
	ctx context.Context,
	q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	businessID string,
	categoryID *string,
) (*string, error) {
	if categoryID == nil {
		return nil, nil
	}

	value := strings.TrimSpace(*categoryID)
	if value == "" {
		return nil, nil
	}

	var status string
	err := q.QueryRow(
		ctx,
		`SELECT status
           FROM vendor_supplier_categories
          WHERE business_id=$1
            AND id=$2`,
		businessID,
		value,
	).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, validation(
			"category_id",
			"Choose an active supplier category owned by this Vendor.",
		)
	}
	if err != nil {
		return nil, err
	}
	if status != StatusActive {
		return nil, validation(
			"category_id",
			"Choose an active supplier category.",
		)
	}

	return &value, nil
}

func (s *Service) ListSuppliers(
	ctx context.Context,
	actor Actor,
	scope Scope,
	branchFilter string,
	limit int,
	offset int,
) ([]Supplier, error) {
	if s == nil || s.db == nil || !s.enabled || !actor.Validate() ||
		!scope.HasAny() {
		return nil, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	branchFilter = strings.TrimSpace(branchFilter)
	if branchFilter != "" && !scope.Covers(branchFilter) {
		return nil, apperrors.Forbidden(
			"Purchasing access is not permitted for this branch.",
			nil,
		)
	}

	limit, offset = normalizePage(limit, offset)

	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}

	rows, err := s.db.Query(
		ctx,
		`SELECT `+supplierColumns+`
           FROM vendor_suppliers s
          WHERE s.business_id=$1
            AND (
                $2::boolean
                OR EXISTS (
                    SELECT 1
                      FROM vendor_supplier_products sp
                     WHERE sp.business_id=s.business_id
                       AND sp.supplier_id=s.id
                       AND sp.status='ACTIVE'
                       AND sp.branch_id=ANY($3::text[])
                )
            )
            AND (
                $4::text=''
                OR EXISTS (
                    SELECT 1
                      FROM vendor_supplier_products sp
                     WHERE sp.business_id=s.business_id
                       AND sp.supplier_id=s.id
                       AND sp.status='ACTIVE'
                       AND sp.branch_id=$4
                )
            )
          ORDER BY s.name,s.id
          LIMIT $5 OFFSET $6`,
		actor.VendorID,
		scope.Global,
		branches,
		branchFilter,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Supplier, 0)
	for rows.Next() {
		item, scanErr := scanSupplier(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) GetSupplier(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
) (Supplier, error) {
	if s == nil || s.db == nil || !s.enabled || !actor.Validate() ||
		!scope.HasAny() {
		return Supplier{}, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}

	out, err := scanSupplier(
		s.db.QueryRow(
			ctx,
			`SELECT `+supplierColumns+`
               FROM vendor_suppliers s
              WHERE s.business_id=$1
                AND s.id=$2
                AND (
                    $3::boolean
                    OR EXISTS (
                        SELECT 1
                          FROM vendor_supplier_products sp
                         WHERE sp.business_id=s.business_id
                           AND sp.supplier_id=s.id
                           AND sp.status='ACTIVE'
                           AND sp.branch_id=ANY($4::text[])
                    )
                )`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
			scope.Global,
			branches,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Supplier{}, notFound("Supplier")
	}
	return out, err
}

func (s *Service) CreateSupplier(
	ctx context.Context,
	actor Actor,
	scope Scope,
	input CreateSupplierInput,
) (Supplier, error) {
	if err := s.requireWrite(); err != nil {
		return Supplier{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return Supplier{}, err
	}

	input.Code = normalizedCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.LegalName = strings.TrimSpace(input.LegalName)
	input.Notes = strings.TrimSpace(input.Notes)

	if input.Code != "" && !codePattern.MatchString(input.Code) {
		return Supplier{}, validation(
			"code",
			"Use uppercase letters, digits, underscore or hyphen.",
		)
	}
	if input.Name == "" || len(input.Name) > 200 {
		return Supplier{}, validation(
			"name",
			"Provide a supplier name.",
		)
	}
	if len(input.LegalName) > 240 || len(input.Notes) > 4000 {
		return Supplier{}, validation(
			"notes",
			"Supplier details are too long.",
		)
	}
	if err := validateLeadTime(input.DefaultLeadTimeHours); err != nil {
		return Supplier{}, err
	}

	status, err := supplierStatus(input.Status, false)
	if err != nil {
		return Supplier{}, err
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return Supplier{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	categoryID, err := s.validateCategory(
		ctx,
		tx,
		actor.VendorID,
		input.CategoryID,
	)
	if err != nil {
		return Supplier{}, err
	}

	out, err := scanSupplier(
		tx.QueryRow(
			ctx,
			`INSERT INTO vendor_suppliers (
                id,
                business_id,
                category_id,
                code,
                name,
                legal_name,
                default_lead_time_hours,
                status,
                notes
             )
             VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,$9
             )
             RETURNING `+supplierColumns,
			"vsup_"+uuid.NewString(),
			actor.VendorID,
			categoryID,
			input.Code,
			input.Name,
			input.LegalName,
			input.DefaultLeadTimeHours,
			status,
			input.Notes,
		),
	)
	if err != nil {
		return Supplier{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier.created",
		"vendor.purchasing.supplier.created",
		"vendor_supplier",
		out.ID,
		"",
		out.Version,
		nil,
		out,
		map[string]any{
			"supplier_id": out.ID,
			"status":      out.Status,
			"category_id": out.CategoryID,
		},
	); err != nil {
		return Supplier{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Supplier{}, err
	}

	return out, nil
}

func (s *Service) UpdateSupplier(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	input UpdateSupplierInput,
) (Supplier, error) {
	if err := s.requireWrite(); err != nil {
		return Supplier{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return Supplier{}, err
	}
	if input.ExpectedVersion <= 0 {
		return Supplier{}, versionConflict()
	}

	input.Code = normalizedCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.LegalName = strings.TrimSpace(input.LegalName)
	input.Notes = strings.TrimSpace(input.Notes)

	if input.Code != "" && !codePattern.MatchString(input.Code) {
		return Supplier{}, validation(
			"code",
			"Use uppercase letters, digits, underscore or hyphen.",
		)
	}
	if input.Name == "" || len(input.Name) > 200 {
		return Supplier{}, validation(
			"name",
			"Provide a supplier name.",
		)
	}
	if err := validateLeadTime(input.DefaultLeadTimeHours); err != nil {
		return Supplier{}, err
	}

	status, err := supplierStatus(input.Status, false)
	if err != nil {
		return Supplier{}, err
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return Supplier{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanSupplier(
		tx.QueryRow(
			ctx,
			`SELECT `+supplierColumns+`
               FROM vendor_suppliers
              WHERE business_id=$1
                AND id=$2
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Supplier{}, notFound("Supplier")
	}
	if err != nil {
		return Supplier{}, err
	}
	if current.Status == StatusArchived {
		return Supplier{}, apperrors.Conflict(
			"Archived suppliers cannot be changed.",
			nil,
		)
	}
	if current.Version != input.ExpectedVersion {
		return Supplier{}, versionConflict()
	}

	categoryID, err := s.validateCategory(
		ctx,
		tx,
		actor.VendorID,
		input.CategoryID,
	)
	if err != nil {
		return Supplier{}, err
	}

	out, err := scanSupplier(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_suppliers
                SET category_id=$3,
                    code=$4,
                    name=$5,
                    legal_name=$6,
                    default_lead_time_hours=$7,
                    status=$8,
                    notes=$9,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND id=$2
              RETURNING `+supplierColumns,
			actor.VendorID,
			current.ID,
			categoryID,
			input.Code,
			input.Name,
			input.LegalName,
			input.DefaultLeadTimeHours,
			status,
			input.Notes,
		),
	)
	if err != nil {
		return Supplier{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier.updated",
		"vendor.purchasing.supplier.updated",
		"vendor_supplier",
		out.ID,
		"",
		out.Version,
		current,
		out,
		map[string]any{
			"supplier_id": out.ID,
			"status":      out.Status,
			"category_id": out.CategoryID,
		},
	); err != nil {
		return Supplier{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Supplier{}, err
	}

	return out, nil
}

func (s *Service) ArchiveSupplier(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	expectedVersion int64,
) (Supplier, error) {
	if err := s.requireWrite(); err != nil {
		return Supplier{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return Supplier{}, err
	}
	if expectedVersion <= 0 {
		return Supplier{}, versionConflict()
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return Supplier{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanSupplier(
		tx.QueryRow(
			ctx,
			`SELECT `+supplierColumns+`
               FROM vendor_suppliers
              WHERE business_id=$1
                AND id=$2
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Supplier{}, notFound("Supplier")
	}
	if err != nil {
		return Supplier{}, err
	}
	if current.Version != expectedVersion {
		return Supplier{}, versionConflict()
	}
	if current.Status == StatusArchived {
		return current, nil
	}

	var activeProducts int
	if err = tx.QueryRow(
		ctx,
		`SELECT count(*)
           FROM vendor_supplier_products
          WHERE business_id=$1
            AND supplier_id=$2
            AND status='ACTIVE'`,
		actor.VendorID,
		current.ID,
	).Scan(&activeProducts); err != nil {
		return Supplier{}, err
	}

	if activeProducts > 0 {
		return Supplier{}, apperrors.Conflict(
			"Archive this supplier's active supplier-product mappings first.",
			nil,
		)
	}

	out, err := scanSupplier(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_suppliers
                SET status='ARCHIVED',
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND id=$2
              RETURNING `+supplierColumns,
			actor.VendorID,
			current.ID,
		),
	)
	if err != nil {
		return Supplier{}, err
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier.archived",
		"vendor.purchasing.supplier.archived",
		"vendor_supplier",
		out.ID,
		"",
		out.Version,
		current,
		out,
		map[string]any{
			"supplier_id": out.ID,
			"status":      out.Status,
		},
	); err != nil {
		return Supplier{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Supplier{}, err
	}

	return out, nil
}

func (s *Service) ensureSupplierReadable(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
) (Supplier, error) {
	return s.GetSupplier(
		ctx,
		actor,
		scope,
		supplierID,
	)
}

// -----------------------------------------------------------------------------
// Contacts
// -----------------------------------------------------------------------------

func (s *Service) ListContacts(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
) ([]SupplierContact, error) {
	supplier, err := s.ensureSupplierReadable(
		ctx,
		actor,
		scope,
		supplierID,
	)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		ctx,
		`SELECT `+contactColumns+`
           FROM vendor_supplier_contacts
          WHERE business_id=$1
            AND supplier_id=$2
          ORDER BY is_primary DESC,name,id`,
		actor.VendorID,
		supplier.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SupplierContact, 0)
	for rows.Next() {
		item, scanErr := scanContact(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) CreateContact(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	input CreateContactInput,
) (SupplierContact, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierContact{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierContact{}, err
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.Phone = strings.TrimSpace(input.Phone)

	if input.Name == "" || len(input.Name) > 160 {
		return SupplierContact{}, validation(
			"name",
			"Provide a supplier contact name.",
		)
	}
	if len(input.Email) > 320 || len(input.Phone) > 80 {
		return SupplierContact{}, validation(
			"email",
			"Supplier contact details are too long.",
		)
	}

	role, err := contactRole(input.ContactRole)
	if err != nil {
		return SupplierContact{}, err
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierContact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var supplierStatusValue string
	err = tx.QueryRow(
		ctx,
		`SELECT status
           FROM vendor_suppliers
          WHERE business_id=$1
            AND id=$2
          FOR UPDATE`,
		actor.VendorID,
		strings.TrimSpace(supplierID),
	).Scan(&supplierStatusValue)

	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierContact{}, notFound("Supplier")
	}
	if err != nil {
		return SupplierContact{}, err
	}
	if supplierStatusValue == StatusArchived {
		return SupplierContact{}, apperrors.Conflict(
			"Archived suppliers cannot receive contacts.",
			nil,
		)
	}

	if input.IsPrimary {
		_, err = tx.Exec(
			ctx,
			`UPDATE vendor_supplier_contacts
                SET is_primary=false,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND supplier_id=$2
                AND status='ACTIVE'
                AND is_primary=true`,
			actor.VendorID,
			supplierID,
		)
		if err != nil {
			return SupplierContact{}, err
		}
	}

	out, err := scanContact(
		tx.QueryRow(
			ctx,
			`INSERT INTO vendor_supplier_contacts (
                id,business_id,supplier_id,contact_role,
                name,email,phone,is_primary
             )
             VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
             RETURNING `+contactColumns,
			"vsct_"+uuid.NewString(),
			actor.VendorID,
			supplierID,
			role,
			input.Name,
			input.Email,
			input.Phone,
			input.IsPrimary,
		),
	)
	if err != nil {
		return SupplierContact{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_contact.created",
		"vendor.purchasing.supplier_contact.created",
		"vendor_supplier_contact",
		out.ID,
		"",
		out.Version,
		nil,
		out,
		map[string]any{
			"supplier_id":  out.SupplierID,
			"contact_role": out.ContactRole,
			"is_primary":   out.IsPrimary,
			"status":       out.Status,
		},
	); err != nil {
		return SupplierContact{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierContact{}, err
	}
	return out, nil
}

func (s *Service) UpdateContact(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	contactID string,
	input UpdateContactInput,
) (SupplierContact, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierContact{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierContact{}, err
	}
	if input.ExpectedVersion <= 0 {
		return SupplierContact{}, versionConflict()
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.Phone = strings.TrimSpace(input.Phone)

	if input.Name == "" || len(input.Name) > 160 {
		return SupplierContact{}, validation(
			"name",
			"Provide a supplier contact name.",
		)
	}

	role, err := contactRole(input.ContactRole)
	if err != nil {
		return SupplierContact{}, err
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierContact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanContact(
		tx.QueryRow(
			ctx,
			`SELECT `+contactColumns+`
               FROM vendor_supplier_contacts
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
			strings.TrimSpace(contactID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierContact{}, notFound("Supplier contact")
	}
	if err != nil {
		return SupplierContact{}, err
	}
	if current.Status == StatusArchived {
		return SupplierContact{}, apperrors.Conflict(
			"Archived supplier contacts cannot be changed.",
			nil,
		)
	}
	if current.Version != input.ExpectedVersion {
		return SupplierContact{}, versionConflict()
	}

	if input.IsPrimary {
		_, err = tx.Exec(
			ctx,
			`UPDATE vendor_supplier_contacts
                SET is_primary=false,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND supplier_id=$2
                AND id<>$3
                AND status='ACTIVE'
                AND is_primary=true`,
			actor.VendorID,
			current.SupplierID,
			current.ID,
		)
		if err != nil {
			return SupplierContact{}, err
		}
	}

	out, err := scanContact(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_supplier_contacts
                SET contact_role=$4,
                    name=$5,
                    email=$6,
                    phone=$7,
                    is_primary=$8,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              RETURNING `+contactColumns,
			actor.VendorID,
			current.SupplierID,
			current.ID,
			role,
			input.Name,
			input.Email,
			input.Phone,
			input.IsPrimary,
		),
	)
	if err != nil {
		return SupplierContact{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_contact.updated",
		"vendor.purchasing.supplier_contact.updated",
		"vendor_supplier_contact",
		out.ID,
		"",
		out.Version,
		current,
		out,
		map[string]any{
			"supplier_id":  out.SupplierID,
			"contact_role": out.ContactRole,
			"is_primary":   out.IsPrimary,
			"status":       out.Status,
		},
	); err != nil {
		return SupplierContact{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierContact{}, err
	}
	return out, nil
}

func (s *Service) ArchiveContact(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	contactID string,
	expectedVersion int64,
) (SupplierContact, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierContact{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierContact{}, err
	}
	if expectedVersion <= 0 {
		return SupplierContact{}, versionConflict()
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierContact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanContact(
		tx.QueryRow(
			ctx,
			`SELECT `+contactColumns+`
               FROM vendor_supplier_contacts
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
			strings.TrimSpace(contactID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierContact{}, notFound("Supplier contact")
	}
	if err != nil {
		return SupplierContact{}, err
	}
	if current.Version != expectedVersion {
		return SupplierContact{}, versionConflict()
	}
	if current.Status == StatusArchived {
		return current, nil
	}

	out, err := scanContact(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_supplier_contacts
                SET status='ARCHIVED',
                    is_primary=false,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              RETURNING `+contactColumns,
			actor.VendorID,
			current.SupplierID,
			current.ID,
		),
	)
	if err != nil {
		return SupplierContact{}, err
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_contact.archived",
		"vendor.purchasing.supplier_contact.archived",
		"vendor_supplier_contact",
		out.ID,
		"",
		out.Version,
		current,
		out,
		map[string]any{
			"supplier_id": out.SupplierID,
			"status":      out.Status,
		},
	); err != nil {
		return SupplierContact{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierContact{}, err
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Terms
// -----------------------------------------------------------------------------

func (s *Service) GetTerms(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
) (SupplierTerms, error) {
	supplier, err := s.ensureSupplierReadable(
		ctx,
		actor,
		scope,
		supplierID,
	)
	if err != nil {
		return SupplierTerms{}, err
	}

	out, err := scanTerms(
		s.db.QueryRow(
			ctx,
			`SELECT `+termsColumns+`
               FROM vendor_supplier_terms
              WHERE business_id=$1
                AND supplier_id=$2`,
			actor.VendorID,
			supplier.ID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierTerms{}, notFound("Supplier terms")
	}
	return out, err
}

func (s *Service) UpsertTerms(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	input UpsertTermsInput,
) (SupplierTerms, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierTerms{}, err
	}
	if err := requireGlobal(scope); err != nil {
		return SupplierTerms{}, err
	}
	if input.PaymentTermsDays < 0 ||
		input.PaymentTermsDays > 3650 {
		return SupplierTerms{}, validation(
			"payment_terms_days",
			"Payment terms must be between 0 and 3,650 days.",
		)
	}

	currency, err := normalizedCurrency(input.CurrencyCode)
	if err != nil {
		return SupplierTerms{}, err
	}

	input.DeliveryTerms = strings.TrimSpace(input.DeliveryTerms)
	if len(input.DeliveryTerms) > 2000 {
		return SupplierTerms{}, validation(
			"delivery_terms",
			"Delivery terms are too long.",
		)
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierTerms{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var supplierState string
	err = tx.QueryRow(
		ctx,
		`SELECT status
           FROM vendor_suppliers
          WHERE business_id=$1
            AND id=$2
          FOR UPDATE`,
		actor.VendorID,
		strings.TrimSpace(supplierID),
	).Scan(&supplierState)

	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierTerms{}, notFound("Supplier")
	}
	if err != nil {
		return SupplierTerms{}, err
	}
	if supplierState == StatusArchived {
		return SupplierTerms{}, apperrors.Conflict(
			"Archived suppliers cannot receive new terms.",
			nil,
		)
	}

	current, currentErr := scanTerms(
		tx.QueryRow(
			ctx,
			`SELECT `+termsColumns+`
               FROM vendor_supplier_terms
              WHERE business_id=$1
                AND supplier_id=$2
              FOR UPDATE`,
			actor.VendorID,
			supplierID,
		),
	)

	var out SupplierTerms
	var before any

	switch {
	case errors.Is(currentErr, pgx.ErrNoRows):
		if input.ExpectedVersion != 0 {
			return SupplierTerms{}, versionConflict()
		}

		out, err = scanTerms(
			tx.QueryRow(
				ctx,
				`INSERT INTO vendor_supplier_terms (
                    supplier_id,
                    business_id,
                    payment_terms_days,
                    currency_code,
                    delivery_terms
                 )
                 VALUES ($1,$2,$3,$4,$5)
                 RETURNING `+termsColumns,
				supplierID,
				actor.VendorID,
				input.PaymentTermsDays,
				currency,
				input.DeliveryTerms,
			),
		)
		before = nil

	case currentErr != nil:
		return SupplierTerms{}, currentErr

	default:
		if input.ExpectedVersion <= 0 ||
			current.Version != input.ExpectedVersion {
			return SupplierTerms{}, versionConflict()
		}

		out, err = scanTerms(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_supplier_terms
                    SET payment_terms_days=$3,
                        currency_code=$4,
                        delivery_terms=$5,
                        version=version+1,
                        updated_at=now()
                  WHERE business_id=$1
                    AND supplier_id=$2
                  RETURNING `+termsColumns,
				actor.VendorID,
				supplierID,
				input.PaymentTermsDays,
				currency,
				input.DeliveryTerms,
			),
		)
		before = current
	}

	if err != nil {
		return SupplierTerms{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_terms.updated",
		"vendor.purchasing.supplier_terms.updated",
		"vendor_supplier_terms",
		supplierID,
		"",
		out.Version,
		before,
		out,
		map[string]any{
			"supplier_id":        supplierID,
			"payment_terms_days": out.PaymentTermsDays,
			"currency_code":      out.CurrencyCode,
		},
	); err != nil {
		return SupplierTerms{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierTerms{}, err
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Supplier products / Inventory linkage
// -----------------------------------------------------------------------------

type inventoryIdentity struct {
	ID              string
	BranchID        string
	MeasureRef      string
	MeasureOwnerKey string
	UnitKind        string
	Status          string
}

func loadActiveInventoryIdentity(
	ctx context.Context,
	q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	businessID string,
	branchID string,
	itemID string,
) (inventoryIdentity, error) {
	var out inventoryIdentity

	err := q.QueryRow(
		ctx,
		`SELECT
            id,
            branch_id,
            stock_measure_ref,
            stock_measure_owner_key,
            stock_unit_kind,
            status
           FROM inventory_items
          WHERE id=$1
            AND business_id=$2
            AND branch_id=$3`,
		strings.TrimSpace(itemID),
		strings.TrimSpace(businessID),
		strings.TrimSpace(branchID),
	).Scan(
		&out.ID,
		&out.BranchID,
		&out.MeasureRef,
		&out.MeasureOwnerKey,
		&out.UnitKind,
		&out.Status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return inventoryIdentity{}, notFound("Inventory item")
	}
	if err != nil {
		return inventoryIdentity{}, err
	}
	if out.Status != StatusActive {
		return inventoryIdentity{}, apperrors.Conflict(
			"Supplier products can only map to an active Inventory item.",
			nil,
		)
	}

	return out, nil
}

func validateSupplierProductInput(
	input CreateSupplierProductInput,
) (CreateSupplierProductInput, error) {
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.InventoryItemID = strings.TrimSpace(input.InventoryItemID)
	input.SupplierSKU = strings.TrimSpace(input.SupplierSKU)
	input.PurchaseUnitCode = normalizedCode(input.PurchaseUnitCode)
	input.PurchaseUnitName = strings.TrimSpace(input.PurchaseUnitName)

	if input.BranchID == "" {
		return input, validation(
			"branch_id",
			"Choose the branch that owns the Inventory item.",
		)
	}
	if input.InventoryItemID == "" {
		return input, validation(
			"inventory_item_id",
			"Choose an Inventory item.",
		)
	}
	if !codePattern.MatchString(input.PurchaseUnitCode) {
		return input, validation(
			"purchase_unit_code",
			"Use an uppercase purchase-unit code.",
		)
	}
	if input.PurchaseUnitName == "" ||
		len(input.PurchaseUnitName) > 160 {
		return input, validation(
			"purchase_unit_name",
			"Provide a purchase-unit name.",
		)
	}
	if len(input.SupplierSKU) > 160 {
		return input, validation(
			"supplier_sku",
			"Supplier SKU is too long.",
		)
	}

	mode, factor, err := NormalizeExactConversion(
		input.ConversionMode,
		input.InventoryUnitsPerPurchaseUnit,
	)
	if err != nil {
		return input, err
	}
	input.ConversionMode = mode
	input.InventoryUnitsPerPurchaseUnit = factor

	if input.MinimumOrderQuantity <= 0 {
		return input, validation(
			"minimum_order_quantity",
			"Minimum order quantity must be positive.",
		)
	}
	if input.PurchaseMultiple <= 0 {
		return input, validation(
			"purchase_multiple",
			"Purchase multiple must be positive.",
		)
	}
	if err = validateCosts(input.TypicalCostMinor); err != nil {
		return input, err
	}
	if err = validateOptionalLeadTime(input.LeadTimeHours); err != nil {
		return input, err
	}

	input.CurrencyCode, err = normalizedCurrency(input.CurrencyCode)
	if err != nil {
		return input, err
	}

	return input, nil
}

func (s *Service) ListSupplierProducts(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	branchFilter string,
	limit int,
	offset int,
) ([]SupplierProduct, error) {
	if s == nil || s.db == nil || !s.enabled ||
		!actor.Validate() || !scope.HasAny() {
		return nil, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	branchFilter = strings.TrimSpace(branchFilter)
	if branchFilter != "" && !scope.Covers(branchFilter) {
		return nil, apperrors.Forbidden(
			"Purchasing access is not permitted for this branch.",
			nil,
		)
	}

	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}

	limit, offset = normalizePage(limit, offset)

	rows, err := s.db.Query(
		ctx,
		`SELECT `+supplierProductColumns+`
           FROM vendor_supplier_products
          WHERE business_id=$1
            AND supplier_id=$2
            AND (
                $3::boolean
                OR branch_id=ANY($4::text[])
            )
            AND (
                $5::text=''
                OR branch_id=$5
            )
          ORDER BY branch_id,purchase_unit_name,id
          LIMIT $6 OFFSET $7`,
		actor.VendorID,
		strings.TrimSpace(supplierID),
		scope.Global,
		branches,
		branchFilter,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SupplierProduct, 0)
	for rows.Next() {
		item, scanErr := scanSupplierProduct(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) GetSupplierProduct(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	productID string,
) (SupplierProduct, error) {
	if s == nil || s.db == nil || !s.enabled ||
		!actor.Validate() || !scope.HasAny() {
		return SupplierProduct{}, apperrors.Forbidden(
			"Purchasing access is not permitted.",
			nil,
		)
	}

	out, err := scanSupplierProduct(
		s.db.QueryRow(
			ctx,
			`SELECT `+supplierProductColumns+`
               FROM vendor_supplier_products
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
			strings.TrimSpace(productID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierProduct{}, notFound("Supplier product")
	}
	if err != nil {
		return SupplierProduct{}, err
	}

	if err = requireBranch(scope, out.BranchID); err != nil {
		return SupplierProduct{}, err
	}

	return out, nil
}

func (s *Service) CreateSupplierProduct(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	input CreateSupplierProductInput,
) (SupplierProduct, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierProduct{}, err
	}

	input, err := validateSupplierProductInput(input)
	if err != nil {
		return SupplierProduct{}, err
	}

	if err = requireBranch(scope, input.BranchID); err != nil {
		return SupplierProduct{}, err
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierProduct{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var supplierState string
	err = tx.QueryRow(
		ctx,
		`SELECT status
           FROM vendor_suppliers
          WHERE business_id=$1
            AND id=$2
          FOR UPDATE`,
		actor.VendorID,
		strings.TrimSpace(supplierID),
	).Scan(&supplierState)

	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierProduct{}, notFound("Supplier")
	}
	if err != nil {
		return SupplierProduct{}, err
	}
	if supplierState != StatusActive {
		return SupplierProduct{}, apperrors.Conflict(
			"Supplier products can only be created for an active supplier.",
			nil,
		)
	}

	inventoryItem, err := loadActiveInventoryIdentity(
		ctx,
		tx,
		actor.VendorID,
		input.BranchID,
		input.InventoryItemID,
	)
	if err != nil {
		return SupplierProduct{}, err
	}

	out, err := scanSupplierProduct(
		tx.QueryRow(
			ctx,
			`INSERT INTO vendor_supplier_products (
                id,
                business_id,
                supplier_id,
                branch_id,
                inventory_item_id,
                supplier_sku,
                purchase_unit_code,
                purchase_unit_name,
                conversion_mode,
                inventory_units_per_purchase_unit,
                minimum_order_quantity,
                purchase_multiple,
                typical_cost_minor,
                currency_code,
                lead_time_hours,
                inventory_measure_ref,
                inventory_measure_owner_key,
                inventory_unit_kind
             )
             VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,$9,
                $10,$11,$12,$13,$14,$15,$16,$17,$18
             )
             RETURNING `+supplierProductColumns,
			"vsp_"+uuid.NewString(),
			actor.VendorID,
			supplierID,
			input.BranchID,
			inventoryItem.ID,
			input.SupplierSKU,
			input.PurchaseUnitCode,
			input.PurchaseUnitName,
			input.ConversionMode,
			input.InventoryUnitsPerPurchaseUnit,
			input.MinimumOrderQuantity,
			input.PurchaseMultiple,
			input.TypicalCostMinor,
			input.CurrencyCode,
			input.LeadTimeHours,
			inventoryItem.MeasureRef,
			inventoryItem.MeasureOwnerKey,
			inventoryItem.UnitKind,
		),
	)
	if err != nil {
		return SupplierProduct{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_product.created",
		"vendor.purchasing.supplier_product.created",
		"vendor_supplier_product",
		out.ID,
		out.BranchID,
		out.Version,
		nil,
		out,
		map[string]any{
			"supplier_id":       out.SupplierID,
			"inventory_item_id": out.InventoryItemID,
			"purchase_unit":     out.PurchaseUnitCode,
			"conversion_mode":   out.ConversionMode,
			"status":            out.Status,
		},
	); err != nil {
		return SupplierProduct{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierProduct{}, err
	}

	return out, nil
}

func validateSupplierProductUpdate(
	input UpdateSupplierProductInput,
) (UpdateSupplierProductInput, error) {
	input.SupplierSKU = strings.TrimSpace(input.SupplierSKU)

	if len(input.SupplierSKU) > 160 {
		return input, validation(
			"supplier_sku",
			"Supplier SKU is too long.",
		)
	}
	if input.MinimumOrderQuantity <= 0 {
		return input, validation(
			"minimum_order_quantity",
			"Minimum order quantity must be positive.",
		)
	}
	if input.PurchaseMultiple <= 0 {
		return input, validation(
			"purchase_multiple",
			"Purchase multiple must be positive.",
		)
	}
	if err := validateCosts(input.TypicalCostMinor); err != nil {
		return input, err
	}
	if err := validateOptionalLeadTime(input.LeadTimeHours); err != nil {
		return input, err
	}

	currency, err := normalizedCurrency(input.CurrencyCode)
	if err != nil {
		return input, err
	}
	input.CurrencyCode = currency

	if input.ExpectedVersion <= 0 {
		return input, versionConflict()
	}

	return input, nil
}

func (s *Service) UpdateSupplierProduct(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	productID string,
	input UpdateSupplierProductInput,
) (SupplierProduct, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierProduct{}, err
	}

	input, err := validateSupplierProductUpdate(input)
	if err != nil {
		return SupplierProduct{}, err
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierProduct{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanSupplierProduct(
		tx.QueryRow(
			ctx,
			`SELECT `+supplierProductColumns+`
               FROM vendor_supplier_products
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
			strings.TrimSpace(productID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierProduct{}, notFound("Supplier product")
	}
	if err != nil {
		return SupplierProduct{}, err
	}
	if err = requireBranch(scope, current.BranchID); err != nil {
		return SupplierProduct{}, err
	}
	if current.Status == StatusArchived {
		return SupplierProduct{}, apperrors.Conflict(
			"Archived supplier products cannot be changed.",
			nil,
		)
	}
	if current.Version != input.ExpectedVersion {
		return SupplierProduct{}, versionConflict()
	}

	// Revalidate the Inventory identity before updating commercial data. A
	// source that has since been archived cannot silently remain procurement-
	// eligible.
	_, err = loadActiveInventoryIdentity(
		ctx,
		tx,
		actor.VendorID,
		current.BranchID,
		current.InventoryItemID,
	)
	if err != nil {
		return SupplierProduct{}, err
	}

	out, err := scanSupplierProduct(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_supplier_products
                SET supplier_sku=$4,
                    minimum_order_quantity=$5,
                    purchase_multiple=$6,
                    typical_cost_minor=$7,
                    currency_code=$8,
                    lead_time_hours=$9,
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              RETURNING `+supplierProductColumns,
			actor.VendorID,
			current.SupplierID,
			current.ID,
			input.SupplierSKU,
			input.MinimumOrderQuantity,
			input.PurchaseMultiple,
			input.TypicalCostMinor,
			input.CurrencyCode,
			input.LeadTimeHours,
		),
	)
	if err != nil {
		return SupplierProduct{}, mapWriteError(err)
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_product.updated",
		"vendor.purchasing.supplier_product.updated",
		"vendor_supplier_product",
		out.ID,
		out.BranchID,
		out.Version,
		current,
		out,
		map[string]any{
			"supplier_id":       out.SupplierID,
			"inventory_item_id": out.InventoryItemID,
			"purchase_unit":     out.PurchaseUnitCode,
			"status":            out.Status,
		},
	); err != nil {
		return SupplierProduct{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierProduct{}, err
	}

	return out, nil
}

func (s *Service) ArchiveSupplierProduct(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	productID string,
	expectedVersion int64,
) (SupplierProduct, error) {
	if err := s.requireWrite(); err != nil {
		return SupplierProduct{}, err
	}
	if expectedVersion <= 0 {
		return SupplierProduct{}, versionConflict()
	}

	tx, err := begin(ctx, s.db)
	if err != nil {
		return SupplierProduct{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanSupplierProduct(
		tx.QueryRow(
			ctx,
			`SELECT `+supplierProductColumns+`
               FROM vendor_supplier_products
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              FOR UPDATE`,
			actor.VendorID,
			strings.TrimSpace(supplierID),
			strings.TrimSpace(productID),
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierProduct{}, notFound("Supplier product")
	}
	if err != nil {
		return SupplierProduct{}, err
	}
	if err = requireBranch(scope, current.BranchID); err != nil {
		return SupplierProduct{}, err
	}
	if current.Version != expectedVersion {
		return SupplierProduct{}, versionConflict()
	}
	if current.Status == StatusArchived {
		return current, nil
	}

	out, err := scanSupplierProduct(
		tx.QueryRow(
			ctx,
			`UPDATE vendor_supplier_products
                SET status='ARCHIVED',
                    version=version+1,
                    updated_at=now()
              WHERE business_id=$1
                AND supplier_id=$2
                AND id=$3
              RETURNING `+supplierProductColumns,
			actor.VendorID,
			current.SupplierID,
			current.ID,
		),
	)
	if err != nil {
		return SupplierProduct{}, err
	}

	if err = s.recordMutation(
		ctx,
		tx,
		actor,
		"purchasing.supplier_product.archived",
		"vendor.purchasing.supplier_product.archived",
		"vendor_supplier_product",
		out.ID,
		out.BranchID,
		out.Version,
		current,
		out,
		map[string]any{
			"supplier_id":       out.SupplierID,
			"inventory_item_id": out.InventoryItemID,
			"status":            out.Status,
		},
	); err != nil {
		return SupplierProduct{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierProduct{}, err
	}
	return out, nil
}

func (s *Service) DebugSummary() string {
	if s == nil {
		return "purchasing:nil"
	}
	return fmt.Sprintf(
		"purchasing:enabled=%t writes=%t",
		s.enabled,
		s.writesEnabled,
	)
}
