package vendors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

type Actor struct{ StaffID, VendorID, SessionID, CorrelationID, CausationID string }

type Authorizer interface {
	Authorize(context.Context, string, string, *string) (bool, error)
}
type AuditEntry struct {
	ActorID, Action, AggregateType, AggregateID, VendorID, BranchID, Reason string
	Before, After                                                           any
}
type AuditRecorder interface {
	Record(context.Context, AuditEntry) error
}
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, string, any) error
}

type Service struct {
	assetValidator func(context.Context, string, string, string, string) error
	db             *pgxpool.Pool
	auth           Authorizer
	audit          AuditRecorder
	events         EventPublisher
	verification   VerificationAuthority
	bank           BankAuthority
	media          MediaAuthority
	stepUp         StepUpConsumer
	metrics        *Section7Metrics
}

type ListOptions struct {
	Limit          int
	Offset         int
	Status         string
	CapabilityType string
	BrandID        string
	Search         string
	Sort           string
	Direction      string
}

func (o ListOptions) bounded() ListOptions {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if o.Limit > 100 {
		o.Limit = 100
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	o.Status = strings.TrimSpace(o.Status)
	o.CapabilityType = strings.TrimSpace(o.CapabilityType)
	o.BrandID = strings.TrimSpace(o.BrandID)
	o.Search = strings.TrimSpace(o.Search)
	if strings.ToLower(o.Direction) != "asc" {
		o.Direction = "DESC"
	} else {
		o.Direction = "ASC"
	}
	return o
}

func NewService(db *pgxpool.Pool, auth Authorizer, audit AuditRecorder, events EventPublisher) *Service {
	return &Service{db: db, auth: auth, audit: audit, events: events, metrics: NewSection7Metrics()}
}

func (s *Service) SetOnboardingDependencies(verification VerificationAuthority, bank BankAuthority, media MediaAuthority, stepUp StepUpConsumer) {
	s.verification = verification
	s.bank = bank
	s.media = media
	s.stepUp = stepUp
}

func (s *Service) require(ctx context.Context, actor Actor, permission string, branchID *string) error {
	if actor.StaffID == "" || actor.VendorID == "" {
		return apperrors.Unauthorized("Authentication is required.", nil)
	}
	allowed, err := s.auth.Authorize(ctx, actor.StaffID, permission, branchID)
	if err != nil {
		return err
	}
	if !allowed {
		return apperrors.Forbidden("You do not have access to this action.", nil)
	}
	return nil
}
func conflict(err error) error {
	return apperrors.Conflict("This record changed since you last loaded it.", err)
}
func mapDB(err error, label string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound(label+" not found.", err)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return apperrors.Conflict(label+" already exists.", err)
	}
	if errors.As(err, &pe) && pe.Code == "23503" {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "reference", Message: "Referenced resource does not belong to this business."}})
	}
	return err
}
func newID(prefix string) string { return prefix + uuid.NewString() }

func scanBusiness(row pgx.Row) (Business, error) {
	var b Business
	err := row.Scan(&b.ID, &b.LegalName, &b.TradingName, &b.Status, &b.OwnerRef, &b.SupportEmail, &b.SupportPhone, &b.Currency, &b.Timezone, &b.CreatedAt, &b.UpdatedAt, &b.Version)
	return b, err
}

const businessCols = `id,legal_name,COALESCE(trading_name,''),status,owner_ref,COALESCE(support_email,''),COALESCE(support_phone,''),currency,timezone,created_at,updated_at,version`

func (s *Service) GetBusiness(ctx context.Context, actor Actor, id string) (Business, error) {
	if err := s.require(ctx, actor, "vendor.business.read", nil); err != nil {
		return Business{}, err
	}
	if id != actor.VendorID {
		return Business{}, apperrors.NotFound("Business not found.", nil)
	}
	b, err := scanBusiness(s.db.QueryRow(ctx, `SELECT `+businessCols+` FROM vendor_businesses WHERE id=$1`, actor.VendorID))
	return b, mapDB(err, "Business")
}
func (s *Service) ListBusinesses(ctx context.Context, actor Actor) ([]Business, error) {
	b, err := s.GetBusiness(ctx, actor, actor.VendorID)
	if err != nil {
		if apperrors.From(err).Code == apperrors.CodeNotFound {
			return []Business{}, nil
		}
		return nil, err
	}
	return []Business{b}, nil
}

func (s *Service) CreateBusiness(ctx context.Context, actor Actor, in Business) (Business, error) {
	if err := s.require(ctx, actor, "vendor.business.manage", nil); err != nil {
		return Business{}, err
	}
	in.ID = actor.VendorID
	in.OwnerRef = actor.StaffID
	in.Status = BusinessDraft
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Timezone = strings.TrimSpace(in.Timezone)
	if err := validateBusiness(in); err != nil {
		return Business{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Business{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBusiness(tx.QueryRow(ctx, `INSERT INTO vendor_businesses(id,legal_name,trading_name,status,owner_ref,support_email,support_phone,currency,timezone) VALUES($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9) RETURNING `+businessCols, in.ID, strings.TrimSpace(in.LegalName), strings.TrimSpace(in.TradingName), in.Status, in.OwnerRef, strings.TrimSpace(in.SupportEmail), strings.TrimSpace(in.SupportPhone), in.Currency, in.Timezone))
	if err != nil {
		return Business{}, mapDB(err, "Business")
	}
	if err = s.publish(ctx, tx, "vendor.business.created", "vendor_business", b.ID, b.Version, actor, b); err != nil {
		return Business{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Business{}, err
	}
	s.record(ctx, actor, "vendor.business.created", "vendor_business", b.ID, "", nil, b, "")
	return b, nil
}

type UpdateBusinessInput struct {
	LegalName, TradingName, SupportEmail, SupportPhone, Currency, Timezone string
	ExpectedVersion                                                        int64
}

func (s *Service) UpdateBusiness(ctx context.Context, actor Actor, id string, in UpdateBusinessInput) (Business, error) {
	if id != actor.VendorID {
		return Business{}, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.business.manage", nil); err != nil {
		return Business{}, err
	}
	before, err := s.GetBusinessUnchecked(ctx, actor.VendorID)
	if err != nil {
		return Business{}, mapDB(err, "Business")
	}
	candidate := before
	candidate.LegalName = in.LegalName
	candidate.TradingName = in.TradingName
	candidate.SupportEmail = in.SupportEmail
	candidate.SupportPhone = in.SupportPhone
	candidate.Currency = strings.ToUpper(in.Currency)
	candidate.Timezone = in.Timezone
	if err := validateBusiness(candidate); err != nil {
		return Business{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Business{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBusiness(tx.QueryRow(ctx, `UPDATE vendor_businesses SET legal_name=$1,trading_name=NULLIF($2,''),support_email=NULLIF($3,''),support_phone=NULLIF($4,''),currency=$5,timezone=$6,updated_at=now(),version=version+1 WHERE id=$7 AND version=$8 RETURNING `+businessCols, candidate.LegalName, candidate.TradingName, candidate.SupportEmail, candidate.SupportPhone, candidate.Currency, candidate.Timezone, actor.VendorID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Business{}, conflict(err)
	}
	if err != nil {
		return Business{}, err
	}
	if err = s.publish(ctx, tx, "vendor.business.updated", "vendor_business", b.ID, b.Version, actor, b); err != nil {
		return Business{}, err
	}
	if err = s.invalidateSubjectProjectionTx(ctx, tx, actor.VendorID, DimensionBusiness, "BUSINESS", actor.VendorID, "BUSINESS_PROFILE_CHANGED"); err != nil {
		return Business{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Business{}, err
	}
	s.record(ctx, actor, "vendor.business.updated", "vendor_business", b.ID, "", before, b, "")
	return b, nil
}
func (s *Service) GetBusinessUnchecked(ctx context.Context, id string) (Business, error) {
	return scanBusiness(s.db.QueryRow(ctx, `SELECT `+businessCols+` FROM vendor_businesses WHERE id=$1`, id))
}
func (s *Service) ChangeBusinessStatus(ctx context.Context, actor Actor, id string, to BusinessStatus, expected int64) (Business, error) {
	if id != actor.VendorID {
		return Business{}, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.business.manage", nil); err != nil {
		return Business{}, err
	}
	before, err := s.GetBusinessUnchecked(ctx, id)
	if err != nil {
		return Business{}, mapDB(err, "Business")
	}
	if !vendorBusinessTransitionAllowed(before.Status, to) {
		return Business{}, apperrors.Conflict("Only ACTIVE businesses can be paused and only PAUSED businesses can be resumed.", nil)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Business{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBusiness(tx.QueryRow(ctx, `UPDATE vendor_businesses SET status=$1,updated_at=now(),version=version+1 WHERE id=$2 AND version=$3 AND status=$4 RETURNING `+businessCols, to, id, expected, before.Status))
	if errors.Is(err, pgx.ErrNoRows) {
		return Business{}, conflict(err)
	}
	if err != nil {
		return Business{}, err
	}
	if err = s.publish(ctx, tx, "vendor.business.status_changed", "vendor_business", id, b.Version, actor, map[string]any{"status": to}); err != nil {
		return Business{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Business{}, err
	}
	s.record(ctx, actor, "vendor.business.status_changed", "vendor_business", id, "", before, b, "")
	return b, nil
}

const capabilityCols = `id,business_id,capability_type,status,COALESCE(status_reason,''),requested_at,updated_at,version`

func scanCapability(row pgx.Row) (Capability, error) {
	var c Capability
	err := row.Scan(&c.ID, &c.BusinessID, &c.CapabilityType, &c.Status, &c.StatusReason, &c.RequestedAt, &c.UpdatedAt, &c.Version)
	return c, err
}
func (s *Service) ListCapabilities(ctx context.Context, actor Actor, businessID string, options ...ListOptions) ([]Capability, error) {
	if businessID != actor.VendorID {
		return nil, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.capabilities.read", nil); err != nil {
		return nil, err
	}
	o := ListOptions{}.bounded()
	if len(options) > 0 {
		o = options[0].bounded()
	}
	rows, err := s.db.Query(ctx, `SELECT `+capabilityCols+` FROM vendor_capabilities WHERE business_id=$1 AND ($2='' OR status=$2) AND ($3='' OR capability_type=$3) ORDER BY capability_type LIMIT $4 OFFSET $5`, actor.VendorID, o.Status, o.CapabilityType, o.Limit+1, o.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Capability{}
	for rows.Next() {
		c, e := scanCapability(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) CreateCapability(ctx context.Context, actor Actor, businessID string, t CapabilityType) (Capability, error) {
	if businessID != actor.VendorID {
		return Capability{}, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.capabilities.manage", nil); err != nil {
		return Capability{}, err
	}
	if !IsSupportedCapability(t) {
		return Capability{}, validation(apperrors.FieldViolation{Field: "capability_type", Message: "Unsupported capability type."})
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Capability{}, err
	}
	defer tx.Rollback(ctx)
	c, err := scanCapability(tx.QueryRow(ctx, `INSERT INTO vendor_capabilities(id,business_id,capability_type,status) VALUES($1,$2,$3,'DRAFT') RETURNING `+capabilityCols, newID("vc_"), actor.VendorID, t))
	if err != nil {
		return Capability{}, mapDB(err, "Capability")
	}
	if err = linkBranchCapabilitiesTx(ctx, tx, actor.VendorID, "", c.ID); err != nil {
		return Capability{}, err
	}
	if err = s.publish(ctx, tx, "vendor.capability.changed", "vendor_capability", c.ID, c.Version, actor, c); err != nil {
		return Capability{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Capability{}, err
	}
	s.record(ctx, actor, "vendor.capability.created", "vendor_capability", c.ID, "", nil, c, "")
	return c, nil
}
func (s *Service) GetCapability(ctx context.Context, actor Actor, businessID, id string) (Capability, error) {
	if businessID != actor.VendorID {
		return Capability{}, apperrors.NotFound("Capability not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.capabilities.read", nil); err != nil {
		return Capability{}, err
	}
	c, err := scanCapability(s.db.QueryRow(ctx, `SELECT `+capabilityCols+` FROM vendor_capabilities WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	return c, mapDB(err, "Capability")
}
func (s *Service) ChangeCapabilityStatus(ctx context.Context, actor Actor, businessID, id string, to CapabilityStatus, expected int64) (Capability, error) {
	if businessID != actor.VendorID {
		return Capability{}, apperrors.NotFound("Capability not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.capabilities.manage", nil); err != nil {
		return Capability{}, err
	}
	before, err := scanCapability(s.db.QueryRow(ctx, `SELECT `+capabilityCols+` FROM vendor_capabilities WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	if err != nil {
		return Capability{}, mapDB(err, "Capability")
	}
	if !capabilityTransitionAllowed(before.Status, to) {
		return Capability{}, apperrors.Conflict("Only ACTIVE capabilities can be paused and only PAUSED capabilities can be resumed.", nil)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Capability{}, err
	}
	defer tx.Rollback(ctx)
	c, err := scanCapability(tx.QueryRow(ctx, `UPDATE vendor_capabilities SET status=$1,updated_at=now(),version=version+1 WHERE id=$2 AND business_id=$3 AND status=$4 AND version=$5 RETURNING `+capabilityCols, to, id, actor.VendorID, before.Status, expected))
	if errors.Is(err, pgx.ErrNoRows) {
		return Capability{}, conflict(err)
	}
	if err != nil {
		return Capability{}, err
	}
	if err = s.publish(ctx, tx, "vendor.capability.changed", "vendor_capability", id, c.Version, actor, c); err != nil {
		return Capability{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Capability{}, err
	}
	s.record(ctx, actor, "vendor.capability.status_changed", "vendor_capability", id, "", before, c, "")
	return c, nil
}

const brandCols = `id,business_id,display_name,description,media_refs,verification_state,created_at,updated_at,version`

func scanBrand(row pgx.Row) (Brand, error) {
	var b Brand
	var raw []byte
	err := row.Scan(&b.ID, &b.BusinessID, &b.DisplayName, &b.Description, &raw, &b.VerificationState, &b.CreatedAt, &b.UpdatedAt, &b.Version)
	if err == nil {
		err = json.Unmarshal(raw, &b.MediaRefs)
	}
	return b, err
}
func (s *Service) ListBrands(ctx context.Context, actor Actor, businessID string, options ...ListOptions) ([]Brand, error) {
	if businessID != actor.VendorID {
		return nil, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.brands.read", nil); err != nil {
		return nil, err
	}
	o := ListOptions{}.bounded()
	if len(options) > 0 {
		o = options[0].bounded()
	}
	order := "created_at"
	if o.Sort == "display_name" {
		order = "display_name"
	} else if o.Sort == "verification_state" {
		order = "verification_state"
	}
	query := `SELECT ` + brandCols + ` FROM vendor_brands WHERE business_id=$1 AND ($2='' OR verification_state=$2) AND ($3='' OR display_name ILIKE '%'||$3||'%') ORDER BY ` + order + ` ` + o.Direction + `, id ASC LIMIT $4 OFFSET $5`
	rows, err := s.db.Query(ctx, query, actor.VendorID, o.Status, o.Search, o.Limit+1, o.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Brand{}
	for rows.Next() {
		b, e := scanBrand(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Service) CreateBrand(ctx context.Context, actor Actor, businessID string, in Brand) (Brand, error) {
	if businessID != actor.VendorID {
		return Brand{}, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.brands.manage", nil); err != nil {
		return Brand{}, err
	}
	in.BusinessID = actor.VendorID
	in.VerificationState = VerificationNotStarted
	if in.MediaRefs == nil {
		in.MediaRefs = []MediaReference{}
	}
	if err := validateBrand(in); err != nil {
		return Brand{}, err
	}
	raw, _ := json.Marshal(in.MediaRefs)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Brand{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBrand(tx.QueryRow(ctx, `INSERT INTO vendor_brands(id,business_id,display_name,description,media_refs,verification_state) VALUES($1,$2,$3,$4,$5,'NOT_STARTED') RETURNING `+brandCols, newID("vb_"), actor.VendorID, strings.TrimSpace(in.DisplayName), in.Description, raw))
	if err != nil {
		return Brand{}, mapDB(err, "Brand")
	}
	if err = s.publish(ctx, tx, "vendor.brand.changed", "vendor_brand", b.ID, b.Version, actor, b); err != nil {
		return Brand{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Brand{}, err
	}
	s.record(ctx, actor, "vendor.brand.created", "vendor_brand", b.ID, "", nil, b, "")
	return b, nil
}
func (s *Service) GetBrand(ctx context.Context, actor Actor, businessID, id string) (Brand, error) {
	if businessID != actor.VendorID {
		return Brand{}, apperrors.NotFound("Brand not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.brands.read", nil); err != nil {
		return Brand{}, err
	}
	b, err := scanBrand(s.db.QueryRow(ctx, `SELECT `+brandCols+` FROM vendor_brands WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	return b, mapDB(err, "Brand")
}
func (s *Service) UpdateBrand(ctx context.Context, actor Actor, businessID, id string, in Brand, expected int64) (Brand, error) {
	if businessID != actor.VendorID {
		return Brand{}, apperrors.NotFound("Brand not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.brands.manage", nil); err != nil {
		return Brand{}, err
	}
	before, err := scanBrand(s.db.QueryRow(ctx, `SELECT `+brandCols+` FROM vendor_brands WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	if err != nil {
		return Brand{}, mapDB(err, "Brand")
	}
	in.VerificationState = before.VerificationState
	if in.MediaRefs == nil {
		in.MediaRefs = []MediaReference{}
	}
	if err := validateBrand(in); err != nil {
		return Brand{}, err
	}
	existing := map[string]bool{}
	for _, ref := range before.MediaRefs {
		existing[ref.MediaRef] = true
	}
	for _, ref := range in.MediaRefs {
		if existing[ref.MediaRef] {
			continue
		}
		if s.assetValidator == nil {
			return Brand{}, apperrors.Unavailable("Brand image validation is unavailable.", nil)
		}
		if err = s.assetValidator(ctx, businessID, "BRAND", id, ref.MediaRef); err != nil {
			return Brand{}, apperrors.BadRequest("Choose a validated image uploaded for this brand.", nil)
		}
	}

	raw, _ := json.Marshal(in.MediaRefs)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Brand{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBrand(tx.QueryRow(ctx, `UPDATE vendor_brands SET display_name=$1,description=$2,media_refs=$3,updated_at=now(),version=version+1 WHERE id=$4 AND business_id=$5 AND version=$6 RETURNING `+brandCols, in.DisplayName, in.Description, raw, id, actor.VendorID, expected))
	if errors.Is(err, pgx.ErrNoRows) {
		return Brand{}, conflict(err)
	}
	if err != nil {
		return Brand{}, err
	}
	if err = s.publish(ctx, tx, "vendor.brand.changed", "vendor_brand", id, b.Version, actor, b); err != nil {
		return Brand{}, err
	}
	if err = s.invalidateSubjectProjectionTx(ctx, tx, actor.VendorID, DimensionBrand, "BRAND", id, "BRAND_PROFILE_CHANGED"); err != nil {
		return Brand{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Brand{}, err
	}
	s.record(ctx, actor, "vendor.brand.updated", "vendor_brand", id, "", before, b, "")
	return b, nil
}

const branchCols = `id,business_id,COALESCE(brand_id,''),display_name,address_line1,COALESCE(address_line2,''),city,region,COALESCE(postal_code,''),country_code,latitude,longitude,timezone,status,created_at,updated_at,version`

func scanBranch(row pgx.Row) (Branch, error) {
	var b Branch
	err := row.Scan(&b.ID, &b.BusinessID, &b.BrandID, &b.DisplayName, &b.Address.Line1, &b.Address.Line2, &b.Address.City, &b.Address.Region, &b.Address.PostalCode, &b.Address.CountryCode, &b.Latitude, &b.Longitude, &b.Timezone, &b.Status, &b.CreatedAt, &b.UpdatedAt, &b.Version)
	return b, err
}
func (s *Service) BranchBelongsToVendor(ctx context.Context, vendorID, branchID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_branches WHERE id=$1 AND business_id=$2)`, branchID, vendorID).Scan(&ok)
	return ok, err
}
func (s *Service) ListBranches(ctx context.Context, actor Actor, businessID string, options ...ListOptions) ([]Branch, error) {
	if businessID != actor.VendorID {
		return nil, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.read", nil); err != nil {
		return nil, err
	}
	o := ListOptions{}.bounded()
	if len(options) > 0 {
		o = options[0].bounded()
	}
	order := "created_at"
	if o.Sort == "display_name" {
		order = "display_name"
	} else if o.Sort == "status" {
		order = "status"
	}
	query := `SELECT ` + branchCols + ` FROM vendor_branches WHERE business_id=$1 AND ($2='' OR status=$2) AND ($3='' OR COALESCE(brand_id,'')=$3) AND ($4='' OR display_name ILIKE '%'||$4||'%') ORDER BY ` + order + ` ` + o.Direction + `, id ASC LIMIT $5 OFFSET $6`
	rows, err := s.db.Query(ctx, query, actor.VendorID, o.Status, o.BrandID, o.Search, o.Limit+1, o.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Branch{}
	for rows.Next() {
		b, e := scanBranch(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Service) CreateBranch(ctx context.Context, actor Actor, businessID string, in Branch) (Branch, error) {
	if businessID != actor.VendorID {
		return Branch{}, apperrors.NotFound("Business not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.manage", nil); err != nil {
		return Branch{}, err
	}
	in.BusinessID = actor.VendorID
	in.Status = BranchDraft
	if err := validateBranch(in); err != nil {
		return Branch{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Branch{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBranch(tx.QueryRow(ctx, `INSERT INTO vendor_branches(id,business_id,brand_id,display_name,address_line1,address_line2,city,region,postal_code,country_code,latitude,longitude,timezone,status) VALUES($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),$7,$8,NULLIF($9,''),$10,$11,$12,$13,'DRAFT') RETURNING `+branchCols, newID("vbr_"), actor.VendorID, in.BrandID, in.DisplayName, in.Address.Line1, in.Address.Line2, in.Address.City, in.Address.Region, in.Address.PostalCode, in.Address.CountryCode, in.Latitude, in.Longitude, in.Timezone))
	if err != nil {
		return Branch{}, mapDB(err, "Branch")
	}
	if err = linkBranchCapabilitiesTx(ctx, tx, actor.VendorID, b.ID, ""); err != nil {
		return Branch{}, err
	}
	if err = s.publish(ctx, tx, "vendor.branch.created", "vendor_branch", b.ID, b.Version, actor, b); err != nil {
		return Branch{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Branch{}, err
	}
	s.record(ctx, actor, "vendor.branch.created", "vendor_branch", b.ID, b.ID, nil, b, "")
	return b, nil
}
func (s *Service) GetBranch(ctx context.Context, actor Actor, businessID, id string) (Branch, error) {
	if businessID != actor.VendorID {
		return Branch{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.read", &id); err != nil {
		return Branch{}, err
	}
	b, err := scanBranch(s.db.QueryRow(ctx, `SELECT `+branchCols+` FROM vendor_branches WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	return b, mapDB(err, "Branch")
}
func (s *Service) UpdateBranch(ctx context.Context, actor Actor, businessID, id string, in Branch, expected int64) (Branch, error) {
	if businessID != actor.VendorID {
		return Branch{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.manage", &id); err != nil {
		return Branch{}, err
	}
	before, err := scanBranch(s.db.QueryRow(ctx, `SELECT `+branchCols+` FROM vendor_branches WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	if err != nil {
		return Branch{}, mapDB(err, "Branch")
	}
	if err := validateBranch(in); err != nil {
		return Branch{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Branch{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBranch(tx.QueryRow(ctx, `UPDATE vendor_branches SET brand_id=NULLIF($1,''),display_name=$2,address_line1=$3,address_line2=NULLIF($4,''),city=$5,region=$6,postal_code=NULLIF($7,''),country_code=$8,latitude=$9,longitude=$10,timezone=$11,updated_at=now(),version=version+1 WHERE id=$12 AND business_id=$13 AND version=$14 RETURNING `+branchCols, in.BrandID, in.DisplayName, in.Address.Line1, in.Address.Line2, in.Address.City, in.Address.Region, in.Address.PostalCode, in.Address.CountryCode, in.Latitude, in.Longitude, in.Timezone, id, actor.VendorID, expected))
	if errors.Is(err, pgx.ErrNoRows) {
		return Branch{}, conflict(err)
	}
	if err != nil {
		return Branch{}, mapDB(err, "Branch")
	}
	if err = s.publish(ctx, tx, "vendor.branch.updated", "vendor_branch", id, b.Version, actor, b); err != nil {
		return Branch{}, err
	}
	if err = s.invalidateSubjectProjectionTx(ctx, tx, actor.VendorID, DimensionLocation, "BRANCH", id, "BRANCH_LOCATION_CHANGED"); err != nil {
		return Branch{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Branch{}, err
	}
	s.record(ctx, actor, "vendor.branch.updated", "vendor_branch", id, id, before, b, "")
	return b, nil
}
func (s *Service) ChangeBranchStatus(ctx context.Context, actor Actor, businessID, id string, to BranchStatus, expected int64) (Branch, error) {
	if businessID != actor.VendorID {
		return Branch{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.manage", &id); err != nil {
		return Branch{}, err
	}
	before, err := scanBranch(s.db.QueryRow(ctx, `SELECT `+branchCols+` FROM vendor_branches WHERE id=$1 AND business_id=$2`, id, actor.VendorID))
	if err != nil {
		return Branch{}, mapDB(err, "Branch")
	}
	if !branchTransitionAllowed(before.Status, to) {
		return Branch{}, apperrors.Conflict("Only ACTIVE branches can be paused and only PAUSED branches can be resumed.", nil)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Branch{}, err
	}
	defer tx.Rollback(ctx)
	b, err := scanBranch(tx.QueryRow(ctx, `UPDATE vendor_branches SET status=$1,updated_at=now(),version=version+1 WHERE id=$2 AND business_id=$3 AND status=$4 AND version=$5 RETURNING `+branchCols, to, id, actor.VendorID, before.Status, expected))
	if errors.Is(err, pgx.ErrNoRows) {
		return Branch{}, conflict(err)
	}
	if err != nil {
		return Branch{}, err
	}
	if err = s.publish(ctx, tx, "vendor.branch.status_changed", "vendor_branch", id, b.Version, actor, b); err != nil {
		return Branch{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Branch{}, err
	}
	s.record(ctx, actor, "vendor.branch.status_changed", "vendor_branch", id, id, before, b, "")
	return b, nil
}

func (s *Service) GetHours(ctx context.Context, actor Actor, businessID, branchID string) ([]OperatingWindow, error) {
	if businessID != actor.VendorID {
		return nil, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.read", &branchID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,weekday,is_closed,COALESCE(to_char(opens_at,'HH24:MI'),''),COALESCE(to_char(closes_at,'HH24:MI'),''),version FROM vendor_branch_hours WHERE business_id=$1 AND branch_id=$2 ORDER BY weekday,opens_at`, actor.VendorID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OperatingWindow{}
	for rows.Next() {
		var w OperatingWindow
		if err := rows.Scan(&w.ID, &w.Weekday, &w.IsClosed, &w.OpensAt, &w.ClosesAt, &w.Version); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (s *Service) PutHours(ctx context.Context, actor Actor, businessID, branchID string, windows []OperatingWindow, expected int64) ([]OperatingWindow, error) {
	if businessID != actor.VendorID {
		return nil, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branch.hours.manage", &branchID); err != nil {
		return nil, err
	}
	if err := validateHours(windows); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT version FROM vendor_branches WHERE id=$1 AND business_id=$2 FOR UPDATE`, branchID, actor.VendorID).Scan(&current); err != nil {
		return nil, mapDB(err, "Branch")
	}
	if current != expected {
		return nil, conflict(nil)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM vendor_branch_hours WHERE branch_id=$1 AND business_id=$2`, branchID, actor.VendorID); err != nil {
		return nil, err
	}
	for _, w := range windows {
		_, err = tx.Exec(ctx, `INSERT INTO vendor_branch_hours(id,business_id,branch_id,weekday,is_closed,opens_at,closes_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::time,NULLIF($7,'')::time)`, newID("vbh_"), actor.VendorID, branchID, w.Weekday, w.IsClosed, w.OpensAt, w.ClosesAt)
		if err != nil {
			return nil, err
		}
	}
	var version int64
	if err = tx.QueryRow(ctx, `UPDATE vendor_branches SET updated_at=now(),version=version+1 WHERE id=$1 RETURNING version`, branchID).Scan(&version); err != nil {
		return nil, err
	}
	if err = s.publish(ctx, tx, "vendor.branch.updated", "vendor_branch", branchID, version, actor, map[string]any{"hours": windows}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.record(ctx, actor, "vendor.branch.hours_replaced", "vendor_branch", branchID, branchID, nil, windows, "")
	return s.GetHours(ctx, actor, businessID, branchID)
}

func (s *Service) GetServiceArea(ctx context.Context, actor Actor, businessID, branchID string) (ServiceArea, error) {
	if businessID != actor.VendorID {
		return ServiceArea{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.read", &branchID); err != nil {
		return ServiceArea{}, err
	}
	var a ServiceArea
	err := s.db.QueryRow(ctx, `SELECT mode,center_latitude,center_longitude,radius_meters,version FROM vendor_branch_service_areas WHERE business_id=$1 AND branch_id=$2`, actor.VendorID, branchID).Scan(&a.Mode, &a.CenterLatitude, &a.CenterLongitude, &a.RadiusMeters, &a.Version)
	return a, mapDB(err, "Service area")
}
func (s *Service) PutServiceArea(ctx context.Context, actor Actor, businessID, branchID string, a ServiceArea, expected int64) (ServiceArea, error) {
	if businessID != actor.VendorID {
		return ServiceArea{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branch.settings.manage", &branchID); err != nil {
		return ServiceArea{}, err
	}
	if err := validateServiceArea(a); err != nil {
		return ServiceArea{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ServiceArea{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	var old int64
	err = tx.QueryRow(ctx, `SELECT true,version FROM vendor_branch_service_areas WHERE business_id=$1 AND branch_id=$2 FOR UPDATE`, actor.VendorID, branchID).Scan(&exists, &old)
	if errors.Is(err, pgx.ErrNoRows) {
		if expected != 0 {
			return ServiceArea{}, conflict(nil)
		}
		_, err = tx.Exec(ctx, `INSERT INTO vendor_branch_service_areas(branch_id,business_id,mode,center_latitude,center_longitude,radius_meters) SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS(SELECT 1 FROM vendor_branches WHERE id=$1 AND business_id=$2)`, branchID, actor.VendorID, a.Mode, a.CenterLatitude, a.CenterLongitude, a.RadiusMeters)
	} else if err == nil {
		if old != expected {
			return ServiceArea{}, conflict(nil)
		}
		_, err = tx.Exec(ctx, `UPDATE vendor_branch_service_areas SET mode=$1,center_latitude=$2,center_longitude=$3,radius_meters=$4,updated_at=now(),version=version+1 WHERE branch_id=$5`, a.Mode, a.CenterLatitude, a.CenterLongitude, a.RadiusMeters, branchID)
	}
	if err != nil {
		return ServiceArea{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT mode,center_latitude,center_longitude,radius_meters,version FROM vendor_branch_service_areas WHERE business_id=$1 AND branch_id=$2`, actor.VendorID, branchID).Scan(&a.Mode, &a.CenterLatitude, &a.CenterLongitude, &a.RadiusMeters, &a.Version); err != nil {
		return ServiceArea{}, mapDB(err, "Branch")
	}
	if err = s.publish(ctx, tx, "vendor.branch.updated", "vendor_branch_service_area", branchID, a.Version, actor, a); err != nil {
		return ServiceArea{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ServiceArea{}, err
	}
	s.record(ctx, actor, "vendor.branch.service_area_changed", "vendor_branch", branchID, branchID, nil, a, "")
	return a, nil
}

func (s *Service) GetSettings(ctx context.Context, actor Actor, businessID, branchID string) (OperationsSettings, error) {
	if businessID != actor.VendorID {
		return OperationsSettings{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branches.read", &branchID); err != nil {
		return OperationsSettings{}, err
	}
	var o OperationsSettings
	err := s.db.QueryRow(ctx, `SELECT settings_schema_version,accepting_new_orders,pickup_enabled,version FROM vendor_branch_settings WHERE business_id=$1 AND branch_id=$2`, actor.VendorID, branchID).Scan(&o.SchemaVersion, &o.AcceptingNewOrders, &o.PickupEnabled, &o.Version)
	return o, mapDB(err, "Operations settings")
}
func (s *Service) PutSettings(ctx context.Context, actor Actor, businessID, branchID string, o OperationsSettings, expected int64) (OperationsSettings, error) {
	if businessID != actor.VendorID {
		return OperationsSettings{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branch.settings.manage", &branchID); err != nil {
		return OperationsSettings{}, err
	}
	if err := validateSettings(o); err != nil {
		return OperationsSettings{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OperationsSettings{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO vendor_branch_settings(branch_id,business_id,settings_schema_version,accepting_new_orders,pickup_enabled) SELECT $1,$2,$3,$4,$5 WHERE $6=0 AND EXISTS(SELECT 1 FROM vendor_branches WHERE id=$1 AND business_id=$2) ON CONFLICT(branch_id) DO UPDATE SET settings_schema_version=EXCLUDED.settings_schema_version,accepting_new_orders=EXCLUDED.accepting_new_orders,pickup_enabled=EXCLUDED.pickup_enabled,updated_at=now(),version=vendor_branch_settings.version+1 WHERE vendor_branch_settings.business_id=$2 AND vendor_branch_settings.version=$6 RETURNING settings_schema_version,accepting_new_orders,pickup_enabled,version`, branchID, actor.VendorID, o.SchemaVersion, o.AcceptingNewOrders, o.PickupEnabled, expected).Scan(&o.SchemaVersion, &o.AcceptingNewOrders, &o.PickupEnabled, &o.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return OperationsSettings{}, conflict(nil)
	}
	if err != nil {
		return OperationsSettings{}, err
	}
	if err = s.publish(ctx, tx, "vendor.branch.updated", "vendor_branch_settings", branchID, o.Version, actor, o); err != nil {
		return OperationsSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OperationsSettings{}, err
	}
	s.record(ctx, actor, "vendor.branch.settings_changed", "vendor_branch", branchID, branchID, nil, o, "")
	return o, nil
}

const branchCapabilityCols = `bc.id,bc.business_id,bc.branch_id,bc.vendor_capability_id,bc.capability_type,bc.status,bc.created_at,bc.updated_at,bc.version,vb.status,b.status,c.status`

func scanBranchCapability(row pgx.Row) (BranchCapability, error) {
	var c BranchCapability
	var businessStatus BusinessStatus
	var bs BranchStatus
	var cs CapabilityStatus
	err := row.Scan(&c.ID, &c.BusinessID, &c.BranchID, &c.VendorCapabilityID, &c.CapabilityType, &c.Status, &c.CreatedAt, &c.UpdatedAt, &c.Version, &businessStatus, &bs, &cs)
	if err == nil {
		c.Effective, c.EffectiveReason = EffectiveCapabilityState(businessStatus, cs, bs, c.Status)
	}
	return c, err
}
func (s *Service) ListBranchCapabilities(ctx context.Context, actor Actor, businessID, branchID string) ([]BranchCapability, error) {
	if businessID != actor.VendorID {
		return nil, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.capabilities.read", &branchID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+branchCapabilityCols+` FROM vendor_branch_capabilities bc JOIN vendor_businesses vb ON vb.id=bc.business_id JOIN vendor_branches b ON b.id=bc.branch_id JOIN vendor_capabilities c ON c.id=bc.vendor_capability_id WHERE bc.business_id=$1 AND bc.branch_id=$2 ORDER BY bc.capability_type`, actor.VendorID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BranchCapability{}
	for rows.Next() {
		c, e := scanBranchCapability(rows)
		if e != nil {
			return nil, e
		}
		if c.Effective {
			c.Effective, c.EffectiveReason = s.verificationEffective(ctx, c)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) effective(ctx context.Context, c BranchCapability) (bool, string) {
	var b BusinessStatus
	var br BranchStatus
	var cap CapabilityStatus
	if err := s.db.QueryRow(ctx, `SELECT vb.status,vbr.status,vc.status FROM vendor_businesses vb JOIN vendor_branches vbr ON vbr.business_id=vb.id JOIN vendor_capabilities vc ON vc.business_id=vb.id WHERE vb.id=$1 AND vbr.id=$2 AND vc.id=$3`, c.BusinessID, c.BranchID, c.VendorCapabilityID).Scan(&b, &br, &cap); err != nil {
		return false, "STATE_UNAVAILABLE"
	}
	effective, reason := EffectiveCapabilityState(b, cap, br, c.Status)
	if !effective {
		return effective, reason
	}
	return s.verificationEffective(ctx, c)
}

func (s *Service) verificationEffective(ctx context.Context, c BranchCapability) (bool, string) {
	var onboardingStatus string
	err := s.db.QueryRow(ctx, `SELECT status FROM vendor_onboarding_profiles WHERE business_id=$1`, c.BusinessID).Scan(&onboardingStatus)
	if errors.Is(err, pgx.ErrNoRows) || onboardingStatus == "NOT_STARTED" {
		return true, "ACTIVE"
	}
	if err != nil {
		return false, "VERIFICATION_STATE_UNAVAILABLE"
	}
	var blocked bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM vendor_verification_projections p
		WHERE p.business_id=$1 AND (
			(p.dimension IN ('IDENTITY','BUSINESS') AND p.subject_type IN ('BUSINESS','REPRESENTATIVE')) OR
			(p.dimension='LOCATION' AND p.subject_type='BRANCH' AND p.subject_ref=$2) OR
			(p.dimension='FOOD_KITCHEN' AND p.subject_type='BRANCH_CAPABILITY' AND p.subject_ref=$3 AND $4 IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN'))
		) AND (p.status<>'VERIFIED' OR (p.expires_at IS NOT NULL AND p.expires_at<=now()))
	) OR NOT EXISTS(SELECT 1 FROM vendor_verification_projections WHERE business_id=$1 AND dimension='IDENTITY' AND status='VERIFIED' AND (expires_at IS NULL OR expires_at>now()))
	  OR NOT EXISTS(SELECT 1 FROM vendor_verification_projections WHERE business_id=$1 AND dimension='BUSINESS' AND status='VERIFIED' AND (expires_at IS NULL OR expires_at>now()))
	  OR NOT EXISTS(SELECT 1 FROM vendor_verification_projections WHERE business_id=$1 AND dimension='LOCATION' AND subject_type='BRANCH' AND subject_ref=$2 AND status='VERIFIED' AND (expires_at IS NULL OR expires_at>now()))
	  OR ($4 IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN') AND NOT EXISTS(SELECT 1 FROM vendor_verification_projections WHERE business_id=$1 AND dimension='FOOD_KITCHEN' AND subject_type='BRANCH_CAPABILITY' AND subject_ref=$3 AND status='VERIFIED' AND (expires_at IS NULL OR expires_at>now())))`, c.BusinessID, c.BranchID, c.ID, c.CapabilityType).Scan(&blocked)
	if err != nil {
		return false, "VERIFICATION_STATE_UNAVAILABLE"
	}
	if blocked {
		return false, "VERIFICATION_REQUIREMENTS_NOT_SATISFIED"
	}
	return true, "ACTIVE"
}
func (s *Service) PutBranchCapability(ctx context.Context, actor Actor, businessID, branchID, capabilityID string, expected int64) (BranchCapability, error) {
	if businessID != actor.VendorID {
		return BranchCapability{}, apperrors.NotFound("Branch not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branch.capabilities.manage", &branchID); err != nil {
		return BranchCapability{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BranchCapability{}, err
	}
	defer tx.Rollback(ctx)
	var c BranchCapability
	var bs BranchStatus
	var cs CapabilityStatus
	err = tx.QueryRow(ctx, `INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status) SELECT $1,$2,$3,id,capability_type,'DRAFT' FROM vendor_capabilities WHERE id=$4 AND business_id=$2 ON CONFLICT(branch_id,capability_type) DO UPDATE SET vendor_capability_id=EXCLUDED.vendor_capability_id,updated_at=now(),version=vendor_branch_capabilities.version+1 WHERE vendor_branch_capabilities.version=$5 RETURNING id,business_id,branch_id,vendor_capability_id,capability_type,status,created_at,updated_at,version,(SELECT status FROM vendor_branches WHERE id=$3),(SELECT status FROM vendor_capabilities WHERE id=$4)`, newID("vbc_"), actor.VendorID, branchID, capabilityID, expected).Scan(&c.ID, &c.BusinessID, &c.BranchID, &c.VendorCapabilityID, &c.CapabilityType, &c.Status, &c.CreatedAt, &c.UpdatedAt, &c.Version, &bs, &cs)
	if errors.Is(err, pgx.ErrNoRows) {
		return BranchCapability{}, conflict(err)
	}
	if err != nil {
		return BranchCapability{}, mapDB(err, "Branch capability")
	}
	var businessStatus BusinessStatus
	if err = tx.QueryRow(ctx, `SELECT status FROM vendor_businesses WHERE id=$1`, actor.VendorID).Scan(&businessStatus); err != nil {
		return BranchCapability{}, err
	}
	c.Effective, c.EffectiveReason = EffectiveCapabilityState(businessStatus, cs, bs, c.Status)
	if err = s.publish(ctx, tx, "vendor.branch.capability_changed", "vendor_branch_capability", c.ID, c.Version, actor, c); err != nil {
		return BranchCapability{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BranchCapability{}, err
	}
	s.record(ctx, actor, "vendor.branch.capability_changed", "vendor_branch_capability", c.ID, branchID, nil, c, "")
	return c, nil
}

func (s *Service) ChangeBranchCapabilityStatus(ctx context.Context, actor Actor, businessID, branchID, capabilityID string, to BranchCapabilityStatus, expected int64) (BranchCapability, error) {
	if businessID != actor.VendorID {
		return BranchCapability{}, apperrors.NotFound("Branch capability not found.", nil)
	}
	if err := s.require(ctx, actor, "vendor.branch.capabilities.manage", &branchID); err != nil {
		return BranchCapability{}, err
	}
	var before BranchCapability
	var businessStatus BusinessStatus
	var branchStatus BranchStatus
	var capabilityStatus CapabilityStatus
	err := s.db.QueryRow(ctx, `SELECT bc.id,bc.business_id,bc.branch_id,bc.vendor_capability_id,bc.capability_type,bc.status,bc.created_at,bc.updated_at,bc.version,vb.status,b.status,c.status FROM vendor_branch_capabilities bc JOIN vendor_businesses vb ON vb.id=bc.business_id JOIN vendor_branches b ON b.id=bc.branch_id JOIN vendor_capabilities c ON c.id=bc.vendor_capability_id WHERE bc.business_id=$1 AND bc.branch_id=$2 AND bc.vendor_capability_id=$3`, actor.VendorID, branchID, capabilityID).Scan(&before.ID, &before.BusinessID, &before.BranchID, &before.VendorCapabilityID, &before.CapabilityType, &before.Status, &before.CreatedAt, &before.UpdatedAt, &before.Version, &businessStatus, &branchStatus, &capabilityStatus)
	if err != nil {
		return BranchCapability{}, mapDB(err, "Branch capability")
	}
	if !branchCapabilityTransitionAllowed(before.Status, to) {
		return BranchCapability{}, apperrors.Conflict("Only ACTIVE branch capabilities can be paused and only PAUSED branch capabilities can be resumed.", nil)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BranchCapability{}, err
	}
	defer tx.Rollback(ctx)
	var out BranchCapability
	err = tx.QueryRow(ctx, `UPDATE vendor_branch_capabilities SET status=$1,updated_at=now(),version=version+1 WHERE id=$2 AND business_id=$3 AND branch_id=$4 AND status=$5 AND version=$6 RETURNING id,business_id,branch_id,vendor_capability_id,capability_type,status,created_at,updated_at,version`, to, before.ID, actor.VendorID, branchID, before.Status, expected).Scan(&out.ID, &out.BusinessID, &out.BranchID, &out.VendorCapabilityID, &out.CapabilityType, &out.Status, &out.CreatedAt, &out.UpdatedAt, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return BranchCapability{}, conflict(err)
	}
	if err != nil {
		return BranchCapability{}, err
	}
	out.Effective, out.EffectiveReason = EffectiveCapabilityState(businessStatus, capabilityStatus, branchStatus, out.Status)
	if err = s.publish(ctx, tx, "vendor.branch.capability_changed", "vendor_branch_capability", out.ID, out.Version, actor, out); err != nil {
		return BranchCapability{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BranchCapability{}, err
	}
	s.record(ctx, actor, "vendor.branch.capability.status_changed", "vendor_branch_capability", out.ID, branchID, before, out, "")
	return out, nil
}

func (s *Service) publish(ctx context.Context, tx pgx.Tx, event, aggregate, id string, version int64, actor Actor, payload any) error {
	if s.events == nil {
		return nil
	}
	return s.events.Publish(ctx, tx, event, aggregate, id, version, actor.VendorID, actor.CorrelationID, actor.CausationID, payload)
}
func (s *Service) record(ctx context.Context, actor Actor, action, aggregate, id, branch string, before, after any, reason string) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, AuditEntry{ActorID: actor.StaffID, Action: action, AggregateType: aggregate, AggregateID: id, VendorID: actor.VendorID, BranchID: branch, Before: before, After: after, Reason: reason})
}

func (s *Service) UseAssetValidator(fn func(context.Context, string, string, string, string) error) {
	s.assetValidator = fn
}

// linkBranchCapabilitiesTx offers a business's types (restaurant, shop, ...)
// at its branches as DRAFT branch capabilities, so a merchant who chose
// "Restaurant" can build a menu at a branch straight away. Publishing still
// requires the business, branch and branch capability to be ACTIVE. Pass a
// branchID to link every capability to that branch, or a capabilityID to link
// that capability to every branch. Existing rows are left untouched.
func linkBranchCapabilitiesTx(ctx context.Context, tx pgx.Tx, businessID, branchID, capabilityID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status)
		SELECT 'vbc_'||gen_random_uuid()::text, c.business_id, b.id, c.id, c.capability_type, 'DRAFT'
		FROM vendor_capabilities c JOIN vendor_branches b ON b.business_id=c.business_id
		WHERE c.business_id=$1 AND ($2='' OR b.id=$2) AND ($3='' OR c.id=$3)
		  AND c.status<>'REJECTED' AND b.status<>'CLOSED'
		ON CONFLICT (branch_id,capability_type) DO NOTHING`, businessID, branchID, capabilityID)
	return err
}
