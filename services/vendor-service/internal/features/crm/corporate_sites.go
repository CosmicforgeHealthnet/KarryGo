package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	_ "time/tzdata" // embedded IANA database, so location timezone validation works on Windows and slim containers

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Corporate locations and cost centres: the organization's own structure, as
// far as the Vendor's CRM relationship needs to know it.
//
// A corporate location is a site of the CUSTOMER's organization — a head
// office, a campus, a warehouse. It is not a customer's home address, not a
// checkout delivery address and not a logistics mission address. A cost centre
// is a label the customer uses to group things internally; it carries no
// budget, no credit limit and no currency, because a number here would make
// vendor-service a finance authority alongside payment-wallet-service.

// Corporate location types. LocationBranch means a branch of the CUSTOMER's
// organization and has nothing to do with a vendor_branches row — hence the
// deliberately distinct naming in code.
const (
	LocationHeadquarters = "HEADQUARTERS"
	LocationOffice       = "OFFICE"
	LocationBranch       = "BRANCH"
	LocationStore        = "STORE"
	LocationWarehouse    = "WAREHOUSE"
	LocationSite         = "SITE"
	LocationCampus       = "CAMPUS"
	LocationOther        = "OTHER"
)

var corporateLocationTypes = map[string]bool{
	LocationHeadquarters: true, LocationOffice: true, LocationBranch: true, LocationStore: true,
	LocationWarehouse: true, LocationSite: true, LocationCampus: true, LocationOther: true,
}

// CorporateLocation is one site of the customer's organization.
type CorporateLocation struct {
	ID                  string
	BusinessID          string
	CorporateCustomerID string
	Code                string
	NormalizedCode      string
	Name                string
	LocationType        string
	Status              string
	AddressLine1        *string
	AddressLine2        *string
	City                *string
	StateRegion         *string
	PostalCode          *string
	CountryCode         *string
	Timezone            *string
	ExternalLocationRef *string
	CreatedByStaffID    string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
}

// CorporateCostCenter is an organizational grouping label.
type CorporateCostCenter struct {
	ID                  string
	BusinessID          string
	CorporateCustomerID string
	Code                string
	NormalizedCode      string
	Name                string
	Status              string
	LocationID          *string
	ExternalReference   *string
	CreatedByStaffID    string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
}

func errCorporateLocationNotFound() error {
	return crmError(apperrors.NotFound("Corporate location not found.", nil), CodeCorporateLocationMissing)
}
func errCorporateCostCenterNotFound() error {
	return crmError(apperrors.NotFound("Corporate cost centre not found.", nil), CodeCorporateCostCenterGone)
}

// ---------------------------------------------------------------------------
// Locations
// ---------------------------------------------------------------------------

// CorporateLocationInput is a vendor request to define or edit a site.
type CorporateLocationInput struct {
	Code                string
	Name                string
	LocationType        string
	AddressLine1        string
	AddressLine2        string
	City                string
	StateRegion         string
	PostalCode          string
	CountryCode         string
	Timezone            string
	ExternalLocationRef string
	Status              string
	ExpectedVersion     int64
}

type validatedLocation struct {
	Code, NormalizedCode, Name, LocationType, Status string
	AddressLine1, AddressLine2, City, StateRegion    *string
	PostalCode, CountryCode, Timezone, External      *string
}

func validateCorporateLocation(in CorporateLocationInput, requireStatus bool) (validatedLocation, error) {
	var out validatedLocation
	var err error

	if out.Code, err = normalizePlainText(in.Code, MaxCostCenterCodeLength, true, true, "location_code"); err != nil {
		return out, err
	}
	out.NormalizedCode = normalizeCorporateCode(out.Code)
	if out.NormalizedCode == "" {
		return out, corporateValidation("location_code", "A location code needs letters or digits.", "")
	}
	if out.Name, err = normalizePlainText(in.Name, MaxLocationNameLength, true, true, "name"); err != nil {
		return out, err
	}

	out.LocationType = strings.ToUpper(strings.TrimSpace(in.LocationType))
	if out.LocationType == "" {
		out.LocationType = LocationSite
	}
	if !corporateLocationTypes[out.LocationType] {
		return out, corporateValidation("location_type", "Unsupported corporate location type.", CodeCorporateInvalidState)
	}

	out.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if out.Status == "" {
		out.Status = CorporateChildActive
	}
	if !corporateChildStatuses[out.Status] {
		return out, corporateValidation("status", "Unsupported location status.", CodeCorporateInvalidState)
	}
	if requireStatus && out.Status == CorporateChildArchived {
		// Archiving is its own command, so an ordinary edit cannot retire a
		// site as a side effect of changing its name.
		return out, corporateValidation("status", "Archive a location through its archive command.", CodeCorporateInvalidState)
	}

	// Address components are bounded and discrete. There is deliberately no
	// free-form blob: an unbounded address field is where unvalidated text and
	// personal data arrive together.
	for _, field := range []struct {
		value  string
		max    int
		name   string
		target **string
	}{
		{in.AddressLine1, 200, "address_line1", &out.AddressLine1},
		{in.AddressLine2, 200, "address_line2", &out.AddressLine2},
		{in.City, 120, "city", &out.City},
		{in.StateRegion, 120, "state_region", &out.StateRegion},
		{in.PostalCode, 40, "postal_code", &out.PostalCode},
	} {
		trimmed := strings.TrimSpace(field.value)
		if trimmed == "" {
			continue
		}
		normalized, fieldErr := normalizePlainText(trimmed, field.max, true, true, field.name)
		if fieldErr != nil {
			return out, fieldErr
		}
		*field.target = &normalized
	}

	if country := strings.ToUpper(strings.TrimSpace(in.CountryCode)); country != "" {
		if len(country) != 2 {
			return out, corporateValidation("country_code", "Use a two-letter ISO country code.", "")
		}
		for _, r := range country {
			if r < 'A' || r > 'Z' {
				return out, corporateValidation("country_code", "Use a two-letter ISO country code.", "")
			}
		}
		out.CountryCode = &country
	}
	if zone := strings.TrimSpace(in.Timezone); zone != "" {
		// Validated against the embedded IANA database rather than accepted as
		// text: a stored timezone nobody can load is a field that will fail the
		// first time somebody actually needs it.
		if zone == "Local" || len(zone) > 64 {
			return out, corporateValidation("timezone", "Use an IANA timezone, such as Africa/Lagos.", "")
		}
		if _, loadErr := time.LoadLocation(zone); loadErr != nil {
			return out, corporateValidation("timezone", "Use an IANA timezone, such as Africa/Lagos.", "")
		}
		out.Timezone = &zone
	}
	if out.External, err = validateSafeReference(in.ExternalLocationRef, "external_location_ref"); err != nil {
		return out, err
	}
	return out, nil
}

const corporateLocationCols = `id,business_id,corporate_customer_id,location_code,normalized_location_code,name,
location_type,status,address_line1,address_line2,city,state_region,postal_code,country_code,timezone,
external_location_ref,created_by_staff_id,created_at,updated_at,archived_at,version`

func scanCorporateLocation(row rowScanner) (CorporateLocation, error) {
	var l CorporateLocation
	err := row.Scan(&l.ID, &l.BusinessID, &l.CorporateCustomerID, &l.Code, &l.NormalizedCode, &l.Name,
		&l.LocationType, &l.Status, &l.AddressLine1, &l.AddressLine2, &l.City, &l.StateRegion, &l.PostalCode,
		&l.CountryCode, &l.Timezone, &l.ExternalLocationRef, &l.CreatedByStaffID, &l.CreatedAt, &l.UpdatedAt,
		&l.ArchivedAt, &l.Version)
	return l, err
}

// CreateCorporateLocation adds a site to an account.
func (s *Service) CreateCorporateLocation(ctx context.Context, actor Actor, scope Scope, corporateID string, in CorporateLocationInput) (CorporateLocation, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateLocation{}, err
	}
	validated, err := validateCorporateLocation(in, true)
	if err != nil {
		return CorporateLocation{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateLocation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return CorporateLocation{}, err
	}
	location, err := scanCorporateLocation(tx.QueryRow(ctx, `INSERT INTO corporate_customer_locations
		(id,business_id,corporate_customer_id,location_code,normalized_location_code,name,location_type,status,
		 address_line1,address_line2,city,state_region,postal_code,country_code,timezone,external_location_ref,
		 created_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING `+corporateLocationCols,
		newID("crmcloc_"), actor.VendorID, profile.ID, validated.Code, validated.NormalizedCode, validated.Name,
		validated.LocationType, validated.Status, validated.AddressLine1, validated.AddressLine2, validated.City,
		validated.StateRegion, validated.PostalCode, validated.CountryCode, validated.Timezone, validated.External,
		actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateLocation{}, crmError(
				apperrors.Conflict("A location with this code already exists for this corporate customer.", nil),
				CodeCorporateDuplicate)
		}
		return CorporateLocation{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.location.created", "crm_corporate_location", location.ID, "",
		nil, safeLocationAudit(location), ""); err != nil {
		return CorporateLocation{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.corporate.location.created", "crm_corporate_location",
		location.ID, location.Version, map[string]any{
			"corporate_ref": profile.ID, "location_ref": location.ID, "location_type": location.LocationType,
			"status": location.Status}); err != nil {
		return CorporateLocation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateLocation{}, err
	}
	s.metrics.Inc("crm_corporate_location_create_total", "location_type", location.LocationType)
	return location, nil
}

// UpdateCorporateLocation edits a site in place.
func (s *Service) UpdateCorporateLocation(ctx context.Context, actor Actor, scope Scope, corporateID, locationID string, in CorporateLocationInput) (CorporateLocation, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateLocation{}, err
	}
	if in.ExpectedVersion < 1 {
		return CorporateLocation{}, corporateValidation("expected_version", "Expected version is required.",
			CodeCorporateVersionConflict)
	}
	validated, err := validateCorporateLocation(in, true)
	if err != nil {
		return CorporateLocation{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateLocation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateLocation{}, err
	}
	current, err := s.lockLocation(ctx, tx, actor.VendorID, corporateID, locationID)
	if err != nil {
		return CorporateLocation{}, err
	}
	if current.Version != in.ExpectedVersion {
		return CorporateLocation{}, errCorporateVersionConflict()
	}
	if current.Status == CorporateChildArchived {
		return CorporateLocation{}, errCorporateState("An archived location cannot be changed.")
	}

	updated, err := scanCorporateLocation(tx.QueryRow(ctx, `UPDATE corporate_customer_locations
		SET location_code=$1, normalized_location_code=$2, name=$3, location_type=$4, status=$5,
			address_line1=$6, address_line2=$7, city=$8, state_region=$9, postal_code=$10,
			country_code=$11, timezone=$12, external_location_ref=$13, updated_at=now(), version=version+1
		WHERE business_id=$14 AND corporate_customer_id=$15 AND id=$16 AND version=$17 AND status <> 'ARCHIVED'
		RETURNING `+corporateLocationCols,
		validated.Code, validated.NormalizedCode, validated.Name, validated.LocationType, validated.Status,
		validated.AddressLine1, validated.AddressLine2, validated.City, validated.StateRegion, validated.PostalCode,
		validated.CountryCode, validated.Timezone, validated.External,
		actor.VendorID, corporateID, locationID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateLocation{}, errCorporateVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateLocation{}, crmError(
				apperrors.Conflict("A location with this code already exists for this corporate customer.", nil),
				CodeCorporateDuplicate)
		}
		return CorporateLocation{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.location.updated", "crm_corporate_location", locationID, "",
		safeLocationAudit(current), safeLocationAudit(updated), ""); err != nil {
		return CorporateLocation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateLocation{}, err
	}
	return updated, nil
}

// ArchiveCorporateLocation retires a site without destroying it.
//
// Archiving rather than deleting is load-bearing: a cost centre or a historical
// interaction may name this location, and a dangling reference would make that
// history unreadable.
func (s *Service) ArchiveCorporateLocation(ctx context.Context, actor Actor, scope Scope, corporateID, locationID string, expectedVersion int64) (CorporateLocation, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateLocation{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateLocation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateLocation{}, err
	}
	current, err := s.lockLocation(ctx, tx, actor.VendorID, corporateID, locationID)
	if err != nil {
		return CorporateLocation{}, err
	}
	if current.Status == CorporateChildArchived {
		if err = tx.Commit(ctx); err != nil {
			return CorporateLocation{}, err
		}
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return CorporateLocation{}, errCorporateVersionConflict()
	}

	updated, err := scanCorporateLocation(tx.QueryRow(ctx, `UPDATE corporate_customer_locations
		SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 AND version=$4
		RETURNING `+corporateLocationCols, actor.VendorID, corporateID, locationID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateLocation{}, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateLocation{}, err
	}
	// Memberships pointing at a retired site stop being actionable with it.
	// The rows survive as evidence of what the arrangement was.
	if _, err = tx.Exec(ctx, `UPDATE corporate_customer_memberships
		SET status='INACTIVE', updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_location_id=$2 AND status='ACTIVE'`,
		actor.VendorID, locationID); err != nil {
		return CorporateLocation{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.location.archived", "crm_corporate_location", locationID, "",
		safeLocationAudit(current), safeLocationAudit(updated), ""); err != nil {
		return CorporateLocation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateLocation{}, err
	}
	return updated, nil
}

func (s *Service) lockLocation(ctx context.Context, tx pgx.Tx, businessID, corporateID, locationID string) (CorporateLocation, error) {
	location, err := scanCorporateLocation(tx.QueryRow(ctx, `SELECT `+corporateLocationCols+`
		FROM corporate_customer_locations WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 FOR UPDATE`,
		businessID, corporateID, locationID))
	if errors.Is(err, pgx.ErrNoRows) {
		// The corporate id is part of the predicate, so a location belonging to
		// another account under this URL is reported missing rather than found.
		return CorporateLocation{}, errCorporateLocationNotFound()
	}
	return location, err
}

// GetCorporateLocation reads one site through its account.
func (s *Service) GetCorporateLocation(ctx context.Context, actor Actor, scope Scope, corporateID, locationID string) (CorporateLocation, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return CorporateLocation{}, err
	}
	location, err := scanCorporateLocation(s.db.QueryRow(ctx, `SELECT `+corporateLocationCols+`
		FROM corporate_customer_locations WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3`,
		actor.VendorID, corporateID, locationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateLocation{}, errCorporateLocationNotFound()
	}
	return location, err
}

// ListCorporateLocations pages an account's sites.
func (s *Service) ListCorporateLocations(ctx context.Context, actor Actor, scope Scope, corporateID string, status string, limit, offset int) ([]CorporateLocation, bool, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	args := []any{actor.VendorID, corporateID}
	where := "business_id=$1 AND corporate_customer_id=$2"
	if status != "" {
		value := strings.ToUpper(strings.TrimSpace(status))
		if !corporateChildStatuses[value] {
			return nil, false, corporateValidation("status", "Unsupported location status.", CodeCorporateInvalidState)
		}
		args = append(args, value)
		where += " AND status=$3"
	}
	args = append(args, limit+1, offset)
	rows, err := s.db.Query(ctx, `SELECT `+corporateLocationCols+` FROM corporate_customer_locations
		WHERE `+where+` ORDER BY name ASC, id ASC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]CorporateLocation, 0, limit)
	for rows.Next() {
		location, scanErr := scanCorporateLocation(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, location)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func safeLocationAudit(l CorporateLocation) map[string]any {
	// The address is deliberately not audited: it is bounded organizational
	// data the row already holds, and repeating it in every audit entry would
	// scatter it across a second store for no operational benefit.
	return map[string]any{
		"location_ref": l.ID, "corporate_ref": l.CorporateCustomerID, "location_code": l.Code,
		"name": l.Name, "location_type": l.LocationType, "status": l.Status, "version": l.Version,
	}
}

// ---------------------------------------------------------------------------
// Cost centres
// ---------------------------------------------------------------------------

// CorporateCostCenterInput is a vendor request to define or edit a label.
//
// There is deliberately no budget, limit, currency or balance field. A cost
// centre in Section 8G groups things; it does not authorise, cap or fund them.
type CorporateCostCenterInput struct {
	Code              string
	Name              string
	Status            string
	LocationRef       string
	ClearLocation     bool
	ExternalReference string
	ExpectedVersion   int64
}

const corporateCostCenterCols = `id,business_id,corporate_customer_id,code,normalized_code,name,status,
corporate_location_id,external_reference,created_by_staff_id,created_at,updated_at,archived_at,version`

func scanCorporateCostCenter(row rowScanner) (CorporateCostCenter, error) {
	var c CorporateCostCenter
	err := row.Scan(&c.ID, &c.BusinessID, &c.CorporateCustomerID, &c.Code, &c.NormalizedCode, &c.Name,
		&c.Status, &c.LocationID, &c.ExternalReference, &c.CreatedByStaffID, &c.CreatedAt, &c.UpdatedAt,
		&c.ArchivedAt, &c.Version)
	return c, err
}

type validatedCostCenter struct {
	Code, NormalizedCode, Name, Status string
	LocationID, External               *string
}

func validateCostCenter(in CorporateCostCenterInput) (validatedCostCenter, error) {
	var out validatedCostCenter
	var err error
	if out.Code, err = normalizePlainText(in.Code, MaxCostCenterCodeLength, true, true, "code"); err != nil {
		return out, err
	}
	out.NormalizedCode = normalizeCorporateCode(out.Code)
	if out.NormalizedCode == "" {
		return out, corporateValidation("code", "A cost centre code needs letters or digits.", "")
	}
	if out.Name, err = normalizePlainText(in.Name, MaxCostCenterNameLength, true, true, "name"); err != nil {
		return out, err
	}
	out.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if out.Status == "" {
		out.Status = CorporateChildActive
	}
	if !corporateChildStatuses[out.Status] {
		return out, corporateValidation("status", "Unsupported cost centre status.", CodeCorporateInvalidState)
	}
	if out.Status == CorporateChildArchived {
		return out, corporateValidation("status", "Archive a cost centre through its archive command.", CodeCorporateInvalidState)
	}
	if location := strings.TrimSpace(in.LocationRef); location != "" {
		if !safeToken.MatchString(location) {
			return out, corporateValidation("location_ref", "That location reference is not valid.", CodeCorporateNestedScope)
		}
		out.LocationID = &location
	}
	if out.External, err = validateSafeReference(in.ExternalReference, "external_reference"); err != nil {
		return out, err
	}
	return out, nil
}

// CreateCorporateCostCenter adds a grouping label to an account.
func (s *Service) CreateCorporateCostCenter(ctx context.Context, actor Actor, scope Scope, corporateID string, in CorporateCostCenterInput) (CorporateCostCenter, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateCostCenter{}, err
	}
	validated, err := validateCostCenter(in)
	if err != nil {
		return CorporateCostCenter{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateCostCenter{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return CorporateCostCenter{}, err
	}
	if err = s.assertLocationBelongs(ctx, tx, actor.VendorID, corporateID, validated.LocationID); err != nil {
		return CorporateCostCenter{}, err
	}

	center, err := scanCorporateCostCenter(tx.QueryRow(ctx, `INSERT INTO corporate_customer_cost_centers
		(id,business_id,corporate_customer_id,code,normalized_code,name,status,corporate_location_id,
		 external_reference,created_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+corporateCostCenterCols,
		newID("crmccc_"), actor.VendorID, profile.ID, validated.Code, validated.NormalizedCode, validated.Name,
		validated.Status, validated.LocationID, validated.External, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateCostCenter{}, crmError(
				apperrors.Conflict("A cost centre with this code already exists for this corporate customer.", nil),
				CodeCorporateDuplicate)
		}
		return CorporateCostCenter{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.cost_center.created", "crm_corporate_cost_center",
		center.ID, "", nil, safeCostCenterAudit(center), ""); err != nil {
		return CorporateCostCenter{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.corporate.cost_center.created", "crm_corporate_cost_center",
		center.ID, center.Version, map[string]any{
			"corporate_ref": profile.ID, "cost_center_ref": center.ID, "status": center.Status}); err != nil {
		return CorporateCostCenter{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateCostCenter{}, err
	}
	s.metrics.Inc("crm_corporate_cost_center_create_total")
	return center, nil
}

// UpdateCorporateCostCenter edits a label in place.
func (s *Service) UpdateCorporateCostCenter(ctx context.Context, actor Actor, scope Scope, corporateID, costCenterID string, in CorporateCostCenterInput) (CorporateCostCenter, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateCostCenter{}, err
	}
	if in.ExpectedVersion < 1 {
		return CorporateCostCenter{}, corporateValidation("expected_version", "Expected version is required.",
			CodeCorporateVersionConflict)
	}
	validated, err := validateCostCenter(in)
	if err != nil {
		return CorporateCostCenter{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateCostCenter{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateCostCenter{}, err
	}
	current, err := s.lockCostCenter(ctx, tx, actor.VendorID, corporateID, costCenterID)
	if err != nil {
		return CorporateCostCenter{}, err
	}
	if current.Version != in.ExpectedVersion {
		return CorporateCostCenter{}, errCorporateVersionConflict()
	}
	if current.Status == CorporateChildArchived {
		return CorporateCostCenter{}, errCorporateState("An archived cost centre cannot be changed.")
	}
	location := validated.LocationID
	if in.ClearLocation {
		location = nil
	} else if location == nil {
		location = current.LocationID
	}
	if err = s.assertLocationBelongs(ctx, tx, actor.VendorID, corporateID, location); err != nil {
		return CorporateCostCenter{}, err
	}

	updated, err := scanCorporateCostCenter(tx.QueryRow(ctx, `UPDATE corporate_customer_cost_centers
		SET code=$1, normalized_code=$2, name=$3, status=$4, corporate_location_id=$5, external_reference=$6,
			updated_at=now(), version=version+1
		WHERE business_id=$7 AND corporate_customer_id=$8 AND id=$9 AND version=$10 AND status <> 'ARCHIVED'
		RETURNING `+corporateCostCenterCols,
		validated.Code, validated.NormalizedCode, validated.Name, validated.Status, location, validated.External,
		actor.VendorID, corporateID, costCenterID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateCostCenter{}, errCorporateVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateCostCenter{}, crmError(
				apperrors.Conflict("A cost centre with this code already exists for this corporate customer.", nil),
				CodeCorporateDuplicate)
		}
		return CorporateCostCenter{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.cost_center.updated", "crm_corporate_cost_center",
		costCenterID, "", safeCostCenterAudit(current), safeCostCenterAudit(updated), ""); err != nil {
		return CorporateCostCenter{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateCostCenter{}, err
	}
	return updated, nil
}

// ArchiveCorporateCostCenter retires a label without destroying it.
func (s *Service) ArchiveCorporateCostCenter(ctx context.Context, actor Actor, scope Scope, corporateID, costCenterID string, expectedVersion int64) (CorporateCostCenter, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateCostCenter{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateCostCenter{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateCostCenter{}, err
	}
	current, err := s.lockCostCenter(ctx, tx, actor.VendorID, corporateID, costCenterID)
	if err != nil {
		return CorporateCostCenter{}, err
	}
	if current.Status == CorporateChildArchived {
		if err = tx.Commit(ctx); err != nil {
			return CorporateCostCenter{}, err
		}
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return CorporateCostCenter{}, errCorporateVersionConflict()
	}
	updated, err := scanCorporateCostCenter(tx.QueryRow(ctx, `UPDATE corporate_customer_cost_centers
		SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 AND version=$4
		RETURNING `+corporateCostCenterCols, actor.VendorID, corporateID, costCenterID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateCostCenter{}, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateCostCenter{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE corporate_customer_memberships
		SET status='INACTIVE', updated_at=now(), version=version+1
		WHERE business_id=$1 AND cost_center_id=$2 AND status='ACTIVE'`, actor.VendorID, costCenterID); err != nil {
		return CorporateCostCenter{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.cost_center.archived", "crm_corporate_cost_center",
		costCenterID, "", safeCostCenterAudit(current), safeCostCenterAudit(updated), ""); err != nil {
		return CorporateCostCenter{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateCostCenter{}, err
	}
	return updated, nil
}

func (s *Service) lockCostCenter(ctx context.Context, tx pgx.Tx, businessID, corporateID, costCenterID string) (CorporateCostCenter, error) {
	center, err := scanCorporateCostCenter(tx.QueryRow(ctx, `SELECT `+corporateCostCenterCols+`
		FROM corporate_customer_cost_centers WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 FOR UPDATE`,
		businessID, corporateID, costCenterID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateCostCenter{}, errCorporateCostCenterNotFound()
	}
	return center, err
}

// GetCorporateCostCenter reads one label through its account.
func (s *Service) GetCorporateCostCenter(ctx context.Context, actor Actor, scope Scope, corporateID, costCenterID string) (CorporateCostCenter, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return CorporateCostCenter{}, err
	}
	center, err := scanCorporateCostCenter(s.db.QueryRow(ctx, `SELECT `+corporateCostCenterCols+`
		FROM corporate_customer_cost_centers WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3`,
		actor.VendorID, corporateID, costCenterID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateCostCenter{}, errCorporateCostCenterNotFound()
	}
	return center, err
}

// ListCorporateCostCenters pages an account's labels.
func (s *Service) ListCorporateCostCenters(ctx context.Context, actor Actor, scope Scope, corporateID, status string, limit, offset int) ([]CorporateCostCenter, bool, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	args := []any{actor.VendorID, corporateID}
	where := "business_id=$1 AND corporate_customer_id=$2"
	if status != "" {
		value := strings.ToUpper(strings.TrimSpace(status))
		if !corporateChildStatuses[value] {
			return nil, false, corporateValidation("status", "Unsupported cost centre status.", CodeCorporateInvalidState)
		}
		args = append(args, value)
		where += " AND status=$3"
	}
	args = append(args, limit+1, offset)
	rows, err := s.db.Query(ctx, `SELECT `+corporateCostCenterCols+` FROM corporate_customer_cost_centers
		WHERE `+where+` ORDER BY normalized_code ASC, id ASC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]CorporateCostCenter, 0, limit)
	for rows.Next() {
		center, scanErr := scanCorporateCostCenter(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, center)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

// assertLocationBelongs proves a referenced site is part of this account.
//
// The composite foreign key already makes a cross-account link unstorable; this
// check exists so the caller gets a clear, stable error instead of a constraint
// violation, and so an archived site cannot be attached to something new.
func (s *Service) assertLocationBelongs(ctx context.Context, q querier, businessID, corporateID string, locationID *string) error {
	if locationID == nil {
		return nil
	}
	var status string
	err := q.QueryRow(ctx, `SELECT status FROM corporate_customer_locations
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3`, businessID, corporateID, *locationID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errCorporateLocationNotFound()
	}
	if err != nil {
		return err
	}
	if status == CorporateChildArchived {
		return crmError(apperrors.Conflict("That location is archived.", nil), CodeCorporateNestedScope)
	}
	return nil
}

func safeCostCenterAudit(c CorporateCostCenter) map[string]any {
	return map[string]any{
		"cost_center_ref": c.ID, "corporate_ref": c.CorporateCustomerID, "code": c.Code, "name": c.Name,
		"status": c.Status, "location_ref": c.LocationID, "version": c.Version,
	}
}
