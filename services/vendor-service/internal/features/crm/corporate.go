package crm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8G is corporate customer CRM: a Vendor business's commercial
// relationship with an organization — a company, a school, a hotel, an NGO.
//
// What it is not is worth stating as plainly as what it is. It is not a second
// KYB service (verification-compliance-service remains the verification
// authority), not a billing account (payment-wallet-service remains the
// monetary authority), not an order surface, and not a copy of customer
// identity (customer-service remains the identity authority). Every one of
// those boundaries shows up as a column that deliberately does not exist.

// Corporate lifecycle.
//
// ARCHIVED is terminal. An account that could come back would come back with
// relationships and tasks that were retired for a reason, so returning it to
// service is a new decision somebody has to make explicitly rather than an undo.
const (
	CorporateProspect = "PROSPECT"
	CorporateActive   = "ACTIVE"
	CorporateOnHold   = "ON_HOLD"
	CorporateInactive = "INACTIVE"
	CorporateArchived = "ARCHIVED"
)

var corporateStatuses = map[string]bool{
	CorporateProspect: true, CorporateActive: true, CorporateOnHold: true,
	CorporateInactive: true, CorporateArchived: true,
}

// corporateTransitions is the whole state machine, stated once.
//
// It is a table rather than a chain of ifs because the interesting property is
// what is absent: nothing returns from ARCHIVED, and nothing jumps straight
// from PROSPECT to ON_HOLD or INACTIVE — a relationship cannot be paused before
// it ever started.
var corporateTransitions = map[string]map[string]bool{
	CorporateProspect: {CorporateActive: true, CorporateArchived: true},
	CorporateActive:   {CorporateOnHold: true, CorporateInactive: true, CorporateArchived: true},
	CorporateOnHold:   {CorporateActive: true, CorporateInactive: true, CorporateArchived: true},
	CorporateInactive: {CorporateActive: true, CorporateArchived: true},
	CorporateArchived: {},
}

// Organization types. CRM classification only: this field says nothing about
// legal entity status, registration or verification, and nothing in the service
// may infer any of those from it.
const (
	OrgCompany     = "COMPANY"
	OrgInstitution = "INSTITUTION"
	OrgSchool      = "SCHOOL"
	OrgHospital    = "HOSPITAL"
	OrgHotel       = "HOTEL"
	OrgRestaurant  = "RESTAURANT"
	OrgRetailer    = "RETAILER"
	OrgNGO         = "NGO"
	OrgAssociation = "ASSOCIATION"
	OrgGovernment  = "GOVERNMENT"
	OrgOther       = "OTHER"
)

var organizationTypes = map[string]bool{
	OrgCompany: true, OrgInstitution: true, OrgSchool: true, OrgHospital: true,
	OrgHotel: true, OrgRestaurant: true, OrgRetailer: true, OrgNGO: true,
	OrgAssociation: true, OrgGovernment: true, OrgOther: true,
}

// Vendor-side reach over a corporate account. Deliberately distinct from the
// corporate customer's own physical locations, which are a different concept
// with a different table.
const (
	CorporateScopeBusiness  = "BUSINESS"
	CorporateScopeBranchSet = "BRANCH_SET"
)

// Shared lifecycle for the sub-resources. Archive rather than delete: a
// location or cost centre named by historical CRM activity must stay
// resolvable, or that history stops being explicable.
const (
	CorporateChildActive   = "ACTIVE"
	CorporateChildInactive = "INACTIVE"
	CorporateChildArchived = "ARCHIVED"
)

var corporateChildStatuses = map[string]bool{
	CorporateChildActive: true, CorporateChildInactive: true, CorporateChildArchived: true,
}

// Stable Section 8G error codes.
const (
	CodeCorporateNotFound        = "CRM_CORPORATE_CUSTOMER_NOT_FOUND"
	CodeCorporateScopeDenied     = "CRM_CORPORATE_CUSTOMER_SCOPE_DENIED"
	CodeCorporateInvalidState    = "CRM_CORPORATE_CUSTOMER_INVALID_STATE"
	CodeCorporateVersionConflict = "CRM_CORPORATE_CUSTOMER_VERSION_CONFLICT"
	CodeCorporateArchived        = "CRM_CORPORATE_CUSTOMER_ARCHIVED"
	CodeCorporateDuplicate       = "CRM_CORPORATE_CUSTOMER_DUPLICATE"
	CodeCorporateContactNotFound = "CRM_CORPORATE_CONTACT_NOT_FOUND"
	CodeCorporateContactProfile  = "CRM_CORPORATE_CONTACT_INVALID_PROFILE"
	CodeCorporateContactRedacted = "CRM_CORPORATE_CONTACT_REDACTED"
	CodeCorporateContactMerged   = "CRM_CORPORATE_CONTACT_MERGED"
	CodeCorporateContactDup      = "CRM_CORPORATE_CONTACT_DUPLICATE"
	CodeCorporateLocationMissing = "CRM_CORPORATE_LOCATION_NOT_FOUND"
	CodeCorporateCostCenterGone  = "CRM_CORPORATE_COST_CENTER_NOT_FOUND"
	CodeCorporateMembershipGone  = "CRM_CORPORATE_MEMBERSHIP_NOT_FOUND"
	CodeCorporateNestedScope     = "CRM_CORPORATE_NESTED_SCOPE_INVALID"
	CodeCorporateManagerInvalid  = "CRM_CORPORATE_ACCOUNT_MANAGER_INVALID"
	CodeCorporateDisabled        = "CRM_CORPORATE_DISABLED"
)

// Bounds. They are named constants rather than scattered literals because
// these are the numbers an operator or a frontend needs to agree with.
const (
	MaxOrganizationNameLength = 250
	MaxCorporateBranchScopes  = 50
	MaxLocationNameLength     = 200
	MaxCostCenterCodeLength   = 100
	MaxCostCenterNameLength   = 200
	MaxCorporateRefLength     = 200
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// CorporateProfile is a Vendor's relationship with one organization.
type CorporateProfile struct {
	ID                     string
	BusinessID             string
	Code                   string
	OrganizationName       string
	NormalizedName         string
	DisplayName            *string
	OrganizationType       string
	Status                 string
	ScopeMode              string
	AccountManagerStaffID  *string
	RelationshipStartedAt  *time.Time
	ExternalOrganizationID *string
	CreatedByStaffID       string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	ArchivedAt             *time.Time
	Version                int64
}

// Actionable reports whether ordinary corporate CRM writes are permitted.
// An archived account keeps every byte of its history and accepts no new work.
func (c CorporateProfile) Actionable() bool { return c.Status != CorporateArchived }

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

func errCorporateNotFound() error {
	// Indistinguishable from "exists but you may not see it", so probing ids
	// tells a caller nothing about accounts outside their reach.
	return crmError(apperrors.NotFound("Corporate customer not found.", nil), CodeCorporateNotFound)
}
func errCorporateArchived() error {
	return crmError(apperrors.Conflict("This corporate customer is archived and cannot be changed.", nil), CodeCorporateArchived)
}
func errCorporateVersionConflict() error {
	return crmError(apperrors.Conflict("This corporate customer changed since you loaded it. Reload and try again.", nil),
		CodeCorporateVersionConflict)
}
func errCorporateState(message string) error {
	return crmError(apperrors.Conflict(message, nil), CodeCorporateInvalidState)
}
func errCorporateDisabled() error {
	return crmError(apperrors.Unavailable("Corporate customer management is temporarily unavailable.", nil), CodeCorporateDisabled)
}
func corporateValidation(field, message, code string) error {
	err := apperrors.Validation(message, []apperrors.FieldViolation{{Field: field, Message: message}})
	if code != "" {
		err.Details = map[string]any{"code": code}
	}
	return err
}

func (s *Service) assertCorporateAvailable() error {
	if !s.flags.CRMEnabled || !s.flags.CorporateEnabled || s.flags.CommerceKillSwitch {
		return errCorporateDisabled()
	}
	return nil
}

func (s *Service) assertCorporateWrites() error {
	if err := s.assertCorporateAvailable(); err != nil {
		return err
	}
	if !s.flags.WritesEnabled {
		return errWritesDisabled()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// normalizeCorporateName produces the search key. Casing and internal spacing
// are collapsed so a human searching "acme  LTD" finds "Acme Ltd"; the display
// name keeps exactly what was typed.
func normalizeCorporateName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

// normalizeCorporateCode collapses a client-supplied code to its comparable
// form, so "cc 100", "CC-100" and "cc-100" are not three cost centres.
func normalizeCorporateCode(code string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func validateOrganizationName(name string) (display, normalized string, err error) {
	display, err = normalizePlainText(name, MaxOrganizationNameLength, true, true, "organization_name")
	if err != nil {
		return "", "", err
	}
	if len([]rune(display)) < 2 {
		return "", "", corporateValidation("organization_name", "An organization name must be at least 2 characters.", "")
	}
	normalized = normalizeCorporateName(display)
	if normalized == "" {
		return "", "", corporateValidation("organization_name", "An organization name must contain visible characters.", "")
	}
	return display, normalized, nil
}

// validateSafeReference bounds the opaque external references a vendor may
// attach. They are matched exactly and never parsed, so the only requirement is
// that they are short, printable and free of the control characters that make
// logs and exports unsafe.
func validateSafeReference(value, field string) (*string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	normalized, err := normalizePlainText(trimmed, MaxCorporateRefLength, true, true, field)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

// CorporateProfileInput is a vendor request to define or edit an account.
//
// There is deliberately no corporate_customer_code field: the code is allocated
// server-side and is immutable, so a client cannot claim one, guess at another
// tenant's numbering, or renumber an account whose code appears on paperwork.
type CorporateProfileInput struct {
	OrganizationName       string
	DisplayName            string
	OrganizationType       string
	ScopeMode              string
	BranchRefs             []string
	AccountManagerStaffRef string
	ClearAccountManager    bool
	RelationshipStartedAt  *time.Time
	ExternalOrganizationID string
}

type validatedCorporateProfile struct {
	OrganizationName       string
	NormalizedName         string
	DisplayName            *string
	OrganizationType       string
	ScopeMode              string
	BranchRefs             []string
	AccountManagerStaffRef *string
	RelationshipStartedAt  *time.Time
	ExternalOrganizationID *string
}

func validateCorporateProfileInput(in CorporateProfileInput, now time.Time) (validatedCorporateProfile, error) {
	var out validatedCorporateProfile
	var err error

	if out.OrganizationName, out.NormalizedName, err = validateOrganizationName(in.OrganizationName); err != nil {
		return out, err
	}
	if trimmed := strings.TrimSpace(in.DisplayName); trimmed != "" {
		display, displayErr := normalizePlainText(trimmed, MaxOrganizationNameLength, true, true, "display_name")
		if displayErr != nil {
			return out, displayErr
		}
		out.DisplayName = &display
	}

	out.OrganizationType = strings.ToUpper(strings.TrimSpace(in.OrganizationType))
	if out.OrganizationType == "" {
		out.OrganizationType = OrgCompany
	}
	if !organizationTypes[out.OrganizationType] {
		return out, corporateValidation("organization_type", "Unsupported organization type.", CodeCorporateInvalidState)
	}

	out.ScopeMode = strings.ToUpper(strings.TrimSpace(in.ScopeMode))
	if out.ScopeMode == "" {
		out.ScopeMode = CorporateScopeBusiness
	}
	if out.ScopeMode != CorporateScopeBusiness && out.ScopeMode != CorporateScopeBranchSet {
		return out, corporateValidation("scope_mode", "Scope must be BUSINESS or BRANCH_SET.", CodeCorporateScopeDenied)
	}
	if out.BranchRefs, err = normalizeBranchRefs(in.BranchRefs); err != nil {
		return out, err
	}
	if out.ScopeMode == CorporateScopeBranchSet && len(out.BranchRefs) == 0 {
		return out, corporateValidation("branch_refs",
			"A branch-scoped corporate customer needs at least one branch.", CodeCorporateScopeDenied)
	}
	if out.ScopeMode == CorporateScopeBusiness && len(out.BranchRefs) > 0 {
		// Silently dropping them would leave the caller believing they had
		// restricted an account they had in fact opened to the whole business.
		return out, corporateValidation("branch_refs",
			"A business-scoped corporate customer cannot name branches.", CodeCorporateScopeDenied)
	}

	if manager := strings.TrimSpace(in.AccountManagerStaffRef); manager != "" {
		if !safeToken.MatchString(manager) {
			return out, corporateValidation("account_manager_staff_ref", "That staff reference is not valid.",
				CodeCorporateManagerInvalid)
		}
		out.AccountManagerStaffRef = &manager
	}

	if in.RelationshipStartedAt != nil {
		started := in.RelationshipStartedAt.UTC()
		if started.After(now.Add(24 * time.Hour)) {
			return out, corporateValidation("relationship_started_at",
				"A relationship cannot start in the future.", "")
		}
		out.RelationshipStartedAt = &started
	}
	if out.ExternalOrganizationID, err = validateSafeReference(in.ExternalOrganizationID, "external_organization_ref"); err != nil {
		return out, err
	}
	return out, nil
}

func normalizeBranchRefs(refs []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		if !safeToken.MatchString(trimmed) {
			return nil, corporateValidation("branch_refs", "One or more branch references are not valid.",
				CodeCorporateScopeDenied)
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	if len(out) > MaxCorporateBranchScopes {
		return nil, corporateValidation("branch_refs",
			fmt.Sprintf("Name at most %d branches.", MaxCorporateBranchScopes), CodeCorporateScopeDenied)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

const corporateCols = `id,business_id,corporate_customer_code,organization_name,normalized_name,display_name,
organization_type,status,scope_mode,account_manager_staff_id,relationship_started_at,external_organization_ref,
created_by_staff_id,created_at,updated_at,archived_at,version`

func scanCorporate(row rowScanner) (CorporateProfile, error) {
	var c CorporateProfile
	err := row.Scan(&c.ID, &c.BusinessID, &c.Code, &c.OrganizationName, &c.NormalizedName, &c.DisplayName,
		&c.OrganizationType, &c.Status, &c.ScopeMode, &c.AccountManagerStaffID, &c.RelationshipStartedAt,
		&c.ExternalOrganizationID, &c.CreatedByStaffID, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version)
	return c, err
}

// corporateVisibilitySQL restricts a query to the accounts an actor may see.
//
// A branch-scoped actor sees BRANCH_SET accounts covering one of their
// branches, and never a BUSINESS-scope account: an account opened to the whole
// business describes commercial reach they have no visibility of. The filter is
// applied inside the query rather than in Go so counts and pagination are
// computed over the permitted set only — filtering afterwards would still have
// counted the rows it discarded, and a count is itself the disclosure.
func corporateVisibilitySQL(alias string, scope Scope, args *[]any) string {
	if scope.Global {
		return "TRUE"
	}
	*args = append(*args, scope.BranchIDs)
	placeholder := "$" + itoa(len(*args))
	return alias + `.scope_mode = 'BRANCH_SET' AND EXISTS (
		SELECT 1 FROM corporate_customer_branch_scopes bs
		WHERE bs.business_id = ` + alias + `.business_id AND bs.corporate_customer_id = ` + alias + `.id
		  AND bs.branch_id = ANY(` + placeholder + `))`
}

// CorporateListOptions are the safe filters over the corporate list. Sorting is
// a closed registry; there is no client-chosen sort column anywhere.
type CorporateListOptions struct {
	Limit, Offset            int
	Status, OrganizationType string
	BranchRef, ManagerRef    string
	Search                   string
	CreatedFrom, CreatedTo   *time.Time
	Sort, Direction          string
}

var corporateSorts = map[string]string{
	"updated_at":              "c.updated_at",
	"created_at":              "c.created_at",
	"organization_name":       "c.normalized_name",
	"relationship_started_at": "c.relationship_started_at",
}

func (o CorporateListOptions) bounded() CorporateListOptions {
	if o.Limit <= 0 {
		o.Limit = DefaultPageSize
	}
	if o.Limit > MaxPageSize {
		o.Limit = MaxPageSize
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	if _, ok := corporateSorts[o.Sort]; !ok {
		o.Sort = "updated_at"
	}
	if strings.EqualFold(o.Direction, "asc") {
		o.Direction = "ASC"
	} else {
		o.Direction = "DESC"
	}
	return o
}

// ListCorporateCustomers pages the accounts visible to the actor.
func (s *Service) ListCorporateCustomers(ctx context.Context, actor Actor, scope Scope, opts CorporateListOptions) ([]CorporateProfile, bool, error) {
	if err := s.assertCorporateAvailable(); err != nil {
		return nil, false, err
	}
	opts = opts.bounded()
	args := []any{actor.VendorID}
	where := []string{"c.business_id = $1", corporateVisibilitySQL("c", scope, &args)}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}

	if opts.Status != "" {
		status := strings.ToUpper(strings.TrimSpace(opts.Status))
		if !corporateStatuses[status] {
			return nil, false, corporateValidation("status", "Unsupported corporate status.", CodeCorporateInvalidState)
		}
		where = append(where, "c.status = "+bind(status))
	}
	if opts.OrganizationType != "" {
		orgType := strings.ToUpper(strings.TrimSpace(opts.OrganizationType))
		if !organizationTypes[orgType] {
			return nil, false, corporateValidation("organization_type", "Unsupported organization type.", CodeCorporateInvalidState)
		}
		where = append(where, "c.organization_type = "+bind(orgType))
	}
	if opts.BranchRef != "" {
		// Asking about a branch the caller cannot see is refused rather than
		// answered, so the filter cannot be used as an existence oracle.
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, false, errCorporateNotFound()
		}
		where = append(where, `EXISTS (SELECT 1 FROM corporate_customer_branch_scopes bf
			WHERE bf.business_id = c.business_id AND bf.corporate_customer_id = c.id
			  AND bf.branch_id = `+bind(opts.BranchRef)+`)`)
	}
	if opts.ManagerRef != "" {
		where = append(where, "c.account_manager_staff_id = "+bind(opts.ManagerRef))
	}
	if opts.CreatedFrom != nil {
		where = append(where, "c.created_at >= "+bind(opts.CreatedFrom.UTC()))
	}
	if opts.CreatedTo != nil {
		where = append(where, "c.created_at <= "+bind(opts.CreatedTo.UTC()))
	}
	if opts.Search != "" {
		if len([]rune(opts.Search)) > 120 {
			return nil, false, corporateValidation("search", "Search text is too long.", "")
		}
		// Organization name and the vendor-local code only. There is
		// deliberately no contact search: finding an organization by the phone
		// number of somebody who works there would be exactly the individual
		// PII lookup this section refuses to hold data for.
		pattern := bind("%" + escapeLike(normalizeCorporateName(opts.Search)) + "%")
		code := bind("%" + escapeLike(strings.ToUpper(strings.TrimSpace(opts.Search))) + "%")
		where = append(where, "(c.normalized_name LIKE "+pattern+" ESCAPE '\\' OR c.corporate_customer_code LIKE "+code+" ESCAPE '\\')")
	}

	limit := bind(opts.Limit + 1)
	offset := bind(opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+corporateCols+` FROM corporate_customer_profiles c
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY `+corporateSorts[opts.Sort]+` `+opts.Direction+` NULLS LAST, c.id ASC
		LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]CorporateProfile, 0, opts.Limit)
	for rows.Next() {
		profile, scanErr := scanCorporate(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, profile)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > opts.Limit
	if hasMore {
		items = items[:opts.Limit]
	}
	return items, hasMore, nil
}

// GetCorporateCustomer loads one account, enforcing tenancy and branch reach.
func (s *Service) GetCorporateCustomer(ctx context.Context, actor Actor, scope Scope, corporateID string) (CorporateProfile, error) {
	if err := s.assertCorporateAvailable(); err != nil {
		return CorporateProfile{}, err
	}
	profile, err := s.loadCorporate(ctx, s.db, actor.VendorID, corporateID)
	if err != nil {
		return CorporateProfile{}, err
	}
	if err = s.assertCorporateVisible(ctx, s.db, scope, profile); err != nil {
		return CorporateProfile{}, err
	}
	return profile, nil
}

func (s *Service) loadCorporate(ctx context.Context, q querier, businessID, corporateID string) (CorporateProfile, error) {
	profile, err := scanCorporate(q.QueryRow(ctx,
		`SELECT `+corporateCols+` FROM corporate_customer_profiles WHERE business_id=$1 AND id=$2`,
		businessID, corporateID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateProfile{}, errCorporateNotFound()
	}
	return profile, err
}

// loadCorporateForUpdate is the corporate fence. Every corporate mutation takes
// this row lock first, so concurrent writes to one account — including archive
// racing a task create — serialize on the account itself rather than on
// whichever child table each happened to touch.
func (s *Service) loadCorporateForUpdate(ctx context.Context, tx pgx.Tx, businessID, corporateID string) (CorporateProfile, error) {
	profile, err := scanCorporate(tx.QueryRow(ctx,
		`/* crm_corporate_fence */ SELECT `+corporateCols+` FROM corporate_customer_profiles
		 WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessID, corporateID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateProfile{}, errCorporateNotFound()
	}
	return profile, err
}

func (s *Service) assertCorporateVisible(ctx context.Context, q querier, scope Scope, profile CorporateProfile) error {
	if scope.Global {
		return nil
	}
	if profile.ScopeMode != CorporateScopeBranchSet {
		return errCorporateNotFound()
	}
	var visible bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM corporate_customer_branch_scopes
		WHERE business_id=$1 AND corporate_customer_id=$2 AND branch_id = ANY($3))`,
		profile.BusinessID, profile.ID, scope.BranchIDs).Scan(&visible); err != nil {
		return err
	}
	if !visible {
		return errCorporateNotFound()
	}
	return nil
}

// corporateForWrite resolves an account for a mutation: locked, visible, and
// actionable. It is the single gate every corporate command passes through, so
// "archived accounts accept no new work" is enforced in one place rather than
// remembered in twenty.
func (s *Service) corporateForWrite(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, corporateID string) (CorporateProfile, error) {
	profile, err := s.loadCorporateForUpdate(ctx, tx, actor.VendorID, corporateID)
	if err != nil {
		return CorporateProfile{}, err
	}
	if err = s.assertCorporateVisible(ctx, tx, scope, profile); err != nil {
		return CorporateProfile{}, err
	}
	if !profile.Actionable() {
		return CorporateProfile{}, errCorporateArchived()
	}
	return profile, nil
}

// BranchScopesFor lists the Vendor branches an account is limited to.
func (s *Service) BranchScopesFor(ctx context.Context, q querier, businessID, corporateID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT branch_id FROM corporate_customer_branch_scopes
		WHERE business_id=$1 AND corporate_customer_id=$2 ORDER BY branch_id`, businessID, corporateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 4)
	for rows.Next() {
		var branchID string
		if err = rows.Scan(&branchID); err != nil {
			return nil, err
		}
		out = append(out, branchID)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// CreateCorporateCustomer opens a corporate relationship in PROSPECT.
func (s *Service) CreateCorporateCustomer(ctx context.Context, actor Actor, scope Scope, in CorporateProfileInput) (CorporateProfile, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateProfile{}, err
	}
	validated, err := validateCorporateProfileInput(in, s.now())
	if err != nil {
		return CorporateProfile{}, err
	}
	// A branch actor can only open an account their own branches will work; a
	// business-wide account is a business-wide grant's decision.
	if err = s.authorizeCorporateScopeChange(scope, validated.ScopeMode, validated.BranchRefs); err != nil {
		return CorporateProfile{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err = s.assertBranchesOwned(ctx, tx, actor.VendorID, validated.BranchRefs); err != nil {
		return CorporateProfile{}, err
	}
	if err = s.assertAccountManager(ctx, tx, actor.VendorID, validated.AccountManagerStaffRef); err != nil {
		return CorporateProfile{}, err
	}

	code, err := s.allocateCorporateCode(ctx, tx, actor.VendorID)
	if err != nil {
		return CorporateProfile{}, err
	}

	profile, err := scanCorporate(tx.QueryRow(ctx, `INSERT INTO corporate_customer_profiles
		(id,business_id,corporate_customer_code,organization_name,normalized_name,display_name,organization_type,
		 status,scope_mode,account_manager_staff_id,relationship_started_at,external_organization_ref,created_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,'PROSPECT',$8,$9,$10,$11,$12) RETURNING `+corporateCols,
		newID("crmcorp_"), actor.VendorID, code, validated.OrganizationName, validated.NormalizedName,
		validated.DisplayName, validated.OrganizationType, validated.ScopeMode, validated.AccountManagerStaffRef,
		validated.RelationshipStartedAt, validated.ExternalOrganizationID, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			// The only unique key a client can collide on is the trusted
			// external organization reference; the code is server-allocated.
			return CorporateProfile{}, crmError(
				apperrors.Conflict("A corporate customer with this external reference already exists.", nil),
				CodeCorporateDuplicate)
		}
		return CorporateProfile{}, err
	}
	if err = s.replaceBranchScopesTx(ctx, tx, actor, profile, validated.BranchRefs); err != nil {
		return CorporateProfile{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.created", "crm_corporate_customer", profile.ID, "",
		nil, safeCorporateAudit(profile, validated.BranchRefs), ""); err != nil {
		return CorporateProfile{}, err
	}
	if err = s.publishCorporate(ctx, tx, actor, "vendor.crm.corporate.created", profile); err != nil {
		return CorporateProfile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateProfile{}, err
	}
	s.metrics.Inc("crm_corporate_create_total", "organization_type", profile.OrganizationType, "scope", profile.ScopeMode)
	return profile, nil
}

// allocateCorporateCode hands out the next vendor-local reference.
//
// The counter is per business rather than global: a global sequence would make
// CORP-000001 disclose how many corporate accounts every other tenant had
// created. The upsert is atomic, so two concurrent creates cannot receive the
// same code.
func (s *Service) allocateCorporateCode(ctx context.Context, tx pgx.Tx, businessID string) (string, error) {
	var value int64
	if err := tx.QueryRow(ctx, `INSERT INTO corporate_customer_code_counters(business_id,next_value)
		VALUES($1,2)
		ON CONFLICT (business_id) DO UPDATE SET next_value = corporate_customer_code_counters.next_value + 1
		RETURNING next_value - 1`, businessID).Scan(&value); err != nil {
		return "", err
	}
	return fmt.Sprintf("CORP-%06d", value), nil
}

// UpdateCorporateCustomerInput edits an account's descriptive fields.
//
// Status is deliberately absent: a lifecycle change is its own command with its
// own transition rules, so a PATCH can never jump an account into a state the
// state machine would have refused.
type UpdateCorporateCustomerInput struct {
	Profile         CorporateProfileInput
	ExpectedVersion int64
}

// UpdateCorporateCustomer edits an account in place.
func (s *Service) UpdateCorporateCustomer(ctx context.Context, actor Actor, scope Scope, corporateID string, in UpdateCorporateCustomerInput) (CorporateProfile, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateProfile{}, err
	}
	if in.ExpectedVersion < 1 {
		return CorporateProfile{}, corporateValidation("expected_version", "Expected version is required.",
			CodeCorporateVersionConflict)
	}
	validated, err := validateCorporateProfileInput(in.Profile, s.now())
	if err != nil {
		return CorporateProfile{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return CorporateProfile{}, err
	}
	if current.Version != in.ExpectedVersion {
		return CorporateProfile{}, errCorporateVersionConflict()
	}
	scopeChanged := validated.ScopeMode != current.ScopeMode
	if scopeChanged {
		if err = s.authorizeCorporateScopeChange(scope, validated.ScopeMode, validated.BranchRefs); err != nil {
			return CorporateProfile{}, err
		}
	}
	if err = s.assertBranchesOwned(ctx, tx, actor.VendorID, validated.BranchRefs); err != nil {
		return CorporateProfile{}, err
	}

	manager := validated.AccountManagerStaffRef
	if in.Profile.ClearAccountManager {
		manager = nil
	} else if manager == nil {
		manager = current.AccountManagerStaffID
	}
	if err = s.assertAccountManager(ctx, tx, actor.VendorID, manager); err != nil {
		return CorporateProfile{}, err
	}

	updated, err := scanCorporate(tx.QueryRow(ctx, `UPDATE corporate_customer_profiles
		SET organization_name=$1, normalized_name=$2, display_name=$3, organization_type=$4, scope_mode=$5,
			account_manager_staff_id=$6, relationship_started_at=$7, external_organization_ref=$8,
			updated_at=now(), version=version+1
		WHERE business_id=$9 AND id=$10 AND version=$11 AND status <> 'ARCHIVED'
		RETURNING `+corporateCols,
		validated.OrganizationName, validated.NormalizedName, validated.DisplayName, validated.OrganizationType,
		validated.ScopeMode, manager, validated.RelationshipStartedAt, validated.ExternalOrganizationID,
		actor.VendorID, corporateID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateProfile{}, errCorporateVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateProfile{}, crmError(
				apperrors.Conflict("A corporate customer with this external reference already exists.", nil),
				CodeCorporateDuplicate)
		}
		return CorporateProfile{}, err
	}
	if err = s.replaceBranchScopesTx(ctx, tx, actor, updated, validated.BranchRefs); err != nil {
		return CorporateProfile{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.updated", "crm_corporate_customer", corporateID, "",
		safeCorporateAudit(current, nil), safeCorporateAudit(updated, validated.BranchRefs), ""); err != nil {
		return CorporateProfile{}, err
	}
	if err = s.publishCorporate(ctx, tx, actor, "vendor.crm.corporate.updated", updated); err != nil {
		return CorporateProfile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateProfile{}, err
	}
	return updated, nil
}

// TransitionCorporateCustomer moves an account through its lifecycle.
//
// Every move is checked against the transition table, so an account cannot be
// put on hold before it was ever active, and nothing returns from ARCHIVED.
func (s *Service) TransitionCorporateCustomer(ctx context.Context, actor Actor, scope Scope, corporateID, target string, expectedVersion int64) (CorporateProfile, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateProfile{}, err
	}
	target = strings.ToUpper(strings.TrimSpace(target))
	if !corporateStatuses[target] || target == CorporateProspect {
		// PROSPECT is an opening state, not a destination: an account that has
		// been active cannot be un-started.
		return CorporateProfile{}, corporateValidation("status", "Unsupported corporate status transition.",
			CodeCorporateInvalidState)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadCorporateForUpdate(ctx, tx, actor.VendorID, corporateID)
	if err != nil {
		return CorporateProfile{}, err
	}
	if err = s.assertCorporateVisible(ctx, tx, scope, current); err != nil {
		return CorporateProfile{}, err
	}
	// Repeating a settled transition returns the settled record, so a retried
	// command converges instead of failing on a version it already advanced.
	if current.Status == target {
		if err = tx.Commit(ctx); err != nil {
			return CorporateProfile{}, err
		}
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return CorporateProfile{}, errCorporateVersionConflict()
	}
	if !corporateTransitions[current.Status][target] {
		return CorporateProfile{}, errCorporateState(
			fmt.Sprintf("A %s corporate customer cannot move to %s.", current.Status, target))
	}

	var archivedAt *time.Time
	if target == CorporateArchived {
		now := s.now().UTC()
		archivedAt = &now
	}
	updated, err := scanCorporate(tx.QueryRow(ctx, `UPDATE corporate_customer_profiles
		SET status=$1, archived_at=$2, updated_at=now(), version=version+1
		WHERE business_id=$3 AND id=$4 AND version=$5 RETURNING `+corporateCols,
		target, archivedAt, actor.VendorID, corporateID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateProfile{}, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateProfile{}, err
	}

	action := "vendor.crm.corporate.status.changed"
	if target == CorporateArchived {
		action = "vendor.crm.corporate.archived"
		// Unfinished work on a retired account is cancelled transactionally
		// rather than left open forever or silently deleted: the task history
		// records exactly what happened and why.
		if err = s.cancelCorporateTasksOnArchiveTx(ctx, tx, actor, updated); err != nil {
			return CorporateProfile{}, err
		}
	}
	if err = s.auditTx(ctx, tx, actor, action, "crm_corporate_customer", corporateID, "",
		map[string]any{"status": current.Status}, map[string]any{"status": target}, ""); err != nil {
		return CorporateProfile{}, err
	}
	if err = s.publishCorporate(ctx, tx, actor, action, updated); err != nil {
		return CorporateProfile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateProfile{}, err
	}
	s.metrics.Inc("crm_corporate_transition_total", "status", target)
	return updated, nil
}

// ---------------------------------------------------------------------------
// Branch scope
// ---------------------------------------------------------------------------

// ReplaceBranchScopes sets the exact branches an account is limited to.
//
// A bounded replace rather than add/remove endpoints: the interesting question
// is "which branches may work this account", and answering it with a set makes
// the audit entry a complete statement rather than a diff a reader has to
// reconstruct.
func (s *Service) ReplaceBranchScopes(ctx context.Context, actor Actor, scope Scope, corporateID, scopeMode string, branchRefs []string, expectedVersion int64) (CorporateProfile, []string, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateProfile{}, nil, err
	}
	if expectedVersion < 1 {
		return CorporateProfile{}, nil, corporateValidation("expected_version", "Expected version is required.",
			CodeCorporateVersionConflict)
	}
	mode := strings.ToUpper(strings.TrimSpace(scopeMode))
	if mode != CorporateScopeBusiness && mode != CorporateScopeBranchSet {
		return CorporateProfile{}, nil, corporateValidation("scope_mode", "Scope must be BUSINESS or BRANCH_SET.",
			CodeCorporateScopeDenied)
	}
	branches, err := normalizeBranchRefs(branchRefs)
	if err != nil {
		return CorporateProfile{}, nil, err
	}
	if mode == CorporateScopeBranchSet && len(branches) == 0 {
		return CorporateProfile{}, nil, corporateValidation("branch_refs",
			"A branch-scoped corporate customer needs at least one branch.", CodeCorporateScopeDenied)
	}
	if mode == CorporateScopeBusiness {
		branches = nil
	}
	if err = s.authorizeCorporateScopeChange(scope, mode, branches); err != nil {
		return CorporateProfile{}, nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateProfile{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return CorporateProfile{}, nil, err
	}
	if current.Version != expectedVersion {
		return CorporateProfile{}, nil, errCorporateVersionConflict()
	}
	if err = s.assertBranchesOwned(ctx, tx, actor.VendorID, branches); err != nil {
		return CorporateProfile{}, nil, err
	}
	previous, err := s.BranchScopesFor(ctx, tx, actor.VendorID, corporateID)
	if err != nil {
		return CorporateProfile{}, nil, err
	}

	updated, err := scanCorporate(tx.QueryRow(ctx, `UPDATE corporate_customer_profiles
		SET scope_mode=$1, updated_at=now(), version=version+1
		WHERE business_id=$2 AND id=$3 AND version=$4 AND status <> 'ARCHIVED' RETURNING `+corporateCols,
		mode, actor.VendorID, corporateID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateProfile{}, nil, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateProfile{}, nil, err
	}
	if err = s.replaceBranchScopesTx(ctx, tx, actor, updated, branches); err != nil {
		return CorporateProfile{}, nil, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.scope.changed", "crm_corporate_customer", corporateID, "",
		map[string]any{"scope_mode": current.ScopeMode, "branch_refs": previous},
		map[string]any{"scope_mode": mode, "branch_refs": branches}, ""); err != nil {
		return CorporateProfile{}, nil, err
	}
	if err = s.publishCorporate(ctx, tx, actor, "vendor.crm.corporate.updated", updated); err != nil {
		return CorporateProfile{}, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateProfile{}, nil, err
	}
	return updated, branches, nil
}

func (s *Service) replaceBranchScopesTx(ctx context.Context, tx pgx.Tx, actor Actor, profile CorporateProfile, branches []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM corporate_customer_branch_scopes
		WHERE business_id=$1 AND corporate_customer_id=$2`, actor.VendorID, profile.ID); err != nil {
		return err
	}
	if profile.ScopeMode != CorporateScopeBranchSet || len(branches) == 0 {
		return nil
	}
	ids := make([]string, len(branches))
	for index := range branches {
		ids[index] = newID("crmcbs_")
	}
	_, err := tx.Exec(ctx, `INSERT INTO corporate_customer_branch_scopes(id,business_id,corporate_customer_id,branch_id)
		SELECT s.id, $1, $2, s.branch_id FROM unnest($3::text[], $4::text[]) AS s(id, branch_id)`,
		actor.VendorID, profile.ID, ids, branches)
	return err
}

// authorizeCorporateScopeChange decides whether an actor may open an account at
// a given reach.
//
// A BUSINESS-scope account reasons over the whole business, so it needs a
// business-wide grant; a branch manager creating one would hand themselves
// commercial reach they cannot otherwise see. A BRANCH_SET account must name
// only branches the actor actually holds.
func (s *Service) authorizeCorporateScopeChange(scope Scope, mode string, branches []string) error {
	if scope.Global {
		return nil
	}
	if mode != CorporateScopeBranchSet {
		return errScopeDenied(CodeCorporateScopeDenied)
	}
	for _, branch := range branches {
		if !containsString(scope.BranchIDs, branch) {
			return errScopeDenied(CodeCorporateScopeDenied)
		}
	}
	return nil
}

func (s *Service) assertBranchesOwned(ctx context.Context, q querier, businessID string, branches []string) error {
	if len(branches) == 0 {
		return nil
	}
	var owned int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM vendor_branches WHERE business_id=$1 AND id = ANY($2)`,
		businessID, branches).Scan(&owned); err != nil {
		return err
	}
	if owned != len(branches) {
		return crmError(apperrors.Forbidden("A referenced branch does not belong to this business.", nil), CodeCrossTenant)
	}
	return nil
}

// assertAccountManager checks that a proposed relationship owner is a real,
// active member of this vendor's staff.
//
// This is Vendor-side ownership, not a contact at the customer. Accepting an
// arbitrary staff id would let one tenant name another tenant's employee as the
// owner of a relationship they cannot see.
func (s *Service) assertAccountManager(ctx context.Context, q querier, businessID string, staffRef *string) error {
	if staffRef == nil {
		return nil
	}
	var active bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff
		WHERE id=$1 AND vendor_id=$2 AND status='active')`, *staffRef, businessID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return crmError(apperrors.Validation("The selected account manager is not available.", nil),
			CodeCorporateManagerInvalid)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Audit and events
// ---------------------------------------------------------------------------

// safeCorporateAudit is the audit payload: organizational relationship metadata
// only. No contact reference, no individual identity, no free text beyond the
// organization's own name.
func safeCorporateAudit(c CorporateProfile, branches []string) map[string]any {
	payload := map[string]any{
		"corporate_ref": c.ID, "corporate_customer_code": c.Code, "organization_name": c.OrganizationName,
		"organization_type": c.OrganizationType, "status": c.Status, "scope_mode": c.ScopeMode,
		"account_manager_staff_ref": c.AccountManagerStaffID, "version": c.Version,
	}
	if branches != nil {
		payload["branch_refs"] = branches
	}
	return payload
}

func (s *Service) publishCorporate(ctx context.Context, tx pgx.Tx, actor Actor, event string, c CorporateProfile) error {
	return s.publishTx(ctx, tx, actor, event, "crm_corporate_customer", c.ID, c.Version, map[string]any{
		"corporate_ref": c.ID, "corporate_customer_code": c.Code, "organization_type": c.OrganizationType,
		"status": c.Status, "scope_mode": c.ScopeMode, "version": c.Version,
	})
}
