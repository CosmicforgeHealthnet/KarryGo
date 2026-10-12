package promotions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

const (
	PermRead   = "vendor.promotions.read"
	PermManage = "vendor.promotions.manage"
)

var (
	ErrNotFound     = errors.New("promotion campaign not found")
	ErrForbidden    = errors.New("promotion campaign forbidden")
	ErrConflict     = errors.New("promotion campaign version conflict")
	ErrCouponExists = errors.New("coupon code is already in use")
)

type Service struct {
	db           *pgxpool.Pool
	authorizer   *production.Service
	couponSecret []byte
	enabled      bool
	now          func() time.Time
	events       *vendorevents.DomainPublisher
}

func NewService(db *pgxpool.Pool, authorizer *production.Service, couponSecret []byte, enabled bool) *Service {
	return &Service{db: db, authorizer: authorizer, couponSecret: append([]byte(nil), couponSecret...), enabled: enabled, now: time.Now}
}

func (s *Service) SetEventPublisher(publisher *vendorevents.DomainPublisher) {
	if s != nil {
		s.events = publisher
	}
}

func DigestCoupon(secret []byte, code string) string {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if len(secret) == 0 || normalized == "" {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

type CampaignInput struct {
	Name                  string              `json:"name"`
	Type                  Type                `json:"promotion_type"`
	Currency              string              `json:"currency"`
	PercentageBPS         int                 `json:"percentage_bps"`
	FixedMinor            int64               `json:"fixed_amount_minor"`
	MaxDiscountMinor      *int64              `json:"max_discount_minor,omitempty"`
	MinimumOrderMinor     int64               `json:"minimum_order_minor"`
	StartsAt              time.Time           `json:"starts_at"`
	EndsAt                time.Time           `json:"ends_at"`
	Sponsor               Sponsor             `json:"funding_sponsor"`
	VendorFundingBPS      int                 `json:"vendor_funding_bps"`
	CouponRequired        bool                `json:"coupon_required"`
	GlobalUsageLimit      *int64              `json:"global_usage_limit,omitempty"`
	PerCustomerUsageLimit *int64              `json:"per_customer_usage_limit,omitempty"`
	Combinability         Combinability       `json:"combinability"`
	Priority              int                 `json:"priority"`
	Scopes                []Scope             `json:"scopes,omitempty"`
	BundleRequirements    []BundleRequirement `json:"bundle_requirements,omitempty"`
	QuantityTiers         []QuantityTier      `json:"quantity_tiers,omitempty"`
}

func (s *Service) authorize(ctx context.Context, actor production.Actor, permission string) (production.Scope, error) {
	if s == nil || !s.enabled || s.db == nil || s.authorizer == nil || actor.VendorID == "" {
		return production.Scope{}, ErrForbidden
	}
	scope, err := s.authorizer.Authorize(ctx, actor, permission)
	if err != nil || scope.Empty() {
		return production.Scope{}, ErrForbidden
	}
	return scope, nil
}

func (s *Service) Create(ctx context.Context, actor production.Actor, input CampaignInput) (Campaign, error) {
	scope, err := s.authorize(ctx, actor, PermManage)
	if err != nil {
		return Campaign{}, err
	}
	if input.Sponsor != Vendor {
		return Campaign{}, ErrForbidden
	}
	campaign := Campaign{ID: "promo_" + uuid.NewString(), BusinessID: actor.VendorID, Name: strings.TrimSpace(input.Name), Type: input.Type,
		Status: Draft, Currency: strings.ToUpper(strings.TrimSpace(input.Currency)), PercentageBPS: input.PercentageBPS,
		FixedMinor: input.FixedMinor, MaxDiscountMinor: input.MaxDiscountMinor, MinimumOrderMinor: input.MinimumOrderMinor,
		StartsAt: input.StartsAt.UTC(), EndsAt: input.EndsAt.UTC(), Sponsor: input.Sponsor, VendorFundingBPS: 10000,
		CouponRequired: input.CouponRequired, GlobalUsageLimit: input.GlobalUsageLimit, PerCustomerUsageLimit: input.PerCustomerUsageLimit,
		Combinability: input.Combinability, Priority: input.Priority, Version: 1, Scopes: input.Scopes,
		BundleRequirements: input.BundleRequirements, QuantityTiers: input.QuantityTiers}
	if campaign.Combinability == "" {
		campaign.Combinability = Exclusive
	}
	for _, target := range campaign.Scopes {
		if target.Type == "BRANCH" && !scope.Covers(target.Ref) {
			return Campaign{}, ErrForbidden
		}
	}
	if !scope.Global {
		hasBranch := false
		for _, target := range campaign.Scopes {
			if target.Type == "BRANCH" {
				hasBranch = true
			}
		}
		if !hasBranch {
			return Campaign{}, ErrForbidden
		}
	}
	if err = campaign.Validate(); err != nil {
		return Campaign{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO commerce_promotion_campaigns
		(id,business_id,name,promotion_type,status,currency,percentage_bps,fixed_amount_minor,max_discount_minor,minimum_order_minor,
		 starts_at,ends_at,funding_sponsor,vendor_funding_bps,coupon_required,global_usage_limit,per_customer_usage_limit,combinability,priority,version,created_by_staff_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,1,$20)`,
		campaign.ID, campaign.BusinessID, campaign.Name, campaign.Type, campaign.Status, campaign.Currency, campaign.PercentageBPS, campaign.FixedMinor,
		campaign.MaxDiscountMinor, campaign.MinimumOrderMinor, campaign.StartsAt, campaign.EndsAt, campaign.Sponsor, campaign.VendorFundingBPS,
		campaign.CouponRequired, campaign.GlobalUsageLimit, campaign.PerCustomerUsageLimit, campaign.Combinability, campaign.Priority, actor.StaffID)
	if err != nil {
		return Campaign{}, err
	}
	if err = insertDefinition(ctx, tx, campaign); err != nil {
		return Campaign{}, err
	}
	if err = s.events.Publish(ctx, tx, "promotion.created", "promotion_campaign", campaign.ID, campaign.Version,
		campaign.BusinessID, actor.CorrelationID, actor.CausationID, campaign); err != nil {
		return Campaign{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Campaign{}, err
	}
	return campaign, nil
}

// Update replaces the mutable campaign definition under optimistic
// concurrency. Existing CheckoutQuote and order snapshots remain immutable;
// a checkout racing this mutation fails its version check at reservation.
func (s *Service) Update(ctx context.Context, actor production.Actor, id string, input CampaignInput, expectedVersion int64) (Campaign, error) {
	scope, err := s.authorize(ctx, actor, PermManage)
	if err != nil {
		return Campaign{}, err
	}
	if input.Sponsor != Vendor {
		return Campaign{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanCampaign(tx.QueryRow(ctx, `SELECT `+campaignColumns+` FROM commerce_promotion_campaigns WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	if err != nil {
		return Campaign{}, err
	}
	if current.Version != expectedVersion {
		return Campaign{}, ErrConflict
	}
	current, err = s.loadChildren(ctx, tx, current)
	if err != nil {
		return Campaign{}, err
	}
	if current.Sponsor != Vendor || !canManageCampaign(scope, current) {
		return Campaign{}, ErrForbidden
	}
	updated := Campaign{ID: current.ID, BusinessID: current.BusinessID, Name: strings.TrimSpace(input.Name), Type: input.Type,
		Status: current.Status, Currency: strings.ToUpper(strings.TrimSpace(input.Currency)), PercentageBPS: input.PercentageBPS,
		FixedMinor: input.FixedMinor, MaxDiscountMinor: input.MaxDiscountMinor, MinimumOrderMinor: input.MinimumOrderMinor,
		StartsAt: input.StartsAt.UTC(), EndsAt: input.EndsAt.UTC(), Sponsor: Vendor, VendorFundingBPS: 10000,
		CouponRequired: input.CouponRequired, GlobalUsageLimit: input.GlobalUsageLimit, PerCustomerUsageLimit: input.PerCustomerUsageLimit,
		Combinability: input.Combinability, Priority: input.Priority, Version: current.Version + 1, Scopes: input.Scopes,
		BundleRequirements: input.BundleRequirements, QuantityTiers: input.QuantityTiers}
	if updated.Combinability == "" {
		updated.Combinability = Exclusive
	}
	for _, target := range updated.Scopes {
		if target.Type == "BRANCH" && !scope.Covers(target.Ref) {
			return Campaign{}, ErrForbidden
		}
	}
	if !scope.Global {
		hasCoveredBranch := false
		for _, target := range updated.Scopes {
			hasCoveredBranch = hasCoveredBranch || (target.Type == "BRANCH" && scope.Covers(target.Ref))
		}
		if !hasCoveredBranch {
			return Campaign{}, ErrForbidden
		}
	}
	if err = updated.Validate(); err != nil {
		return Campaign{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_promotion_campaigns SET
		name=$1,promotion_type=$2,currency=$3,percentage_bps=$4,fixed_amount_minor=$5,max_discount_minor=$6,
		minimum_order_minor=$7,starts_at=$8,ends_at=$9,coupon_required=$10,global_usage_limit=$11,
		per_customer_usage_limit=$12,combinability=$13,priority=$14,version=$15,updated_at=now()
		WHERE id=$16 AND business_id=$17 AND version=$18`, updated.Name, updated.Type, updated.Currency, updated.PercentageBPS,
		updated.FixedMinor, updated.MaxDiscountMinor, updated.MinimumOrderMinor, updated.StartsAt, updated.EndsAt,
		updated.CouponRequired, updated.GlobalUsageLimit, updated.PerCustomerUsageLimit, updated.Combinability, updated.Priority,
		updated.Version, updated.ID, updated.BusinessID, current.Version)
	if err != nil {
		return Campaign{}, err
	}
	for _, table := range []string{"commerce_promotion_scopes", "commerce_promotion_bundle_requirements", "commerce_promotion_quantity_tiers"} {
		if _, err = tx.Exec(ctx, `DELETE FROM `+table+` WHERE campaign_id=$1 AND business_id=$2`, updated.ID, updated.BusinessID); err != nil {
			return Campaign{}, err
		}
	}
	if err = insertDefinition(ctx, tx, updated); err != nil {
		return Campaign{}, err
	}
	if err = s.events.Publish(ctx, tx, "promotion.updated", "promotion_campaign", updated.ID, updated.Version,
		updated.BusinessID, actor.CorrelationID, actor.CausationID, updated); err != nil {
		return Campaign{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Campaign{}, err
	}
	return updated, nil
}

func insertDefinition(ctx context.Context, tx pgx.Tx, c Campaign) error {
	for _, target := range c.Scopes {
		if strings.TrimSpace(target.Ref) == "" {
			return ErrInvalidCampaign
		}
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_promotion_scopes(id,campaign_id,business_id,scope_type,scope_ref) VALUES($1,$2,$3,$4,$5)`, "promoscope_"+uuid.NewString(), c.ID, c.BusinessID, target.Type, target.Ref); err != nil {
			return err
		}
	}
	for _, requirement := range c.BundleRequirements {
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_promotion_bundle_requirements(id,campaign_id,business_id,item_ref,variant_ref,minimum_quantity) VALUES($1,$2,$3,$4,$5,$6)`, "promobundle_"+uuid.NewString(), c.ID, c.BusinessID, requirement.ItemRef, requirement.VariantRef, requirement.MinimumQuantity); err != nil {
			return err
		}
	}
	for _, tier := range c.QuantityTiers {
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_promotion_quantity_tiers(id,campaign_id,business_id,minimum_quantity,maximum_quantity,percentage_bps,fixed_amount_minor) VALUES($1,$2,$3,$4,$5,$6,$7)`, "promotier_"+uuid.NewString(), c.ID, c.BusinessID, tier.MinimumQuantity, tier.MaximumQuantity, tier.PercentageBPS, tier.FixedMinor); err != nil {
			return err
		}
	}
	return nil
}

const campaignColumns = `id,business_id,name,promotion_type,status,currency,percentage_bps,fixed_amount_minor,max_discount_minor,
	minimum_order_minor,starts_at,ends_at,funding_sponsor,vendor_funding_bps,platform_approval_ref,coupon_required,
	global_usage_limit,per_customer_usage_limit,combinability,priority,version`

func scanCampaign(row pgx.Row) (Campaign, error) {
	var c Campaign
	err := row.Scan(&c.ID, &c.BusinessID, &c.Name, &c.Type, &c.Status, &c.Currency, &c.PercentageBPS, &c.FixedMinor, &c.MaxDiscountMinor,
		&c.MinimumOrderMinor, &c.StartsAt, &c.EndsAt, &c.Sponsor, &c.VendorFundingBPS, &c.PlatformApprovalRef, &c.CouponRequired,
		&c.GlobalUsageLimit, &c.PerCustomerUsageLimit, &c.Combinability, &c.Priority, &c.Version)
	return c, err
}

func (s *Service) Get(ctx context.Context, actor production.Actor, id string) (Campaign, error) {
	scope, err := s.authorize(ctx, actor, PermRead)
	if err != nil {
		return Campaign{}, err
	}
	c, err := s.load(ctx, s.db, actor.VendorID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	if err != nil {
		return Campaign{}, err
	}
	if !scope.Global {
		allowed := false
		for _, target := range c.Scopes {
			if target.Type == "BRANCH" && scope.Covers(target.Ref) {
				allowed = true
			}
		}
		if !allowed {
			return Campaign{}, ErrForbidden
		}
	}
	return c, nil
}

func (s *Service) List(ctx context.Context, actor production.Actor) ([]Campaign, error) {
	scope, err := s.authorize(ctx, actor, PermRead)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+campaignColumns+` FROM commerce_promotion_campaigns WHERE business_id=$1 ORDER BY priority,id`, actor.VendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, scanErr := scanCampaign(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if c, scanErr = s.loadChildren(ctx, s.db, c); scanErr != nil {
			return nil, scanErr
		}
		if canReadCampaign(scope, c) {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

func (s *Service) SetStatus(ctx context.Context, actor production.Actor, id string, status Status, expectedVersion int64) (Campaign, error) {
	scope, err := s.authorize(ctx, actor, PermManage)
	if err != nil {
		return Campaign{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := s.load(ctx, tx, actor.VendorID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	if err != nil {
		return Campaign{}, err
	}
	if current.Version != expectedVersion || !CanTransition(current.Status, status) {
		return Campaign{}, ErrConflict
	}
	if !canManageCampaign(scope, current) {
		return Campaign{}, ErrForbidden
	}
	row := tx.QueryRow(ctx, `UPDATE commerce_promotion_campaigns SET status=$1,version=version+1,updated_at=now() WHERE id=$2 AND business_id=$3 AND version=$4 RETURNING `+campaignColumns, status, id, actor.VendorID, expectedVersion)
	updated, err := scanCampaign(row)
	if err != nil {
		return Campaign{}, ErrConflict
	}
	updated, err = s.loadChildren(ctx, tx, updated)
	if err != nil {
		return Campaign{}, err
	}
	if err = s.events.Publish(ctx, tx, "promotion.updated", "promotion_campaign", updated.ID, updated.Version,
		updated.BusinessID, actor.CorrelationID, actor.CausationID, map[string]any{"status": updated.Status}); err != nil {
		return Campaign{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Campaign{}, err
	}
	return updated, nil
}

func (s *Service) CreateCoupon(ctx context.Context, actor production.Actor, campaignID, code string, usageLimit, customerLimit *int64) (string, error) {
	scope, err := s.authorize(ctx, actor, PermManage)
	if err != nil {
		return "", err
	}
	digest := DigestCoupon(s.couponSecret, code)
	if digest == "" {
		return "", ErrCouponInvalid
	}
	c, err := s.Get(ctx, actor, campaignID)
	if err != nil {
		return "", err
	}
	if !canManageCampaign(scope, c) {
		return "", ErrForbidden
	}
	if c.Status == Ended || c.Status == Archived || (usageLimit != nil && *usageLimit < 1) || (customerLimit != nil && *customerLimit < 1) {
		return "", ErrCouponInvalid
	}
	id := "coupon_" + uuid.NewString()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_promotion_coupons(id,campaign_id,business_id,code_digest,usage_limit,per_customer_usage_limit) VALUES($1,$2,$3,$4,$5,$6)`, id, c.ID, c.BusinessID, digest, usageLimit, customerLimit); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrCouponExists
		}
		return "", err
	}
	if err = s.events.Publish(ctx, tx, "promotion.coupon.created", "promotion_campaign", c.ID, c.Version,
		c.BusinessID, actor.CorrelationID, actor.CausationID, map[string]any{"coupon_id": id, "usage_limit": usageLimit, "per_customer_usage_limit": customerLimit}); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) Usage(ctx context.Context, actor production.Actor, campaignID string) (int64, error) {
	if _, err := s.Get(ctx, actor, campaignID); err != nil {
		return 0, err
	}
	var count int64
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM commerce_promotion_redemptions WHERE business_id=$1 AND campaign_id=$2 AND status IN ('RESERVED','CONSUMED')`, actor.VendorID, campaignID).Scan(&count)
	return count, err
}

// MaintainLifecycle advances display state and releases abandoned quote
// reservations. Eligibility still checks timestamps directly, so a late job
// cannot make an expired campaign applicable.
func (s *Service) MaintainLifecycle(ctx context.Context, limit int) (int64, error) {
	if s == nil || !s.enabled || s.db == nil {
		return 0, nil
	}
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := s.now().UTC()
	ended, err := tx.Exec(ctx, `UPDATE commerce_promotion_campaigns SET status='ENDED',version=version+1,updated_at=$1
		WHERE status IN ('ACTIVE','SCHEDULED','PAUSED') AND ends_at<=$1`, now)
	if err != nil {
		return 0, err
	}
	activated, err := tx.Exec(ctx, `UPDATE commerce_promotion_campaigns SET status='ACTIVE',version=version+1,updated_at=$1
		WHERE status='SCHEDULED' AND starts_at<=$1 AND ends_at>$1`, now)
	if err != nil {
		return 0, err
	}
	released, err := tx.Exec(ctx, `WITH due AS (
		SELECT r.id FROM commerce_promotion_redemptions r
		JOIN commerce_quotes q ON q.id=r.quote_id
		WHERE r.status='RESERVED' AND q.expires_at<=$1
		ORDER BY q.expires_at,r.id LIMIT $2 FOR UPDATE OF r SKIP LOCKED)
		UPDATE commerce_promotion_redemptions r SET status='RELEASED',updated_at=$1 FROM due WHERE r.id=due.id`, now, limit)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return ended.RowsAffected() + activated.RowsAffected() + released.RowsAffected(), nil
}

type queryRows interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) load(ctx context.Context, q queryRows, businessID, id string) (Campaign, error) {
	c, err := scanCampaign(q.QueryRow(ctx, `SELECT `+campaignColumns+` FROM commerce_promotion_campaigns WHERE business_id=$1 AND id=$2`, businessID, id))
	if err != nil {
		return Campaign{}, err
	}
	return s.loadChildren(ctx, q, c)
}
func (s *Service) loadChildren(ctx context.Context, q queryRows, c Campaign) (Campaign, error) {
	rows, err := q.Query(ctx, `SELECT scope_type,scope_ref FROM commerce_promotion_scopes WHERE campaign_id=$1 ORDER BY scope_type,scope_ref`, c.ID)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var v Scope
		if err = rows.Scan(&v.Type, &v.Ref); err != nil {
			rows.Close()
			return c, err
		}
		c.Scopes = append(c.Scopes, v)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT item_ref,variant_ref,minimum_quantity FROM commerce_promotion_bundle_requirements WHERE campaign_id=$1 ORDER BY item_ref,variant_ref`, c.ID)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var v BundleRequirement
		if err = rows.Scan(&v.ItemRef, &v.VariantRef, &v.MinimumQuantity); err != nil {
			rows.Close()
			return c, err
		}
		c.BundleRequirements = append(c.BundleRequirements, v)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT minimum_quantity,maximum_quantity,percentage_bps,fixed_amount_minor FROM commerce_promotion_quantity_tiers WHERE campaign_id=$1 ORDER BY minimum_quantity`, c.ID)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var v QuantityTier
		if err = rows.Scan(&v.MinimumQuantity, &v.MaximumQuantity, &v.PercentageBPS, &v.FixedMinor); err != nil {
			rows.Close()
			return c, err
		}
		c.QuantityTiers = append(c.QuantityTiers, v)
	}
	rows.Close()
	return c, nil
}

func (s *Service) Evaluate(ctx context.Context, input EvaluationInput) ([]Result, error) {
	if s == nil || !s.enabled || s.db == nil {
		return nil, nil
	}
	businesses := map[string]bool{}
	for _, line := range input.Lines {
		businesses[line.BusinessID] = true
	}
	var campaigns []Campaign
	for businessID := range businesses {
		rows, err := s.db.Query(ctx, `SELECT `+campaignColumns+` FROM commerce_promotion_campaigns WHERE business_id=$1 AND status IN ('ACTIVE','SCHEDULED') AND starts_at<=$2 AND ends_at>$2 ORDER BY priority,id`, businessID, input.At)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			c, scanErr := scanCampaign(rows)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			c, scanErr = s.loadChildren(ctx, s.db, c)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			campaigns = append(campaigns, c)
		}
		rows.Close()
		if input.CouponDigest != "" {
			var campaignID, digest string
			err = s.db.QueryRow(ctx, `SELECT campaign_id,code_digest FROM commerce_promotion_coupons WHERE business_id=$1 AND code_digest=$2 AND enabled AND revoked_at IS NULL`, businessID, input.CouponDigest).Scan(&campaignID, &digest)
			if err == nil {
				for i := range campaigns {
					if campaigns[i].ID == campaignID {
						campaigns[i].CouponDigest = digest
					}
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		}
		branches := make([]string, 0)
		items := make([]string, 0)
		for _, line := range input.Lines {
			if line.BusinessID == businessID {
				branches = append(branches, line.BranchID)
				items = append(items, line.ItemRef)
			}
		}
		rows, err = s.db.Query(ctx, `SELECT branch_id,capability_type FROM vendor_branch_capabilities WHERE business_id=$1 AND branch_id=ANY($2::text[]) AND status='ACTIVE'`, businessID, branches)
		if err == nil {
			if input.Capabilities == nil {
				input.Capabilities = map[string]bool{}
			}
			for rows.Next() {
				var branchID, capability string
				if rows.Scan(&branchID, &capability) == nil {
					input.Capabilities[capabilityEligibilityKey(businessID, branchID, capability)] = true
				}
			}
			rows.Close()
		}
		rows, err = s.db.Query(ctx, `
			SELECT id,section_id FROM food_items WHERE business_id=$1 AND id=ANY($2::text[])
			UNION ALL
			SELECT id,category_id FROM shop_products WHERE business_id=$1 AND id=ANY($2::text[])`, businessID, items)
		if err == nil {
			if input.CategoryRefs == nil {
				input.CategoryRefs = map[string]bool{}
			}
			for rows.Next() {
				var itemRef, categoryRef string
				if rows.Scan(&itemRef, &categoryRef) == nil {
					input.CategoryRefs[categoryEligibilityKey(businessID, itemRef, categoryRef)] = true
				}
			}
			rows.Close()
		}
		rows, err = s.db.Query(ctx, `SELECT DISTINCT m.segment_id FROM crm_segment_memberships m JOIN vendor_customer_profiles p ON p.id=m.profile_id AND p.business_id=m.business_id WHERE m.business_id=$1 AND p.customer_ref=$2`, businessID, input.CustomerID)
		if err == nil {
			if input.CustomerSegments == nil {
				input.CustomerSegments = map[string]bool{}
			}
			for rows.Next() {
				var segment string
				if rows.Scan(&segment) == nil {
					input.CustomerSegments[segment] = true
				}
			}
			rows.Close()
		}
	}
	return Evaluate(campaigns, input), nil
}
