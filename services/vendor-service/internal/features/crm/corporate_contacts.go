package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Corporate contacts are the most privacy-sensitive part of Section 8G, so the
// design is deliberately minimal: a contact is a REFERENCE to an existing
// vendor-local CRM profile plus a relationship role, and nothing else.
//
// There is no name, email, phone or address field to copy an individual's
// details into. That is not an oversight to be corrected when a UI needs to
// show a contact list — it is the boundary. customer-service remains the
// identity authority, and a corporate CRM that cached contact details would be
// a second copy of customer identity with none of the privacy machinery that
// makes the first one lawful.

// Contact roles describe the person's role AT THE CUSTOMER's organization.
// They are not Vendor IAM roles and are never consulted for authorization.
const (
	ContactRolePrimary        = "PRIMARY_CONTACT"
	ContactRoleProcurement    = "PROCUREMENT"
	ContactRoleOperations     = "OPERATIONS"
	ContactRoleFinance        = "FINANCE"
	ContactRoleManagement     = "MANAGEMENT"
	ContactRoleAdministration = "ADMINISTRATION"
	ContactRoleTechnical      = "TECHNICAL"
	ContactRoleOther          = "OTHER"
)

var contactRoles = map[string]bool{
	ContactRolePrimary: true, ContactRoleProcurement: true, ContactRoleOperations: true,
	ContactRoleFinance: true, ContactRoleManagement: true, ContactRoleAdministration: true,
	ContactRoleTechnical: true, ContactRoleOther: true,
}

// Membership roles are corporate relationship metadata. APPROVER records that
// somebody approves things at their own company; it creates no approval
// workflow, no purchase-order gate and no Vendor authorization whatsoever.
const (
	MembershipMember      = "MEMBER"
	MembershipManager     = "MANAGER"
	MembershipApprover    = "APPROVER"
	MembershipRequester   = "REQUESTER"
	MembershipProcurement = "PROCUREMENT"
	MembershipFinance     = "FINANCE"
	MembershipOperations  = "OPERATIONS"
	MembershipOther       = "OTHER"
)

var membershipRoles = map[string]bool{
	MembershipMember: true, MembershipManager: true, MembershipApprover: true, MembershipRequester: true,
	MembershipProcurement: true, MembershipFinance: true, MembershipOperations: true, MembershipOther: true,
}

// CorporateContact is one individual's relationship with the organization.
type CorporateContact struct {
	ID                  string
	BusinessID          string
	CorporateCustomerID string
	CustomerProfileID   string
	OriginalProfileID   string
	RelationshipRole    string
	IsPrimary           bool
	Status              string
	PrivacySuppressed   bool
	CreatedByStaffID    string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
	// SafeDisplayLabel is the vendor-local presentation label Section 8A
	// already holds, if any. It is joined for reads and never stored here: a
	// copy would become stale the moment the authority changed it, and would be
	// a second place personal data lived.
	SafeDisplayLabel *string
}

// Actionable reports whether the contact may be acted on today. A redacted
// individual leaves the organizational relationship intact and stops being
// contactable — the two are different obligations and are tracked separately.
func (c CorporateContact) Actionable() bool {
	return c.Status == CorporateChildActive && !c.PrivacySuppressed
}

// CorporateMembership binds a contact to a site, a cost centre, or both.
type CorporateMembership struct {
	ID                  string
	BusinessID          string
	CorporateCustomerID string
	ContactID           string
	LocationID          *string
	CostCenterID        *string
	MembershipRole      string
	Status              string
	CreatedByStaffID    string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
}

func errCorporateContactNotFound() error {
	return crmError(apperrors.NotFound("Corporate contact not found.", nil), CodeCorporateContactNotFound)
}
func errCorporateMembershipNotFound() error {
	return crmError(apperrors.NotFound("Corporate membership not found.", nil), CodeCorporateMembershipGone)
}

// ---------------------------------------------------------------------------
// Contacts
// ---------------------------------------------------------------------------

const corporateContactCols = `id,business_id,corporate_customer_id,customer_profile_id,original_profile_id,
relationship_role,is_primary_contact,status,privacy_suppressed,created_by_staff_id,created_at,updated_at,
archived_at,version`

func scanCorporateContact(row rowScanner) (CorporateContact, error) {
	var c CorporateContact
	err := row.Scan(&c.ID, &c.BusinessID, &c.CorporateCustomerID, &c.CustomerProfileID, &c.OriginalProfileID,
		&c.RelationshipRole, &c.IsPrimary, &c.Status, &c.PrivacySuppressed, &c.CreatedByStaffID,
		&c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version)
	return c, err
}

// CorporateContactInput is a vendor request to define or edit a contact.
//
// It carries a CRM profile reference and a role. There is no name, email or
// phone field, because there is nowhere to put one.
type CorporateContactInput struct {
	CustomerProfileRef string
	RelationshipRole   string
	IsPrimary          *bool
	Status             string
	ExpectedVersion    int64
}

// CreateCorporateContact links an individual to the organization.
func (s *Service) CreateCorporateContact(ctx context.Context, actor Actor, scope Scope, corporateID string, in CorporateContactInput) (CorporateContact, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateContact{}, err
	}
	profileRef := strings.TrimSpace(in.CustomerProfileRef)
	if profileRef == "" || !safeToken.MatchString(profileRef) {
		return CorporateContact{}, corporateValidation("customer_profile_ref",
			"A valid customer relationship reference is required.", CodeCorporateContactProfile)
	}
	role := strings.ToUpper(strings.TrimSpace(in.RelationshipRole))
	if role == "" {
		role = ContactRoleOther
	}
	if !contactRoles[role] {
		return CorporateContact{}, corporateValidation("relationship_role", "Unsupported contact role.",
			CodeCorporateInvalidState)
	}
	primary := in.IsPrimary != nil && *in.IsPrimary

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateContact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return CorporateContact{}, err
	}
	// The individual's profile row is locked before anything is written. A
	// redaction or an identity merge takes the same lock, so the two serialize:
	// either their transaction commits first and the check below refuses, or
	// this insert commits first and their handler suppresses or canonicalizes
	// the row immediately afterwards.
	if err = s.lockContactProfile(ctx, tx, actor.VendorID, profileRef); err != nil {
		return CorporateContact{}, err
	}
	if primary {
		// Demote the incumbent inside the same transaction. Doing it here
		// rather than relying on the caller means "one primary contact" holds
		// even when two operators promote different people at once: the second
		// blocks on this row and then finds the first already in place.
		if err = s.demotePrimaryContacts(ctx, tx, actor.VendorID, corporateID, ""); err != nil {
			return CorporateContact{}, err
		}
	}

	contact, err := scanCorporateContact(tx.QueryRow(ctx, `INSERT INTO corporate_customer_contacts
		(id,business_id,corporate_customer_id,customer_profile_id,original_profile_id,relationship_role,
		 is_primary_contact,status,created_by_staff_id)
		VALUES($1,$2,$3,$4,$4,$5,$6,'ACTIVE',$7) RETURNING `+corporateContactCols,
		newID("crmcct_"), actor.VendorID, profile.ID, profileRef, role, primary, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateContact{}, crmError(
				apperrors.Conflict("This person is already a contact for this corporate customer.", nil),
				CodeCorporateContactDup)
		}
		return CorporateContact{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.contact.created", "crm_corporate_contact",
		contact.ID, "", nil, safeContactAudit(contact), ""); err != nil {
		return CorporateContact{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.corporate.contact.created", "crm_corporate_contact",
		contact.ID, contact.Version, map[string]any{
			"corporate_ref": profile.ID, "contact_ref": contact.ID, "relationship_role": contact.RelationshipRole,
			"is_primary_contact": contact.IsPrimary}); err != nil {
		return CorporateContact{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateContact{}, err
	}
	s.metrics.Inc("crm_corporate_contact_create_total", "relationship_role", contact.RelationshipRole)
	return contact, nil
}

// lockContactProfile takes the Section 8A profile fence and refuses a
// relationship the privacy architecture has already ruled out.
//
// This is the same lock ApplyPrivacy and ApplyIdentityLink take, which is what
// makes the create-versus-redaction and create-versus-merge races resolve
// deterministically rather than by timing.
func (s *Service) lockContactProfile(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	profile, err := scanProfile(tx.QueryRow(ctx, `/* crm_corporate_contact_profile_fence */ SELECT `+profileCols+`
		FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessID, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		// Reported as an invalid profile rather than not-found so a caller
		// cannot use contact creation to probe which relationship ids exist.
		return crmError(apperrors.Validation("That customer relationship is not available.", nil),
			CodeCorporateContactProfile)
	}
	if err != nil {
		return err
	}
	if profile.IsRedacted() {
		return crmError(apperrors.Conflict("That customer relationship has been redacted.", nil),
			CodeCorporateContactRedacted)
	}
	if profile.IsMergedSource() {
		// Deliberately refused rather than silently redirected to the survivor:
		// quietly linking a different person than the caller named would be a
		// worse outcome than telling them to use the surviving relationship.
		return crmError(apperrors.Conflict("That customer relationship has been merged into another.", nil),
			CodeCorporateContactMerged)
	}
	return nil
}

func (s *Service) demotePrimaryContacts(ctx context.Context, tx pgx.Tx, businessID, corporateID, exceptContactID string) error {
	_, err := tx.Exec(ctx, `UPDATE corporate_customer_contacts
		SET is_primary_contact=false, updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_customer_id=$2 AND is_primary_contact AND status='ACTIVE'
		  AND ($3='' OR id <> $3)`, businessID, corporateID, exceptContactID)
	return err
}

// UpdateCorporateContact edits a contact's role, status or primary flag.
func (s *Service) UpdateCorporateContact(ctx context.Context, actor Actor, scope Scope, corporateID, contactID string, in CorporateContactInput) (CorporateContact, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateContact{}, err
	}
	if in.ExpectedVersion < 1 {
		return CorporateContact{}, corporateValidation("expected_version", "Expected version is required.",
			CodeCorporateVersionConflict)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateContact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateContact{}, err
	}
	current, err := s.lockContact(ctx, tx, actor.VendorID, corporateID, contactID)
	if err != nil {
		return CorporateContact{}, err
	}
	if current.Version != in.ExpectedVersion {
		return CorporateContact{}, errCorporateVersionConflict()
	}
	if current.Status == CorporateChildArchived {
		return CorporateContact{}, errCorporateState("An archived contact cannot be changed.")
	}
	if current.PrivacySuppressed {
		// The organizational relationship survives a redaction; the person does
		// not become editable again because of it.
		return CorporateContact{}, crmError(
			apperrors.Conflict("That customer relationship has been redacted.", nil), CodeCorporateContactRedacted)
	}

	role := current.RelationshipRole
	if value := strings.ToUpper(strings.TrimSpace(in.RelationshipRole)); value != "" {
		if !contactRoles[value] {
			return CorporateContact{}, corporateValidation("relationship_role", "Unsupported contact role.",
				CodeCorporateInvalidState)
		}
		role = value
	}
	status := current.Status
	if value := strings.ToUpper(strings.TrimSpace(in.Status)); value != "" {
		if !corporateChildStatuses[value] {
			return CorporateContact{}, corporateValidation("status", "Unsupported contact status.",
				CodeCorporateInvalidState)
		}
		if value == CorporateChildArchived {
			return CorporateContact{}, corporateValidation("status",
				"Archive a contact through its archive command.", CodeCorporateInvalidState)
		}
		status = value
	}
	primary := current.IsPrimary
	if in.IsPrimary != nil {
		primary = *in.IsPrimary
	}
	if primary && status != CorporateChildActive {
		return CorporateContact{}, corporateValidation("is_primary_contact",
			"Only an active contact can be the primary contact.", CodeCorporateInvalidState)
	}
	if primary {
		if err = s.demotePrimaryContacts(ctx, tx, actor.VendorID, corporateID, contactID); err != nil {
			return CorporateContact{}, err
		}
	}

	updated, err := scanCorporateContact(tx.QueryRow(ctx, `UPDATE corporate_customer_contacts
		SET relationship_role=$1, is_primary_contact=$2, status=$3, updated_at=now(), version=version+1
		WHERE business_id=$4 AND corporate_customer_id=$5 AND id=$6 AND version=$7 AND status <> 'ARCHIVED'
		RETURNING `+corporateContactCols,
		role, primary, status, actor.VendorID, corporateID, contactID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateContact{}, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateContact{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.contact.updated", "crm_corporate_contact",
		contactID, "", safeContactAudit(current), safeContactAudit(updated), ""); err != nil {
		return CorporateContact{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateContact{}, err
	}
	return updated, nil
}

// ArchiveCorporateContact ends a contact relationship without destroying it.
func (s *Service) ArchiveCorporateContact(ctx context.Context, actor Actor, scope Scope, corporateID, contactID string, expectedVersion int64) (CorporateContact, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateContact{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateContact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateContact{}, err
	}
	current, err := s.lockContact(ctx, tx, actor.VendorID, corporateID, contactID)
	if err != nil {
		return CorporateContact{}, err
	}
	if current.Status == CorporateChildArchived {
		if err = tx.Commit(ctx); err != nil {
			return CorporateContact{}, err
		}
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return CorporateContact{}, errCorporateVersionConflict()
	}

	updated, err := scanCorporateContact(tx.QueryRow(ctx, `UPDATE corporate_customer_contacts
		SET status='ARCHIVED', is_primary_contact=false, archived_at=now(), updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 AND version=$4
		RETURNING `+corporateContactCols, actor.VendorID, corporateID, contactID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateContact{}, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateContact{}, err
	}
	if err = s.deactivateContactMemberships(ctx, tx, actor.VendorID, contactID); err != nil {
		return CorporateContact{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.contact.archived", "crm_corporate_contact",
		contactID, "", safeContactAudit(current), safeContactAudit(updated), ""); err != nil {
		return CorporateContact{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.corporate.contact.archived", "crm_corporate_contact",
		contactID, updated.Version, map[string]any{
			"corporate_ref": corporateID, "contact_ref": contactID}); err != nil {
		return CorporateContact{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateContact{}, err
	}
	return updated, nil
}

// deactivateContactMemberships retires a contact's memberships alongside them.
// The rows survive as evidence of what the arrangement was; they simply stop
// being actionable.
func (s *Service) deactivateContactMemberships(ctx context.Context, tx pgx.Tx, businessID, contactID string) error {
	_, err := tx.Exec(ctx, `UPDATE corporate_customer_memberships
		SET status='INACTIVE', updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_contact_id=$2 AND status='ACTIVE'`, businessID, contactID)
	return err
}

func (s *Service) lockContact(ctx context.Context, tx pgx.Tx, businessID, corporateID, contactID string) (CorporateContact, error) {
	contact, err := scanCorporateContact(tx.QueryRow(ctx, `SELECT `+corporateContactCols+`
		FROM corporate_customer_contacts WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 FOR UPDATE`,
		businessID, corporateID, contactID))
	if errors.Is(err, pgx.ErrNoRows) {
		// corporate_customer_id is part of the predicate, so a contact from
		// another account presented under this URL is simply not found.
		return CorporateContact{}, errCorporateContactNotFound()
	}
	return contact, err
}

// GetCorporateContact reads one contact through its account.
func (s *Service) GetCorporateContact(ctx context.Context, actor Actor, scope Scope, corporateID, contactID string) (CorporateContact, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return CorporateContact{}, err
	}
	contact, err := scanCorporateContactWithLabel(s.db.QueryRow(ctx, `SELECT `+qualifiedContactCols+`, p.safe_display_label
		FROM corporate_customer_contacts c
		JOIN vendor_customer_profiles p ON p.id=c.customer_profile_id AND p.business_id=c.business_id
		WHERE c.business_id=$1 AND c.corporate_customer_id=$2 AND c.id=$3`, actor.VendorID, corporateID, contactID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateContact{}, errCorporateContactNotFound()
	}
	return contact, err
}

const qualifiedContactCols = `c.id,c.business_id,c.corporate_customer_id,c.customer_profile_id,c.original_profile_id,
c.relationship_role,c.is_primary_contact,c.status,c.privacy_suppressed,c.created_by_staff_id,c.created_at,
c.updated_at,c.archived_at,c.version`

func scanCorporateContactWithLabel(row rowScanner) (CorporateContact, error) {
	var c CorporateContact
	err := row.Scan(&c.ID, &c.BusinessID, &c.CorporateCustomerID, &c.CustomerProfileID, &c.OriginalProfileID,
		&c.RelationshipRole, &c.IsPrimary, &c.Status, &c.PrivacySuppressed, &c.CreatedByStaffID,
		&c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version, &c.SafeDisplayLabel)
	if err != nil {
		return CorporateContact{}, err
	}
	if c.PrivacySuppressed {
		// A redacted individual has no presentation label to show, whatever the
		// profile row still happens to carry.
		c.SafeDisplayLabel = nil
	}
	return c, nil
}

// ListCorporateContacts pages an account's contacts.
//
// The display label comes from the Section 8A profile at read time rather than
// from a stored copy, so it cannot go stale and there is no second place
// personal data lives.
func (s *Service) ListCorporateContacts(ctx context.Context, actor Actor, scope Scope, corporateID, status, role string, includeSuppressed bool, limit, offset int) ([]CorporateContact, bool, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	args := []any{actor.VendorID, corporateID}
	where := []string{"c.business_id=$1", "c.corporate_customer_id=$2"}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}
	if status != "" {
		value := strings.ToUpper(strings.TrimSpace(status))
		if !corporateChildStatuses[value] {
			return nil, false, corporateValidation("status", "Unsupported contact status.", CodeCorporateInvalidState)
		}
		where = append(where, "c.status="+bind(value))
	}
	if role != "" {
		value := strings.ToUpper(strings.TrimSpace(role))
		if !contactRoles[value] {
			return nil, false, corporateValidation("relationship_role", "Unsupported contact role.", CodeCorporateInvalidState)
		}
		where = append(where, "c.relationship_role="+bind(value))
	}
	if !includeSuppressed {
		// Redacted individuals disappear from the ordinary actionable list.
		// They remain countable only through the explicit compliance flag.
		where = append(where, "c.privacy_suppressed=false")
	}
	limitArg, offsetArg := bind(limit+1), bind(offset)

	rows, err := s.db.Query(ctx, `SELECT `+qualifiedContactCols+`, p.safe_display_label
		FROM corporate_customer_contacts c
		JOIN vendor_customer_profiles p ON p.id=c.customer_profile_id AND p.business_id=c.business_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.is_primary_contact DESC, c.relationship_role ASC, c.created_at DESC, c.id ASC
		LIMIT `+limitArg+` OFFSET `+offsetArg, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]CorporateContact, 0, limit)
	for rows.Next() {
		contact, scanErr := scanCorporateContactWithLabel(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, contact)
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

// safeContactAudit records the relationship, never the person. The individual
// appears only as the opaque vendor-local profile reference the row already
// holds.
func safeContactAudit(c CorporateContact) map[string]any {
	return map[string]any{
		"contact_ref": c.ID, "corporate_ref": c.CorporateCustomerID, "profile_ref": c.CustomerProfileID,
		"relationship_role": c.RelationshipRole, "is_primary_contact": c.IsPrimary, "status": c.Status,
		"privacy_suppressed": c.PrivacySuppressed, "version": c.Version,
	}
}

// ---------------------------------------------------------------------------
// Memberships
// ---------------------------------------------------------------------------

const corporateMembershipCols = `id,business_id,corporate_customer_id,corporate_contact_id,corporate_location_id,
cost_center_id,membership_role,status,created_by_staff_id,created_at,updated_at,archived_at,version`

func scanCorporateMembership(row rowScanner) (CorporateMembership, error) {
	var m CorporateMembership
	err := row.Scan(&m.ID, &m.BusinessID, &m.CorporateCustomerID, &m.ContactID, &m.LocationID, &m.CostCenterID,
		&m.MembershipRole, &m.Status, &m.CreatedByStaffID, &m.CreatedAt, &m.UpdatedAt, &m.ArchivedAt, &m.Version)
	return m, err
}

// CorporateMembershipInput binds a contact to a place in the organization.
type CorporateMembershipInput struct {
	ContactRef      string
	LocationRef     string
	CostCenterRef   string
	ClearLocation   bool
	ClearCostCenter bool
	MembershipRole  string
	Status          string
	ExpectedVersion int64
}

// CreateCorporateMembership records where a contact sits in the organization.
func (s *Service) CreateCorporateMembership(ctx context.Context, actor Actor, scope Scope, corporateID string, in CorporateMembershipInput) (CorporateMembership, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateMembership{}, err
	}
	contactRef := strings.TrimSpace(in.ContactRef)
	if contactRef == "" || !safeToken.MatchString(contactRef) {
		return CorporateMembership{}, corporateValidation("contact_ref", "A valid corporate contact is required.",
			CodeCorporateContactNotFound)
	}
	role := strings.ToUpper(strings.TrimSpace(in.MembershipRole))
	if role == "" {
		role = MembershipMember
	}
	if !membershipRoles[role] {
		return CorporateMembership{}, corporateValidation("membership_role", "Unsupported membership role.",
			CodeCorporateInvalidState)
	}
	var locationID, costCenterID *string
	if value := strings.TrimSpace(in.LocationRef); value != "" {
		locationID = &value
	}
	if value := strings.TrimSpace(in.CostCenterRef); value != "" {
		costCenterID = &value
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateMembership{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return CorporateMembership{}, err
	}
	contact, err := s.lockContact(ctx, tx, actor.VendorID, corporateID, contactRef)
	if err != nil {
		return CorporateMembership{}, err
	}
	if !contact.Actionable() {
		return CorporateMembership{}, s.contactNotActionableError(contact)
	}
	// The individual's profile row is locked too, so a merge landing during
	// membership creation cannot leave a membership attached to a contact that
	// is about to stop being canonical.
	if err = s.lockContactProfile(ctx, tx, actor.VendorID, contact.CustomerProfileID); err != nil {
		return CorporateMembership{}, err
	}
	if err = s.assertLocationBelongs(ctx, tx, actor.VendorID, corporateID, locationID); err != nil {
		return CorporateMembership{}, err
	}
	if err = s.assertCostCenterBelongs(ctx, tx, actor.VendorID, corporateID, costCenterID); err != nil {
		return CorporateMembership{}, err
	}

	membership, err := scanCorporateMembership(tx.QueryRow(ctx, `INSERT INTO corporate_customer_memberships
		(id,business_id,corporate_customer_id,corporate_contact_id,corporate_location_id,cost_center_id,
		 membership_role,status,created_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,'ACTIVE',$8) RETURNING `+corporateMembershipCols,
		newID("crmcmb_"), actor.VendorID, profile.ID, contact.ID, locationID, costCenterID, role, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateMembership{}, crmError(
				apperrors.Conflict("This membership already exists for the contact.", nil), CodeCorporateDuplicate)
		}
		return CorporateMembership{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.membership.created", "crm_corporate_membership",
		membership.ID, "", nil, safeMembershipAudit(membership), ""); err != nil {
		return CorporateMembership{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateMembership{}, err
	}
	return membership, nil
}

func (s *Service) contactNotActionableError(contact CorporateContact) error {
	if contact.PrivacySuppressed {
		return crmError(apperrors.Conflict("That customer relationship has been redacted.", nil),
			CodeCorporateContactRedacted)
	}
	return errCorporateState("That corporate contact is not active.")
}

// UpdateCorporateMembership edits a membership's role, placement or status.
func (s *Service) UpdateCorporateMembership(ctx context.Context, actor Actor, scope Scope, corporateID, membershipID string, in CorporateMembershipInput) (CorporateMembership, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateMembership{}, err
	}
	if in.ExpectedVersion < 1 {
		return CorporateMembership{}, corporateValidation("expected_version", "Expected version is required.",
			CodeCorporateVersionConflict)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateMembership{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateMembership{}, err
	}
	current, err := s.lockMembership(ctx, tx, actor.VendorID, corporateID, membershipID)
	if err != nil {
		return CorporateMembership{}, err
	}
	if current.Version != in.ExpectedVersion {
		return CorporateMembership{}, errCorporateVersionConflict()
	}
	if current.Status == CorporateChildArchived {
		return CorporateMembership{}, errCorporateState("An archived membership cannot be changed.")
	}

	role := current.MembershipRole
	if value := strings.ToUpper(strings.TrimSpace(in.MembershipRole)); value != "" {
		if !membershipRoles[value] {
			return CorporateMembership{}, corporateValidation("membership_role", "Unsupported membership role.",
				CodeCorporateInvalidState)
		}
		role = value
	}
	status := current.Status
	if value := strings.ToUpper(strings.TrimSpace(in.Status)); value != "" {
		if !corporateChildStatuses[value] || value == CorporateChildArchived {
			return CorporateMembership{}, corporateValidation("status", "Unsupported membership status.",
				CodeCorporateInvalidState)
		}
		status = value
	}
	location := current.LocationID
	if in.ClearLocation {
		location = nil
	} else if value := strings.TrimSpace(in.LocationRef); value != "" {
		location = &value
	}
	costCenter := current.CostCenterID
	if in.ClearCostCenter {
		costCenter = nil
	} else if value := strings.TrimSpace(in.CostCenterRef); value != "" {
		costCenter = &value
	}
	if err = s.assertLocationBelongs(ctx, tx, actor.VendorID, corporateID, location); err != nil {
		return CorporateMembership{}, err
	}
	if err = s.assertCostCenterBelongs(ctx, tx, actor.VendorID, corporateID, costCenter); err != nil {
		return CorporateMembership{}, err
	}

	updated, err := scanCorporateMembership(tx.QueryRow(ctx, `UPDATE corporate_customer_memberships
		SET corporate_location_id=$1, cost_center_id=$2, membership_role=$3, status=$4,
			updated_at=now(), version=version+1
		WHERE business_id=$5 AND corporate_customer_id=$6 AND id=$7 AND version=$8 AND status <> 'ARCHIVED'
		RETURNING `+corporateMembershipCols,
		location, costCenter, role, status, actor.VendorID, corporateID, membershipID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateMembership{}, errCorporateVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return CorporateMembership{}, crmError(
				apperrors.Conflict("This membership already exists for the contact.", nil), CodeCorporateDuplicate)
		}
		return CorporateMembership{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.membership.updated", "crm_corporate_membership",
		membershipID, "", safeMembershipAudit(current), safeMembershipAudit(updated), ""); err != nil {
		return CorporateMembership{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateMembership{}, err
	}
	return updated, nil
}

// ArchiveCorporateMembership ends a membership without destroying it.
func (s *Service) ArchiveCorporateMembership(ctx context.Context, actor Actor, scope Scope, corporateID, membershipID string, expectedVersion int64) (CorporateMembership, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return CorporateMembership{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CorporateMembership{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = s.corporateForWrite(ctx, tx, actor, scope, corporateID); err != nil {
		return CorporateMembership{}, err
	}
	current, err := s.lockMembership(ctx, tx, actor.VendorID, corporateID, membershipID)
	if err != nil {
		return CorporateMembership{}, err
	}
	if current.Status == CorporateChildArchived {
		if err = tx.Commit(ctx); err != nil {
			return CorporateMembership{}, err
		}
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return CorporateMembership{}, errCorporateVersionConflict()
	}
	updated, err := scanCorporateMembership(tx.QueryRow(ctx, `UPDATE corporate_customer_memberships
		SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 AND version=$4
		RETURNING `+corporateMembershipCols, actor.VendorID, corporateID, membershipID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateMembership{}, errCorporateVersionConflict()
	}
	if err != nil {
		return CorporateMembership{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.corporate.membership.archived", "crm_corporate_membership",
		membershipID, "", safeMembershipAudit(current), safeMembershipAudit(updated), ""); err != nil {
		return CorporateMembership{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CorporateMembership{}, err
	}
	return updated, nil
}

func (s *Service) lockMembership(ctx context.Context, tx pgx.Tx, businessID, corporateID, membershipID string) (CorporateMembership, error) {
	membership, err := scanCorporateMembership(tx.QueryRow(ctx, `SELECT `+corporateMembershipCols+`
		FROM corporate_customer_memberships WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 FOR UPDATE`,
		businessID, corporateID, membershipID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateMembership{}, errCorporateMembershipNotFound()
	}
	return membership, err
}

// ListCorporateMemberships pages an account's memberships.
func (s *Service) ListCorporateMemberships(ctx context.Context, actor Actor, scope Scope, corporateID, contactRef, status string, limit, offset int) ([]CorporateMembership, bool, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	args := []any{actor.VendorID, corporateID}
	where := []string{"m.business_id=$1", "m.corporate_customer_id=$2"}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}
	if contactRef != "" {
		where = append(where, "m.corporate_contact_id="+bind(strings.TrimSpace(contactRef)))
	}
	if status != "" {
		value := strings.ToUpper(strings.TrimSpace(status))
		if !corporateChildStatuses[value] {
			return nil, false, corporateValidation("status", "Unsupported membership status.", CodeCorporateInvalidState)
		}
		where = append(where, "m.status="+bind(value))
	}
	limitArg, offsetArg := bind(limit+1), bind(offset)
	rows, err := s.db.Query(ctx, `SELECT `+corporateMembershipCols+` FROM corporate_customer_memberships m
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY m.created_at DESC, m.id ASC LIMIT `+limitArg+` OFFSET `+offsetArg, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]CorporateMembership, 0, limit)
	for rows.Next() {
		membership, scanErr := scanCorporateMembership(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, membership)
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

// assertCostCenterBelongs proves a referenced label is part of this account.
func (s *Service) assertCostCenterBelongs(ctx context.Context, q querier, businessID, corporateID string, costCenterID *string) error {
	if costCenterID == nil {
		return nil
	}
	var status string
	err := q.QueryRow(ctx, `SELECT status FROM corporate_customer_cost_centers
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3`, businessID, corporateID, *costCenterID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errCorporateCostCenterNotFound()
	}
	if err != nil {
		return err
	}
	if status == CorporateChildArchived {
		return crmError(apperrors.Conflict("That cost centre is archived.", nil), CodeCorporateNestedScope)
	}
	return nil
}

func safeMembershipAudit(m CorporateMembership) map[string]any {
	return map[string]any{
		"membership_ref": m.ID, "corporate_ref": m.CorporateCustomerID, "contact_ref": m.ContactID,
		"location_ref": m.LocationID, "cost_center_ref": m.CostCenterID,
		"membership_role": m.MembershipRole, "status": m.Status, "version": m.Version,
	}
}
