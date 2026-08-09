// Package usecases implements the Section 13 company wallet workflows.
//
// The division of authority is absolute: payment-wallet-service owns balances,
// journals and execution; Fleet owns the decision to ask, the policy that
// governs asking, the approval workflow around it, and an opaque reference to
// the result. Nothing here computes a balance, and nothing here treats a local
// projection as permission to move money.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ApprovalPort is the narrow slice of the WP9 approval framework this feature
// uses. It is an interface so the composition root supplies the one real
// implementation and tests supply a fake, rather than a second framework
// growing here.
type ApprovalPort interface {
	Request(ctx context.Context, input ApprovalRequestInput) (approvalID string, err error)
	RequireApproved(ctx context.Context, organizationID, requestID string,
		payload map[string]any) error
}

// ApprovalRequestInput is what this feature needs to raise an approval.
type ApprovalRequestInput struct {
	OrganizationID         string
	OrganizationUserID     string
	BranchID               string
	ResourceType           string
	ResourceID             string
	Action                 string
	OperationType          string
	Payload                map[string]any
	ExpectedSubjectVersion int64
	IdempotencyKey         string
	CorrelationID          string
	CausationID            string
}

// EligibilityPort gates money-moving requests behind a current decision.
type EligibilityPort interface {
	Require(ctx context.Context, organizationID, branchID, operation string,
		actorType, actorID, correlationID string) error
}

// WorkVerifier answers whether a unit of work actually completed. It is the
// Dispatch-owned fact a per-job allocation depends on.
type WorkVerifier interface {
	RequireCompletedDelivery(ctx context.Context, organizationID, tripID string,
		requireRecipientConfirmation bool) error
}

// Flags are the Section 13 feature switches.
//
// Every write defaults false. A deployment that has not deliberately turned a
// capability on does not have it, which is what makes a partial rollout safe
// rather than merely untested.
type Flags struct {
	WalletReadEnabled        bool
	SubWalletEnabled         bool
	AllocationEnabled        bool
	WithdrawalEnabled        bool
	ProjectionRefreshEnabled bool
	ReconciliationEnabled    bool
	KillSwitch               bool
}

// Options wires the service.
type Options struct {
	Pool        *pgxpool.Pool
	Repository  *repositories.Repository
	Wallet      platformclients.FinanceWalletContract
	Approvals   ApprovalPort
	Eligibility EligibilityPort
	Work        WorkVerifier
	Events      *events.DomainPublisher
	Metrics     *observability.Metrics
	Flags       Flags
	StaleAfter  time.Duration
	Now         func() time.Time
}

// Service implements the Section 13 workflows.
type Service struct {
	opts  Options
	audit *audit.Recorder
}

// NewService builds the service. It is always constructible: whether a Wallet
// operation can actually be performed depends on the client being configured,
// and an unconfigured client makes those paths fail closed rather than making
// the read surface disappear.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.StaleAfter <= 0 {
		opts.StaleAfter = 15 * time.Minute
	}
	return &Service{opts: opts, audit: audit.NewRecorder()}
}

// Repository exposes persistence for composition and tests.
func (s *Service) Repository() *repositories.Repository { return s.opts.Repository }

// Flags reports the configured feature switches.
func (s *Service) Flags() Flags { return s.opts.Flags }

// Errors surfaced to callers.
var (
	// ErrProjectionStale means the local copy is not current enough to
	// authorize the requested operation.
	ErrProjectionStale = errorcodes.New(http.StatusServiceUnavailable,
		errorcodes.WalletProjectionStale,
		"Wallet state is not current. Try again shortly.", nil)
)

func notFound(resource string) error {
	// Cross-tenant identifiers and genuinely missing ones are answered
	// identically on purpose: a distinguishable response is an existence oracle.
	return apperrors.NotFound(resource+" was not found.", nil)
}

func (s *Service) now() time.Time { return s.opts.Now().UTC() }

func (s *Service) writesEnabled() error {
	if s.opts.KillSwitchEngaged() {
		return errorcodes.New(http.StatusServiceUnavailable, errorcodes.WalletOperationUnknown,
			"Wallet operations are temporarily unavailable.", nil)
	}
	return nil
}

// KillSwitchEngaged reports whether every Section 13 write is disabled.
func (o Options) KillSwitchEngaged() bool { return o.Flags.KillSwitch }

// KillSwitchEngaged reports whether every Section 13 write is disabled.
func (s *Service) KillSwitchEngaged() bool { return s.opts.KillSwitchEngaged() }

func (s *Service) walletConfigured() bool {
	return s.opts.Wallet != nil && s.opts.Wallet.Configured()
}

// requireEligible runs the durable eligibility gate before any state is read,
// so a suspended or unsubscribed organization cannot probe account state by
// asking for an operation it can never get.
func (s *Service) requireEligible(ctx context.Context, t tenant.Context, branchID,
	operation string) error {
	if s.opts.Eligibility == nil {
		return errorcodes.New(http.StatusServiceUnavailable, errorcodes.EligibilityUnknown,
			"Eligibility cannot be confirmed right now.", nil)
	}
	return s.opts.Eligibility.Require(ctx, t.OrganizationID, branchID, operation,
		string(t.ActorType), t.ActorID, t.CorrelationID)
}

// fingerprint produces a stable request fingerprint over the fields that
// define the operation. Two callers describing the same operation hash the
// same; any changed value hashes differently, which is what makes a retry
// distinguishable from a new request.
func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Wallet reads
// ---------------------------------------------------------------------------

// WalletView is the organization-facing wallet projection.
//
// Balances originate from payment-wallet-service. They are absent, not zero,
// when the authoritative read fails: a zero balance is a claim, and Fleet is
// not entitled to make it.
type WalletView struct {
	WalletID         string     `json:"walletId"`
	Currency         string     `json:"currency,omitempty"`
	AvailableBalance *int64     `json:"availableBalanceMinor,omitempty"`
	PendingBalance   *int64     `json:"pendingBalanceMinor,omitempty"`
	ReservedBalance  *int64     `json:"reservedBalanceMinor,omitempty"`
	Status           string     `json:"status"`
	SourceVersion    int64      `json:"sourceVersion"`
	SyncedAt         *time.Time `json:"syncedAt,omitempty"`
	FreshnessState   string     `json:"freshnessState"`
}

// GetWallet returns the organization wallet projection.
//
// authoritative asks payment-wallet-service directly and refreshes the local
// link. A caller that only wants to render a dashboard passes false; a caller
// about to move money does not use this method at all — it uses the gates.
func (s *Service) GetWallet(ctx context.Context, t tenant.Context, authoritative bool) (WalletView, error) {
	if !s.opts.Flags.WalletReadEnabled {
		return WalletView{}, notFound("Wallet")
	}
	link, err := s.opts.Repository.GetOrganizationLink(ctx, nil, t.OrganizationID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return WalletView{}, notFound("Wallet")
		}
		return WalletView{}, err
	}
	view := WalletView{WalletID: link.WalletAccountID, Currency: link.Currency,
		Status: link.Status, SourceVersion: link.SourceVersion, SyncedAt: link.SyncedAt,
		FreshnessState: link.FreshnessState}
	if !authoritative || !s.walletConfigured() || link.WalletAccountID == "" {
		return view, nil
	}
	account, readErr := s.opts.Wallet.ReadAccount(ctx,
		platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID},
		link.WalletAccountID)
	if readErr != nil {
		// The authoritative read failed. The local copy is downgraded rather
		// than returned as though it were current, so a subsequent sensitive
		// operation fails closed instead of trusting it.
		_ = s.opts.Repository.MarkOrganizationLinkStale(ctx, nil, t.OrganizationID, "wallet_unavailable")
		view.FreshnessState = models.FreshnessUnknown
		return view, nil
	}
	view.Status = account.Status
	view.SourceVersion = account.SourceVersion
	view.FreshnessState = models.FreshnessCurrent
	_ = s.opts.Repository.UpsertOrganizationLink(ctx, nil, t.OrganizationID, account.AccountID,
		linkStatusForAccount(account.Status), link.Currency, models.FreshnessCurrent, "",
		account.SourceVersion)
	return view, nil
}

// linkStatusForAccount maps an authoritative account status onto a Fleet link
// state. An unrecognized status becomes UNKNOWN rather than ACTIVE: a status
// Fleet does not understand is not a status Fleet may act on.
func linkStatusForAccount(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE":
		return models.LinkActive
	case "RESTRICTED":
		return models.LinkRestricted
	case "SUSPENDED":
		return models.LinkSuspended
	case "CLOSED":
		return models.LinkClosed
	default:
		return models.LinkUnknown
	}
}

// requireOperationalWallet is the gate every money-moving path passes.
//
// It refuses on anything short of an ACTIVE link with a CURRENT local copy,
// and it attempts one authoritative read-back before refusing, so a merely
// stale copy is repaired rather than blocking the operation permanently.
func (s *Service) requireOperationalWallet(ctx context.Context,
	t tenant.Context) (models.OrganizationWalletLink, error) {
	link, err := s.opts.Repository.GetOrganizationLink(ctx, nil, t.OrganizationID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return link, errorcodes.Conflict(errorcodes.OrganizationWalletNotReady,
				"The organization wallet is not ready.")
		}
		return link, err
	}
	if link.WalletAccountID == "" {
		return link, errorcodes.Conflict(errorcodes.OrganizationWalletNotReady,
			"The organization wallet is not ready.")
	}
	if link.Operational() && s.now().Sub(derefTime(link.SyncedAt)) < s.opts.StaleAfter {
		return link, nil
	}
	if !s.walletConfigured() {
		return link, ErrProjectionStale
	}
	account, readErr := s.opts.Wallet.ReadAccount(ctx,
		platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID},
		link.WalletAccountID)
	if readErr != nil {
		return link, ErrProjectionStale
	}
	status := linkStatusForAccount(account.Status)
	_ = s.opts.Repository.UpsertOrganizationLink(ctx, nil, t.OrganizationID, account.AccountID,
		status, link.Currency, models.FreshnessCurrent, "", account.SourceVersion)
	if status != models.LinkActive {
		return link, errorcodes.Conflict(errorcodes.OrganizationWalletNotReady,
			"The organization wallet is not ready.")
	}
	link.Status = status
	link.FreshnessState = models.FreshnessCurrent
	link.SourceVersion = account.SourceVersion
	return link, nil
}

// requireOperationalSubWallet is the same gate for a provider sub-wallet.
func (s *Service) requireOperationalSubWallet(ctx context.Context, t tenant.Context,
	membershipID string) (models.ProviderSubWalletLink, error) {
	link, err := s.opts.Repository.GetSubWalletLink(ctx, nil, t.OrganizationID, membershipID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return link, errorcodes.Conflict(errorcodes.ProviderSubwalletNotReady,
				"The provider sub-wallet is not ready.")
		}
		return link, err
	}
	if link.WalletAccountID == "" {
		return link, errorcodes.Conflict(errorcodes.ProviderSubwalletNotReady,
			"The provider sub-wallet is not ready.")
	}
	if link.Operational() && s.now().Sub(derefTime(link.SyncedAt)) < s.opts.StaleAfter {
		return link, nil
	}
	if !s.walletConfigured() {
		return link, ErrProjectionStale
	}
	account, readErr := s.opts.Wallet.ReadAccount(ctx,
		platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID},
		link.WalletAccountID)
	if readErr != nil {
		return link, ErrProjectionStale
	}
	status := linkStatusForAccount(account.Status)
	_ = s.opts.Repository.UpsertSubWalletLink(ctx, nil, t.OrganizationID, membershipID,
		account.AccountID, link.ProviderID, link.ProviderService, link.Currency, status,
		models.FreshnessCurrent, "", account.SourceVersion)
	if status != models.LinkActive {
		return link, errorcodes.Conflict(errorcodes.ProviderSubwalletNotReady,
			"The provider sub-wallet is not ready.")
	}
	link.Status = status
	link.FreshnessState = models.FreshnessCurrent
	link.SourceVersion = account.SourceVersion
	return link, nil
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// SubWalletView is the corporate-facing sub-wallet projection.
type SubWalletView struct {
	ProviderSubWalletID string     `json:"providerSubWalletId"`
	MembershipID        string     `json:"membershipId"`
	Currency            string     `json:"currency,omitempty"`
	Status              string     `json:"status"`
	SourceVersion       int64      `json:"sourceVersion"`
	SyncedAt            *time.Time `json:"syncedAt,omitempty"`
	FreshnessState      string     `json:"freshnessState"`
}

// GetSubWallet returns one provider sub-wallet projection.
func (s *Service) GetSubWallet(ctx context.Context, t tenant.Context,
	membershipID string) (SubWalletView, error) {
	if !s.opts.Flags.SubWalletEnabled {
		return SubWalletView{}, notFound("Provider sub-wallet")
	}
	link, err := s.opts.Repository.GetSubWalletLink(ctx, nil, t.OrganizationID, membershipID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return SubWalletView{}, notFound("Provider sub-wallet")
		}
		return SubWalletView{}, err
	}
	return SubWalletView{ProviderSubWalletID: link.WalletAccountID,
		MembershipID: link.ProviderMembershipID, Currency: link.Currency, Status: link.Status,
		SourceVersion: link.SourceVersion, SyncedAt: link.SyncedAt,
		FreshnessState: link.FreshnessState}, nil
}

// ---------------------------------------------------------------------------
// Policy
// ---------------------------------------------------------------------------

// GetPolicy returns the organization's withdrawal and allocation policy.
func (s *Service) GetPolicy(ctx context.Context, t tenant.Context) (models.WithdrawalPolicy, error) {
	return s.opts.Repository.GetPolicy(ctx, nil, t.OrganizationID)
}

// SetPolicyInput changes the organization's policy.
type SetPolicyInput struct {
	Tenant                           tenant.Context
	PolicyMode                       string
	MinimumAmountMinor               int64
	ApprovalThresholdMinor           int64
	AllocationApprovalThresholdMinor int64
	Currency                         string
	ExpectedVersion                  int64
}

// SetPolicy stores the organization's policy.
func (s *Service) SetPolicy(ctx context.Context, in SetPolicyInput) (models.WithdrawalPolicy, error) {
	if err := s.writesEnabled(); err != nil {
		return models.WithdrawalPolicy{}, err
	}
	if !models.ValidPolicyMode(in.PolicyMode) {
		return models.WithdrawalPolicy{}, apperrors.BadRequest(
			"policyMode must be one of PROVIDER_DIRECT, COMPANY_APPROVAL_REQUIRED, COMPANY_INITIATED_ONLY or WITHDRAWALS_DISABLED.", nil)
	}
	if in.MinimumAmountMinor < 0 || in.ApprovalThresholdMinor < 0 ||
		in.AllocationApprovalThresholdMinor < 0 {
		return models.WithdrawalPolicy{}, apperrors.BadRequest("Thresholds must not be negative.", nil)
	}
	previous, err := s.opts.Repository.GetPolicy(ctx, nil, in.Tenant.OrganizationID)
	if err != nil {
		return models.WithdrawalPolicy{}, err
	}
	var stored models.WithdrawalPolicy
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var writeErr error
		stored, writeErr = s.opts.Repository.UpsertPolicy(ctx, tx, models.WithdrawalPolicy{
			OrganizationID: in.Tenant.OrganizationID, PolicyMode: in.PolicyMode,
			MinimumAmountMinor:               in.MinimumAmountMinor,
			ApprovalThresholdMinor:           in.ApprovalThresholdMinor,
			AllocationApprovalThresholdMinor: in.AllocationApprovalThresholdMinor,
			Currency:                         in.Currency, UpdatedBy: in.Tenant.ActorID}, in.ExpectedVersion)
		if writeErr != nil {
			return writeErr
		}
		if auditErr := s.audit.Record(ctx, tx, audit.Entry{
			OrganizationID: in.Tenant.OrganizationID, ActorType: string(in.Tenant.ActorType),
			ActorID: in.Tenant.ActorID, Action: "wallet.policy.changed",
			ResourceType: "wallet_policy", ResourceID: in.Tenant.OrganizationID,
			PreviousValues: map[string]any{"policyMode": previous.PolicyMode},
			NewValues:      map[string]any{"policyMode": stored.PolicyMode},
			RequestID:      in.Tenant.RequestID, CorrelationID: in.Tenant.CorrelationID}); auditErr != nil {
			return auditErr
		}
		return s.publish(ctx, tx, "organization.wallet.policy_changed.v1", "WALLET_POLICY",
			in.Tenant.OrganizationID, stored.Version, in.Tenant,
			map[string]any{"policyMode": stored.PolicyMode})
	})
	if errors.Is(err, repositories.ErrVersionConflict) {
		return models.WithdrawalPolicy{}, apperrors.New(http.StatusConflict,
			apperrors.CodeConflict, "The policy changed since it was read.", nil)
	}
	return stored, err
}

// publish appends a domain event to the transactional outbox.
//
// Payloads carry safe references only: never a bank detail, a Wallet secret, a
// customer fare, a company margin or an internal note. A consumer that learns
// an allocation happened must not thereby learn what the job was worth.
func (s *Service) publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType,
	aggregateID string, version int64, t tenant.Context, payload map[string]any) error {
	if s.opts.Events == nil {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["organizationId"] = t.OrganizationID
	return s.opts.Events.Publish(ctx, tx, eventType, aggregateType, aggregateID, version,
		t.OrganizationID, t.CorrelationID, payload)
}

func (s *Service) count(name string, labels map[string]string) {
	if s.opts.Metrics != nil {
		s.opts.Metrics.IncCounter(name, labels, 1)
	}
}

// eligibilityOperation names the policy governing a Section 13 operation.
func eligibilityOperation(allocationType string) string {
	if models.DebitsProvider(allocationType) {
		return eligibilitymodels.OperationFinancialReversal
	}
	return eligibilitymodels.OperationFinancialRelease
}

func upper(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func validCurrency(value string) (string, error) {
	currency := upper(value)
	if len(currency) != 3 {
		return "", apperrors.BadRequest("currency must be a three letter ISO code.", nil)
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return "", apperrors.BadRequest("currency must be a three letter ISO code.", nil)
		}
	}
	return currency, nil
}

func requireText(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return apperrors.BadRequest(fmt.Sprintf("%s is required.", field), nil)
	}
	return nil
}
