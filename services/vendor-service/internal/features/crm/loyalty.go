package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8D — loyalty points.
//
// Everything in this file counts POINTS. Points are integers, they are never
// a currency, and nothing here reads or writes a wallet. payment-wallet-service
// remains the only monetary authority on the platform; a vendor granting a
// customer 500 points is a marketing decision recorded in vendor-service, not
// a transfer of value, and no code path converts between the two.

// Programme statuses.
const (
	LoyaltyProgramDisabled = "DISABLED"
	LoyaltyProgramActive   = "ACTIVE"
	LoyaltyProgramPaused   = "PAUSED"
)

// Expiry policies.
const (
	LoyaltyExpiryNever     = "NEVER"
	LoyaltyExpiryFixedDays = "FIXED_DAYS"
)

// Account statuses.
const (
	LoyaltyAccountActive   = "ACTIVE"
	LoyaltyAccountMerged   = "MERGED"
	LoyaltyAccountRedacted = "REDACTED"
)

// Ledger entry types.
const (
	LoyaltyEarn             = "EARN"
	LoyaltyRedeem           = "REDEEM"
	LoyaltyExpire           = "EXPIRE"
	LoyaltyAdjustmentCredit = "ADJUSTMENT_CREDIT"
	LoyaltyAdjustmentDebit  = "ADJUSTMENT_DEBIT"
	LoyaltyReversal         = "REVERSAL"
	LoyaltyMergeIn          = "MERGE_IN"
	LoyaltyMergeOut         = "MERGE_OUT"
)

// Ledger source types.
const (
	LoyaltySourceManual   = "MANUAL_ADJUSTMENT"
	LoyaltySourceOrder    = "ORDER"
	LoyaltySourceExpiry   = "SYSTEM_EXPIRY"
	LoyaltySourceReversal = "REVERSAL"
	LoyaltySourceMerge    = "IDENTITY_MERGE"
	LoyaltySourceExternal = "EXTERNAL"
)

// Section 8D error codes.
const (
	CodeLoyaltyProgramDisabled    = "LOYALTY_PROGRAM_DISABLED"
	CodeLoyaltyProgramPaused      = "LOYALTY_PROGRAM_PAUSED"
	CodeLoyaltyAccountNotFound    = "LOYALTY_ACCOUNT_NOT_FOUND"
	CodeLoyaltyAccountNotAction   = "LOYALTY_ACCOUNT_NOT_ACTIONABLE"
	CodeLoyaltyInsufficient       = "LOYALTY_INSUFFICIENT_POINTS"
	CodeLoyaltyInvalidPoints      = "LOYALTY_INVALID_POINTS"
	CodeLoyaltyAdjustmentLimit    = "LOYALTY_ADJUSTMENT_LIMIT_EXCEEDED"
	CodeLoyaltyInvalidExpiry      = "LOYALTY_INVALID_EXPIRY_POLICY"
	CodeLoyaltyVersionConflict    = "LOYALTY_VERSION_CONFLICT"
	CodeLoyaltyIdempotency        = "LOYALTY_IDEMPOTENCY_CONFLICT"
	CodeLoyaltyEntryNotFound      = "LOYALTY_ENTRY_NOT_FOUND"
	CodeLoyaltyEntryNotReversible = "LOYALTY_ENTRY_NOT_REVERSIBLE"
	CodeLoyaltyAlreadyReversed    = "LOYALTY_ENTRY_ALREADY_REVERSED"
	CodeLoyaltyAlreadyConsumed    = "LOYALTY_REVERSAL_POINTS_ALREADY_CONSUMED"
	CodeLoyaltyCustomerRedacted   = "LOYALTY_CUSTOMER_REDACTED"
	CodeLoyaltyCustomerMerged     = "LOYALTY_CUSTOMER_MERGED"
)

// MaxLoyaltyPoints bounds every points quantity the service will accept or
// hold. It exists so a caller (or a future producer with a unit bug) cannot
// drive a balance towards int64 overflow: any arithmetic in this file stays
// three orders of magnitude below the limit even when several maxima are
// added together.
const MaxLoyaltyPoints int64 = 1_000_000_000_000

// LoyaltyLedgerDefaultPageSize and LoyaltyLedgerMaxPageSize bound history
// reads, matching the Section 8C pagination convention.
const (
	LoyaltyLedgerDefaultPageSize = 50
	LoyaltyLedgerMaxPageSize     = 200
)

// LoyaltyProgram is the per-business programme configuration.
type LoyaltyProgram struct {
	ID, BusinessID, Status               string
	PointsNameSingular, PointsNamePlural string
	ExpiryPolicy                         string
	ExpiryDays                           *int
	MinimumRedemptionPoints              int64
	RedemptionIncrementPoints            int64
	ManualAdjustmentLimit                int64
	CreatedAt, UpdatedAt                 time.Time
	Version                              int64
	// Persisted is false for the synthetic default returned by a read before
	// any configuration has ever been written. It is what lets a GET avoid
	// creating a row purely because someone looked.
	Persisted bool
}

// LoyaltyProgramView is the wire projection.
type LoyaltyProgramView struct {
	Status                    string     `json:"status"`
	PointsNameSingular        string     `json:"points_name_singular"`
	PointsNamePlural          string     `json:"points_name_plural"`
	ExpiryPolicy              string     `json:"expiry_policy"`
	ExpiryDays                *int       `json:"expiry_days,omitempty"`
	MinimumRedemptionPoints   int64      `json:"minimum_redemption_points"`
	RedemptionIncrementPoints int64      `json:"redemption_increment_points"`
	ManualAdjustmentLimit     int64      `json:"manual_adjustment_limit"`
	Configured                bool       `json:"configured"`
	CreatedAt                 *time.Time `json:"created_at,omitempty"`
	UpdatedAt                 *time.Time `json:"updated_at,omitempty"`
	Version                   int64      `json:"version"`
}

// LoyaltyProgramViewOf renders a programme for the wire.
func LoyaltyProgramViewOf(p LoyaltyProgram) LoyaltyProgramView {
	view := LoyaltyProgramView{
		Status: p.Status, PointsNameSingular: p.PointsNameSingular, PointsNamePlural: p.PointsNamePlural,
		ExpiryPolicy: p.ExpiryPolicy, ExpiryDays: p.ExpiryDays,
		MinimumRedemptionPoints: p.MinimumRedemptionPoints, RedemptionIncrementPoints: p.RedemptionIncrementPoints,
		ManualAdjustmentLimit: p.ManualAdjustmentLimit, Configured: p.Persisted, Version: p.Version,
	}
	if p.Persisted {
		created, updated := p.CreatedAt, p.UpdatedAt
		view.CreatedAt, view.UpdatedAt = &created, &updated
	}
	return view
}

// defaultLoyaltyProgram is the configuration a business has before it has ever
// configured one: switched off, never expiring, no redemption granularity.
// Version 0 marks it as unwritten, so the first PATCH must send
// expected_version 0 and two concurrent first PATCHes cannot both succeed.
func defaultLoyaltyProgram(businessID string) LoyaltyProgram {
	return LoyaltyProgram{
		BusinessID: businessID, Status: LoyaltyProgramDisabled,
		PointsNameSingular: "point", PointsNamePlural: "points",
		ExpiryPolicy: LoyaltyExpiryNever, MinimumRedemptionPoints: 0,
		RedemptionIncrementPoints: 1, ManualAdjustmentLimit: 100000, Version: 0,
	}
}

// LoyaltyAccount is one customer's points balance within one business.
//
// It holds no name, email, phone or address: the customer is a CRM profile
// reference and nothing more.
type LoyaltyAccount struct {
	ID, BusinessID, CustomerProfileID, Status string
	CurrentPoints                             int64
	LifetimeCreditedPoints                    int64
	LifetimeDebitedPoints                     int64
	NextSequence                              int64
	CreatedAt, UpdatedAt                      time.Time
	Version                                   int64
}

// LoyaltyEntry is one immutable ledger row.
type LoyaltyEntry struct {
	ID, BusinessID, LoyaltyAccountID string
	Sequence                         int64
	EntryType                        string
	DeltaPoints, BalanceAfter        int64
	ProgramVersion                   int64
	SourceType                       string
	SourceReference                  *string
	IdempotencyReference             *string
	ReversesEntryID                  *string
	ReasonCode                       *string
	CreatedByStaffID                 *string
	CreatedAt                        time.Time
}

// LoyaltyEntryView is the wire projection of a ledger row.
//
// idempotency_reference and source_reference for internal command keys are
// deliberately absent: history is for reading what happened to the balance,
// not for harvesting command keys that could be replayed.
type LoyaltyEntryView struct {
	EntryRef         string    `json:"entry_ref"`
	Sequence         int64     `json:"sequence"`
	EntryType        string    `json:"entry_type"`
	DeltaPoints      int64     `json:"delta_points"`
	BalanceAfter     int64     `json:"balance_after"`
	SourceType       string    `json:"source_type"`
	ReasonCode       *string   `json:"reason_code,omitempty"`
	ReversesEntryRef *string   `json:"reverses_entry_ref,omitempty"`
	ReversedByRef    *string   `json:"reversed_by_entry_ref,omitempty"`
	CreatedByRef     *string   `json:"created_by_staff_ref,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// LoyaltyEntryViewOf renders one ledger row.
func LoyaltyEntryViewOf(e LoyaltyEntry) LoyaltyEntryView {
	return LoyaltyEntryView{
		EntryRef: e.ID, Sequence: e.Sequence, EntryType: e.EntryType, DeltaPoints: e.DeltaPoints,
		BalanceAfter: e.BalanceAfter, SourceType: e.SourceType, ReasonCode: e.ReasonCode,
		ReversesEntryRef: e.ReversesEntryID, CreatedByRef: e.CreatedByStaffID, CreatedAt: e.CreatedAt,
	}
}

// LoyaltyPointLot is one issuance of points with its own expiry.
type LoyaltyPointLot struct {
	ID, BusinessID, LoyaltyAccountID, SourceEntryID string
	IssuedPoints, RemainingPoints                   int64
	ExpiresAt                                       *time.Time
	CreatedAt                                       time.Time
}

// Enrollment states reported by the customer-facing read model.
const (
	LoyaltyNotEnrolled = "NOT_ENROLLED"
	LoyaltyEnrolled    = "ENROLLED"
)

// LoyaltySummary is the customer loyalty read model, also embedded in
// Customer 360.
type LoyaltySummary struct {
	ProgramStatus string `json:"program_status"`
	// Enrollment is NOT_ENROLLED for a customer who has never held points.
	// A GET never creates an account, so this is a genuine "no row" rather
	// than a zero balance we invented.
	Enrollment    string `json:"enrollment"`
	AccountStatus string `json:"account_status,omitempty"`

	PointsNameSingular string `json:"points_name_singular"`
	PointsNamePlural   string `json:"points_name_plural"`

	// AvailablePoints excludes lots whose expiry has already passed, whether
	// or not the expiry worker has swept them yet. It is the number a
	// customer can actually spend.
	AvailablePoints        int64 `json:"available_points"`
	LifetimeCreditedPoints int64 `json:"lifetime_credited_points"`
	LifetimeDebitedPoints  int64 `json:"lifetime_debited_points"`
	// PendingExpiryPoints are already past their expiry and unspendable; they
	// are reported so a UI can explain a balance that is about to drop rather
	// than appear to lose points silently.
	PendingExpiryPoints int64 `json:"pending_expiry_points"`

	NextExpiryAt       *time.Time `json:"next_expiry_at,omitempty"`
	PointsExpiringNext int64      `json:"points_expiring_next,omitempty"`

	Version int64 `json:"version,omitempty"`
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

func loyaltyValidation(field, message, code string) error {
	err := apperrors.Validation("Check your loyalty details.", []apperrors.FieldViolation{{Field: field, Message: message}})
	err.Details = map[string]any{"code": code}
	return err
}
func errLoyaltyAccountNotFound() error {
	return crmError(apperrors.NotFound("This customer has no loyalty account.", nil), CodeLoyaltyAccountNotFound)
}
func errLoyaltyAccountNotActionable() error {
	return crmError(apperrors.Conflict("This loyalty account can no longer be changed.", nil), CodeLoyaltyAccountNotAction)
}
func errLoyaltyInsufficient() error {
	return crmError(apperrors.Conflict("This customer does not have enough available points.", nil), CodeLoyaltyInsufficient)
}
func errLoyaltyProgramState(status string) error {
	if status == LoyaltyProgramPaused {
		return crmError(apperrors.Conflict("The loyalty programme is paused.", nil), CodeLoyaltyProgramPaused)
	}
	return crmError(apperrors.Conflict("The loyalty programme is not enabled.", nil), CodeLoyaltyProgramDisabled)
}
func errLoyaltyVersionConflict() error {
	return crmError(apperrors.Conflict("The loyalty programme changed since you loaded it. Reload and try again.", nil), CodeLoyaltyVersionConflict)
}
func errLoyaltyEntryNotFound() error {
	return crmError(apperrors.NotFound("Loyalty entry not found.", nil), CodeLoyaltyEntryNotFound)
}

// ---------------------------------------------------------------------------
// Programme configuration
// ---------------------------------------------------------------------------

const loyaltyProgramCols = `id,business_id,status,points_name_singular,points_name_plural,expiry_policy,expiry_days,
minimum_redemption_points,redemption_increment_points,manual_adjustment_limit,created_at,updated_at,version`

func scanLoyaltyProgram(row rowScanner) (LoyaltyProgram, error) {
	var p LoyaltyProgram
	err := row.Scan(&p.ID, &p.BusinessID, &p.Status, &p.PointsNameSingular, &p.PointsNamePlural,
		&p.ExpiryPolicy, &p.ExpiryDays, &p.MinimumRedemptionPoints, &p.RedemptionIncrementPoints,
		&p.ManualAdjustmentLimit, &p.CreatedAt, &p.UpdatedAt, &p.Version)
	p.Persisted = err == nil
	return p, err
}

// GetLoyaltyProgram reads the programme without ever writing one.
func (s *Service) GetLoyaltyProgram(ctx context.Context, actor Actor) (LoyaltyProgram, error) {
	program, err := scanLoyaltyProgram(s.db.QueryRow(ctx,
		`SELECT `+loyaltyProgramCols+` FROM crm_loyalty_program_configs WHERE business_id=$1`, actor.VendorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultLoyaltyProgram(actor.VendorID), nil
	}
	if err != nil {
		return LoyaltyProgram{}, err
	}
	return program, nil
}

// UpdateLoyaltyProgramInput is a partial programme update. A nil field is
// left unchanged; ExpiryPolicy and ExpiryDays are validated together.
type UpdateLoyaltyProgramInput struct {
	Status                    *string
	PointsNameSingular        *string
	PointsNamePlural          *string
	ExpiryPolicy              *string
	ExpiryDays                *int
	ClearExpiryDays           bool
	MinimumRedemptionPoints   *int64
	RedemptionIncrementPoints *int64
	ManualAdjustmentLimit     *int64
	ExpectedVersion           int64
}

func validLoyaltyStatus(value string) bool {
	return value == LoyaltyProgramDisabled || value == LoyaltyProgramActive || value == LoyaltyProgramPaused
}

// apply merges the patch onto a programme and validates the result as a whole.
//
// Validation happens after the merge, not field by field, because the invalid
// states here are combinations: FIXED_DAYS without a window, or NEVER with one.
func (in UpdateLoyaltyProgramInput) apply(current LoyaltyProgram) (LoyaltyProgram, error) {
	next := current
	if in.Status != nil {
		next.Status = strings.ToUpper(strings.TrimSpace(*in.Status))
		if !validLoyaltyStatus(next.Status) {
			return next, loyaltyValidation("status", "Status must be DISABLED, ACTIVE or PAUSED.", CodeLoyaltyInvalidPoints)
		}
	}
	if in.PointsNameSingular != nil {
		value, err := normalizePlainText(*in.PointsNameSingular, 40, true, true, "points_name_singular")
		if err != nil {
			return next, err
		}
		next.PointsNameSingular = value
	}
	if in.PointsNamePlural != nil {
		value, err := normalizePlainText(*in.PointsNamePlural, 40, true, true, "points_name_plural")
		if err != nil {
			return next, err
		}
		next.PointsNamePlural = value
	}
	if in.ExpiryPolicy != nil {
		next.ExpiryPolicy = strings.ToUpper(strings.TrimSpace(*in.ExpiryPolicy))
		if next.ExpiryPolicy != LoyaltyExpiryNever && next.ExpiryPolicy != LoyaltyExpiryFixedDays {
			return next, loyaltyValidation("expiry_policy", "Expiry policy must be NEVER or FIXED_DAYS.", CodeLoyaltyInvalidExpiry)
		}
	}
	if in.ClearExpiryDays {
		next.ExpiryDays = nil
	}
	if in.ExpiryDays != nil {
		value := *in.ExpiryDays
		next.ExpiryDays = &value
	}
	switch next.ExpiryPolicy {
	case LoyaltyExpiryNever:
		if next.ExpiryDays != nil {
			return next, loyaltyValidation("expiry_days", "A never-expiring programme cannot set an expiry window.", CodeLoyaltyInvalidExpiry)
		}
	case LoyaltyExpiryFixedDays:
		if next.ExpiryDays == nil {
			return next, loyaltyValidation("expiry_days", "An expiry window in days is required.", CodeLoyaltyInvalidExpiry)
		}
		if *next.ExpiryDays < 1 || *next.ExpiryDays > 3650 {
			return next, loyaltyValidation("expiry_days", "Expiry window must be between 1 and 3650 days.", CodeLoyaltyInvalidExpiry)
		}
	}
	if in.MinimumRedemptionPoints != nil {
		if *in.MinimumRedemptionPoints < 0 || *in.MinimumRedemptionPoints > 1_000_000_000 {
			return next, loyaltyValidation("minimum_redemption_points", "Minimum redemption must be between 0 and 1000000000.", CodeLoyaltyInvalidPoints)
		}
		next.MinimumRedemptionPoints = *in.MinimumRedemptionPoints
	}
	if in.RedemptionIncrementPoints != nil {
		if *in.RedemptionIncrementPoints < 1 || *in.RedemptionIncrementPoints > 1_000_000_000 {
			return next, loyaltyValidation("redemption_increment_points", "Redemption increment must be between 1 and 1000000000.", CodeLoyaltyInvalidPoints)
		}
		next.RedemptionIncrementPoints = *in.RedemptionIncrementPoints
	}
	if in.ManualAdjustmentLimit != nil {
		if *in.ManualAdjustmentLimit < 1 || *in.ManualAdjustmentLimit > 1_000_000_000 {
			return next, loyaltyValidation("manual_adjustment_limit", "Manual adjustment limit must be between 1 and 1000000000.", CodeLoyaltyInvalidPoints)
		}
		next.ManualAdjustmentLimit = *in.ManualAdjustmentLimit
	}
	return next, nil
}

// UpdateLoyaltyProgram writes the programme configuration under optimistic
// concurrency.
//
// Programme management stays available in every programme status, including
// DISABLED, because turning the programme back on is itself a configuration
// change. What it never does is rewrite history: expiry dates already stamped
// onto issued point lots are not recomputed here or anywhere else.
func (s *Service) UpdateLoyaltyProgram(ctx context.Context, actor Actor, in UpdateLoyaltyProgramInput) (LoyaltyProgram, error) {
	if !s.flags.LoyaltyEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return LoyaltyProgram{}, errWritesDisabled()
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LoyaltyProgram{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanLoyaltyProgram(tx.QueryRow(ctx,
		`SELECT `+loyaltyProgramCols+` FROM crm_loyalty_program_configs WHERE business_id=$1 FOR UPDATE`, actor.VendorID))
	existed := true
	if errors.Is(err, pgx.ErrNoRows) {
		existed = false
		current = defaultLoyaltyProgram(actor.VendorID)
	} else if err != nil {
		return LoyaltyProgram{}, err
	}
	if in.ExpectedVersion != current.Version {
		return LoyaltyProgram{}, errLoyaltyVersionConflict()
	}
	next, err := in.apply(current)
	if err != nil {
		return LoyaltyProgram{}, err
	}

	var saved LoyaltyProgram
	if existed {
		saved, err = scanLoyaltyProgram(tx.QueryRow(ctx, `UPDATE crm_loyalty_program_configs SET
			status=$1,points_name_singular=$2,points_name_plural=$3,expiry_policy=$4,expiry_days=$5,
			minimum_redemption_points=$6,redemption_increment_points=$7,manual_adjustment_limit=$8,
			updated_at=now(),version=version+1
			WHERE business_id=$9 AND version=$10 RETURNING `+loyaltyProgramCols,
			next.Status, next.PointsNameSingular, next.PointsNamePlural, next.ExpiryPolicy, next.ExpiryDays,
			next.MinimumRedemptionPoints, next.RedemptionIncrementPoints, next.ManualAdjustmentLimit,
			actor.VendorID, current.Version))
		if errors.Is(err, pgx.ErrNoRows) {
			return LoyaltyProgram{}, errLoyaltyVersionConflict()
		}
	} else {
		saved, err = scanLoyaltyProgram(tx.QueryRow(ctx, `INSERT INTO crm_loyalty_program_configs
			(id,business_id,status,points_name_singular,points_name_plural,expiry_policy,expiry_days,
			 minimum_redemption_points,redemption_increment_points,manual_adjustment_limit)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+loyaltyProgramCols,
			newID("crmlp_"), actor.VendorID, next.Status, next.PointsNameSingular, next.PointsNamePlural,
			next.ExpiryPolicy, next.ExpiryDays, next.MinimumRedemptionPoints, next.RedemptionIncrementPoints,
			next.ManualAdjustmentLimit))
		// Two callers can both find no row to lock, so the unique constraint
		// is the real arbiter of a concurrent first write. The loser is told
		// the state changed, not that the database complained.
		if isUniqueViolation(err) {
			return LoyaltyProgram{}, errLoyaltyVersionConflict()
		}
	}
	if err != nil {
		return LoyaltyProgram{}, err
	}

	if err = s.auditTx(ctx, tx, actor, "crm.loyalty.program.updated", "crm_loyalty_program", saved.ID, "",
		safeLoyaltyProgramAudit(current, existed), safeLoyaltyProgramAudit(saved, true), ""); err != nil {
		return LoyaltyProgram{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.loyalty.program.updated", "crm_loyalty_program", saved.ID, saved.Version,
		map[string]any{
			"business_ref": saved.BusinessID, "status": saved.Status, "expiry_policy": saved.ExpiryPolicy,
			"expiry_days": saved.ExpiryDays, "program_version": saved.Version, "occurred_at": s.now(),
		}); err != nil {
		return LoyaltyProgram{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LoyaltyProgram{}, err
	}
	s.metrics.Inc("crm_loyalty_program_update_total", "status", saved.Status)
	return saved, nil
}

func safeLoyaltyProgramAudit(p LoyaltyProgram, exists bool) map[string]any {
	if !exists {
		return nil
	}
	return map[string]any{
		"status": p.Status, "expiry_policy": p.ExpiryPolicy, "expiry_days": p.ExpiryDays,
		"minimum_redemption_points": p.MinimumRedemptionPoints, "redemption_increment_points": p.RedemptionIncrementPoints,
		"manual_adjustment_limit": p.ManualAdjustmentLimit, "version": p.Version,
	}
}

// loadProgramForUpdate returns the programme inside a mutation transaction,
// materialising the default row if the business has never configured one.
//
// Writing here rather than on read is deliberate: a mutation is already a
// write, and every ledger entry must reference a real programme version, so
// the programme a credit was issued under stays reconstructable.
func (s *Service) loadProgramForUpdate(ctx context.Context, tx pgx.Tx, businessID string) (LoyaltyProgram, error) {
	program, err := scanLoyaltyProgram(tx.QueryRow(ctx,
		`SELECT `+loyaltyProgramCols+` FROM crm_loyalty_program_configs WHERE business_id=$1 FOR UPDATE`, businessID))
	if err == nil {
		return program, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LoyaltyProgram{}, err
	}
	defaults := defaultLoyaltyProgram(businessID)
	program, err = scanLoyaltyProgram(tx.QueryRow(ctx, `INSERT INTO crm_loyalty_program_configs
		(id,business_id,status,points_name_singular,points_name_plural,expiry_policy,
		 minimum_redemption_points,redemption_increment_points,manual_adjustment_limit)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (business_id) DO UPDATE SET updated_at=crm_loyalty_program_configs.updated_at
		RETURNING `+loyaltyProgramCols,
		newID("crmlp_"), businessID, defaults.Status, defaults.PointsNameSingular, defaults.PointsNamePlural,
		defaults.ExpiryPolicy, defaults.MinimumRedemptionPoints, defaults.RedemptionIncrementPoints,
		defaults.ManualAdjustmentLimit))
	return program, err
}
