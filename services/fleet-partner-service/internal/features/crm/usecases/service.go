// Package usecases implements the Section 14 CRM workflows: customers,
// contacts, locations, sales opportunities, quotations, contracts, service
// level agreements and customer credit policy.
//
// The division of authority is deliberate and absolute:
//
//   - media-file-service owns document bytes, safety and retention. Fleet holds
//     an opaque object ID and a projection of its readiness, never a URL, a
//     path, a checksum or a credential.
//   - notification-service owns delivery. Fleet records a durable intent inside
//     the domain transaction and lets a worker deliver it afterwards, so an
//     outage cannot roll back a commitment already made to a customer.
//   - customer-service owns marketplace identity. A CRM customer is a
//     commercial relationship record; it is never created, merged or resolved
//     against a marketplace account.
//   - Fleet owns no receivables. There is no debt calculation, no invoice
//     ageing and no derived available credit anywhere in this package.
//
// Section 15 is out of scope. Accepting a quotation produces a conversion
// intent and nothing else: no order, shipment, package, assignment, trip,
// reservation or wallet movement.
package usecases

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ---------------------------------------------------------------------------
// Ports
// ---------------------------------------------------------------------------

// ApprovalPort is the narrow slice of the WP9 approval framework this feature
// uses.
//
// It is an interface so the composition root supplies the one real
// implementation. There is deliberately no CRM-specific approval table, engine
// or decision record: a second approval framework would be a second, weaker set
// of separation-of-duties rules.
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

// MediaPort is the slice of media-file-service this feature depends on.
//
// Fleet asks Media what state an object is in and never decides for itself. An
// unconfigured client makes every document-dependent path fail closed, which is
// honest, rather than letting an unchecked file back a commercial commitment.
type MediaPort interface {
	ReadObject(ctx context.Context, meta platformclients.CallMeta,
		mediaObjectID string) (platformclients.MediaObjectState, error)
	AuthorizeDownload(ctx context.Context, meta platformclients.CallMeta,
		mediaObjectID string) (platformclients.MediaDownloadGrant, error)
	Configured() bool
}

// NotificationOutcome classifies a delivery attempt.
type NotificationOutcome int

// Delivery outcomes. UNKNOWN is a first-class result: a send whose fate cannot
// be determined is neither delivered nor failed, and collapsing it into either
// produces a wrong record.
const (
	NotificationDelivered NotificationOutcome = iota
	NotificationRetryable
	NotificationPermanent
	NotificationUnknown
)

// NotificationPort delivers a CRM notification intent.
type NotificationPort interface {
	Deliver(ctx context.Context, operation models.NotificationOperation) (
		deliveryReference string, outcome NotificationOutcome, safeErrorCode string)
	SupportedChannels() []string
	Configured() bool
}

// SupportPort raises a support case for an SLA escalation.
type SupportPort interface {
	RaiseSLAEscalation(ctx context.Context, organizationID, slaID, contractID,
		correlationID string) (caseReference string, err error)
	Configured() bool
}

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

// Flags are the Section 14 feature switches.
//
// Every write and every worker defaults false. Read is separable from write so
// an operator can be given visibility into CRM without being given the ability
// to commit to a customer.
type Flags struct {
	CRMEnabled             bool
	CustomerWritesEnabled  bool
	ContactWritesEnabled   bool
	LocationWritesEnabled  bool
	OpportunitiesEnabled   bool
	QuotationsEnabled      bool
	QuotationIssueEnabled  bool
	QuotationAcceptEnabled bool
	ContractsEnabled       bool
	ContractActivation     bool
	SLAEnabled             bool
	CreditEnabled          bool

	NotificationWorkerEnabled bool
	QuotationExpiryWorker     bool
	ContractExpiryWorker      bool
	ContractRenewalWorker     bool
	SLAEvaluationWorker       bool
	SLAEscalationWorker       bool

	// KillSwitch disables every CRM mutation and every CRM worker regardless of
	// the individual switches above.
	KillSwitch bool
}

// Options wires the service.
type Options struct {
	Pool          *pgxpool.Pool
	Repository    *repositories.Repository
	Approvals     ApprovalPort
	Media         MediaPort
	Notifications NotificationPort
	Support       SupportPort
	Events        *events.DomainPublisher
	Metrics       *observability.Metrics
	Flags         Flags
	// TokenSecret keys the customer access-token digest. Without it no token
	// can be minted, and quotation issue that requires one fails closed.
	TokenSecret []byte
	// ContactSecret keys the IP and user-agent digests on acceptance evidence.
	ContactSecret  []byte
	TokenTTL       time.Duration
	RenewalWindow  time.Duration
	WorkerMaxRetry int
	Now            func() time.Time
}

// Service implements the Section 14 workflows.
type Service struct {
	opts  Options
	audit *audit.Recorder
}

// NewService builds the service.
//
// It is always constructible. Whether a quotation can actually be issued
// depends on Media being configured and a token secret being present; an
// absent integration makes those paths fail closed and never makes the read
// surface disappear from the API.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.TokenTTL <= 0 {
		opts.TokenTTL = 14 * 24 * time.Hour
	}
	if opts.RenewalWindow <= 0 {
		opts.RenewalWindow = 30 * 24 * time.Hour
	}
	if opts.WorkerMaxRetry <= 0 {
		opts.WorkerMaxRetry = 8
	}
	return &Service{opts: opts, audit: audit.NewRecorder()}
}

// Repository exposes persistence for composition and tests.
func (s *Service) Repository() *repositories.Repository { return s.opts.Repository }

// Flags reports the configured feature switches.
func (s *Service) Flags() Flags { return s.opts.Flags }

// KillSwitchEngaged reports whether every CRM write is disabled.
func (s *Service) KillSwitchEngaged() bool { return s.opts.Flags.KillSwitch }

func (s *Service) now() time.Time { return s.opts.Now().UTC() }

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

func notFound(resource string) error {
	// A cross-tenant identifier and a genuinely missing one get the same
	// answer. Any distinguishable response is an existence oracle that lets one
	// organization enumerate another's customers.
	return apperrors.NotFound(resource+" was not found.", nil)
}

func conflict(code apperrors.Code, message string) error {
	return errorcodes.Conflict(code, message)
}

func badRequest(code apperrors.Code, message string) error {
	return errorcodes.BadRequest(code, message)
}

var errDisabled = apperrors.NotFound("This capability is not available.", nil)

// ErrKillSwitch reports that CRM mutations are globally disabled.
var ErrKillSwitch = errorcodes.New(http.StatusServiceUnavailable,
	errorcodes.QuotationInvalidState,
	"Customer relationship operations are temporarily unavailable.", nil)

// readEnabled gates every CRM read surface.
func (s *Service) readEnabled() error {
	if !s.opts.Flags.CRMEnabled {
		return errDisabled
	}
	return nil
}

// writeEnabled gates a CRM mutation behind the global switch, the kill switch
// and the capability's own flag.
//
// The kill switch is checked before the capability flag on purpose: an operator
// pulling it must stop everything, including capabilities somebody turned on
// five minutes earlier.
func (s *Service) writeEnabled(capability bool) error {
	if s.opts.Flags.KillSwitch {
		return ErrKillSwitch
	}
	if !s.opts.Flags.CRMEnabled || !capability {
		return errDisabled
	}
	return nil
}

// workerEnabled gates a worker pass.
func (s *Service) workerEnabled(capability bool) bool {
	return !s.opts.Flags.KillSwitch && s.opts.Flags.CRMEnabled && capability
}

// translate maps a repository error onto a stable public error.
//
// No SQLSTATE, table name or constraint name crosses this boundary. The caller
// learns what happened in domain terms and nothing about the schema.
func translate(err error, resource string, notFoundCode, conflictCode apperrors.Code) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repositories.ErrNotFound):
		return errorcodes.NotFound(notFoundCode, resource+" was not found.")
	case errors.Is(err, repositories.ErrVersionConflict):
		return conflict(conflictCode, "The record changed since it was read. Re-read and retry.")
	case errors.Is(err, repositories.ErrDuplicate):
		return conflict(conflictCode, "A conflicting record already exists.")
	case errors.Is(err, repositories.ErrImmutable):
		return conflict(conflictCode, "This record is immutable and cannot be changed.")
	case errors.Is(err, repositories.ErrDocumentNotReady):
		return conflict(errorcodes.QuotationDocumentNotReady,
			"A required document is not confirmed safe and ready.")
	default:
		return err
	}
}

// ---------------------------------------------------------------------------
// Transactions
// ---------------------------------------------------------------------------

// withTx runs fn inside a transaction.
//
// Every CRM mutation goes through here so the domain row, its immutable
// history, the audit record and the outbox event either all land or none do.
// An event announcing a change that was rolled back is worse than no event.
func (s *Service) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}

// auditEntry is the platform audit record. It is aliased rather than wrapped so
// the CRM paths that build an entry directly — the customer-token flows, where
// there is no tenant.Context because there is no staff session — use exactly
// the same shape as everything else.
type auditEntry = audit.Entry

// record writes an audit entry inside the caller's transaction.
func (s *Service) record(ctx context.Context, q database.Querier, t tenant.Context,
	action, resourceType, resourceID string, previous, next map[string]any,
	reason string) error {
	entry := audit.EntryFromTenant(t, action, resourceType, resourceID)
	entry.PreviousValues = audit.Redact(previous)
	entry.NewValues = audit.Redact(next)
	entry.Reason = reason
	return s.audit.Record(ctx, q, entry)
}

// publish appends a domain event to the transactional outbox.
//
// Payloads are built field by field by the callers in events.go. Nothing here
// serializes a database model, because a model that grows a field would
// silently start publishing it.
func (s *Service) publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType,
	aggregateID string, aggregateVersion int64, organizationID, correlationID string,
	payload any) error {
	if s.opts.Events == nil {
		return nil
	}
	return s.opts.Events.Publish(ctx, tx, eventType, aggregateType, aggregateID,
		aggregateVersion, organizationID, correlationID, payload)
}

func (s *Service) count(name string, labels map[string]string) {
	if s.opts.Metrics == nil {
		return
	}
	s.opts.Metrics.IncCounter(name, labels, 1)
}

func (s *Service) gauge(name string, labels map[string]string, value float64) {
	if s.opts.Metrics == nil {
		return
	}
	s.opts.Metrics.SetGauge(name, labels, value)
}

// ---------------------------------------------------------------------------
// Fingerprints, hashes and tokens
// ---------------------------------------------------------------------------

// fingerprint produces a stable request fingerprint over the fields that define
// an operation.
//
// Two callers describing the same operation hash the same; any changed value
// hashes differently. That is what makes a retry distinguishable from a new
// request under the same idempotency key.
func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// hashWithSecret keys a digest so a stored hash cannot be reversed by
// dictionary attack against a known input space.
//
// It is used for customer access tokens, and for the IP and user-agent
// fingerprints on acceptance evidence. An acceptance record is evidence that
// somebody accepted, not a tracking log of where they were.
func hashWithSecret(secret []byte, value string) string {
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// mintToken generates a customer access token and its stored digest.
//
// The raw token is returned to the caller exactly once, to be handed to the
// customer. Only the digest is persisted, so a database copy cannot be replayed
// as the customer and a query log cannot leak a working link.
func (s *Service) mintToken() (raw string, hashed string, err error) {
	if len(s.opts.TokenSecret) == 0 {
		return "", "", errorcodes.New(http.StatusServiceUnavailable,
			errorcodes.QuotationTokenInvalid,
			"Secure customer links are not available in this deployment.", nil)
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", fmt.Errorf("crm: mint token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buffer)
	return raw, hashWithSecret(s.opts.TokenSecret, raw), nil
}

// ---------------------------------------------------------------------------
// Shared guards
// ---------------------------------------------------------------------------

// requireCustomer loads a customer and refuses a cross-tenant identifier the
// same way it refuses a missing one.
func (s *Service) requireCustomer(ctx context.Context, q database.Querier, t tenant.Context,
	customerID string) (models.Customer, error) {
	customer, err := s.opts.Repository.GetCustomer(ctx, q, t.OrganizationID, customerID)
	if err != nil {
		return customer, translate(err, "Customer", errorcodes.CustomerNotFound,
			errorcodes.CustomerInvalidState)
	}
	if !t.BranchPermitted(customer.BranchID) {
		// A caller restricted to another branch is told the customer does not
		// exist, not that it exists elsewhere.
		return customer, notFound("Customer")
	}
	return customer, nil
}

// requireCommittableCustomer refuses a customer whose own state forbids a new
// commercial commitment.
func requireCommittableCustomer(customer models.Customer) error {
	blocked, reason := customer.BlocksNewCommitment()
	if !blocked {
		return nil
	}
	switch reason {
	case "customer_archived":
		return conflict(errorcodes.CustomerArchived,
			"This customer is archived and cannot receive new commercial records.")
	case "customer_risk_restricted":
		return conflict(errorcodes.CustomerRiskRestricted,
			"This customer is restricted and cannot receive new commercial records.")
	default:
		return conflict(errorcodes.CustomerOnHold,
			"This customer is on hold and cannot receive new commercial records.")
	}
}

// requireNoBlockingCreditHold refuses when the customer's credit policy blocks
// new commitments.
//
// A missing credit profile is not a block. An organization that has not set a
// credit policy has not decided to refuse the customer, and inventing a refusal
// on their behalf would be Fleet making a commercial decision it does not own.
func (s *Service) requireNoBlockingCreditHold(ctx context.Context, q database.Querier,
	organizationID, customerID string) error {
	profile, err := s.opts.Repository.GetCreditProfile(ctx, q, organizationID, customerID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if models.BlockingHold(profile.HoldStatus) {
		return conflict(errorcodes.CustomerCreditHold,
			"A credit hold on this customer prevents this operation.")
	}
	return nil
}

// requireExpectedVersion refuses a mutation that did not state which version it
// expected to change.
//
// An unconditional write is a lost update waiting to happen: two operators
// editing the same customer would both succeed and one would silently win.
func requireExpectedVersion(expected int64) error {
	if expected <= 0 {
		return badRequest(errorcodes.QuotationVersionConflict,
			"expectedVersion or If-Match is required for this operation.")
	}
	return nil
}

// requireIdempotencyKey refuses a command that did not carry one.
func requireIdempotencyKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return badRequest(errorcodes.QuotationVersionConflict,
			"An Idempotency-Key header is required for this operation.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Media readiness
// ---------------------------------------------------------------------------

// projectMediaState maps an authoritative Media answer onto Fleet's local
// projection.
//
// An unrecognised state becomes UNKNOWN rather than READY. A state Fleet does
// not understand is not a state Fleet may act on, and the difference decides
// whether an unscanned file can be sent to a customer.
func projectMediaState(state platformclients.MediaObjectState) (statusProjection, readiness string) {
	switch strings.ToUpper(strings.TrimSpace(state.DeletionState)) {
	case "DELETED", "PURGED":
		return models.DocStatusDeleted, models.ReadinessNotReady
	case "PENDING", "SCHEDULED":
		return models.DocStatusRevoked, models.ReadinessNotReady
	}
	switch strings.ToUpper(strings.TrimSpace(state.SafetyStatus)) {
	case "UNSAFE", "REJECTED", "INFECTED":
		return models.DocStatusUnsafe, models.ReadinessNotReady
	case "PENDING", "SCANNING":
		return models.DocStatusPending, models.ReadinessNotReady
	case "PENDING_REVIEW", "REVIEW":
		return models.DocStatusPendingReview, models.ReadinessNotReady
	case "SAFE", "CLEAN", "PASSED":
		if strings.EqualFold(strings.TrimSpace(state.UploadStatus), "COMPLETED") ||
			strings.EqualFold(strings.TrimSpace(state.UploadStatus), "READY") {
			return models.DocStatusReady, models.ReadinessReady
		}
		return models.DocStatusPending, models.ReadinessNotReady
	default:
		return models.DocStatusUnknown, models.ReadinessUnknown
	}
}

// readMediaState asks Media about an object.
//
// A nil or unconfigured client is not treated as "probably fine": the caller
// receives UNKNOWN, and every gate that consumes a projection refuses UNKNOWN.
func (s *Service) readMediaState(ctx context.Context, t tenant.Context,
	mediaObjectID string) (statusProjection, readiness string, sourceVersion int64) {
	if s.opts.Media == nil || !s.opts.Media.Configured() {
		return models.DocStatusUnknown, models.ReadinessUnknown, 0
	}
	state, err := s.opts.Media.ReadObject(ctx, platformclients.CallMeta{
		OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, mediaObjectID)
	if err != nil {
		return models.DocStatusUnknown, models.ReadinessUnknown, 0
	}
	statusProjection, readiness = projectMediaState(state)
	return statusProjection, readiness, state.SourceVersion
}

// ---------------------------------------------------------------------------
// Notification intents
// ---------------------------------------------------------------------------

// enqueueNotification records a durable notification intent inside the caller's
// transaction.
//
// The recipient is an opaque contact reference, never an address: the payload
// that reaches the outbox and the worker carries no email, no phone number and
// no customer name. The worker resolves the recipient at delivery time from the
// contact record, under that contact's consent.
func (s *Service) enqueueNotification(ctx context.Context, q database.Querier,
	organizationID, subjectType, subjectID, intentType, contactReference, channel string,
	payload models.JSONMap, idempotencyKey, correlationID, causationID string) error {
	if strings.TrimSpace(contactReference) == "" {
		// No contactable recipient. Recording an intent nobody can receive
		// would manufacture a permanent dead letter on every issue.
		return nil
	}
	operation := models.NotificationOperation{
		ID:                 repositories.PrefixNotification + "_" + fingerprint(idempotencyKey)[:32],
		OrganizationID:     organizationID,
		SubjectType:        subjectType,
		SubjectID:          subjectID,
		IntentType:         intentType,
		RecipientReference: contactReference,
		Channel:            channel,
		Payload:            payload,
		IdempotencyKey:     idempotencyKey,
		CorrelationID:      correlationID,
		CausationID:        causationID,
	}
	_, err := s.opts.Repository.EnqueueNotification(ctx, q, operation)
	return err
}

// notificationChannelFor picks a deliverable channel for a contact.
//
// A stated preference this platform cannot deliver on degrades to email rather
// than being honoured as a promise. Claiming WhatsApp delivery that
// notification-service does not perform would be a silent no-op dressed as a
// success.
func notificationChannelFor(contact models.Contact) string {
	if models.ChannelDeliverable(contact.PreferredChannel) {
		return contact.PreferredChannel
	}
	if contact.NormalizedEmail != "" {
		return models.ChannelEmail
	}
	return models.ChannelInApp
}
