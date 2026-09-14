// Package managedsettlement lets an execution service find out whether a
// completed corporate job's money actually moved.
//
// It is a read-only contract by design. Taxi, Dispatch and Hauling complete
// work and report it to Fleet; Fleet resolves compensation and instructs
// Wallet. None of the three may instruct a settlement, and nothing here
// computes an amount, a share or a fee -- the figures come back exactly as
// Wallet recorded them.
//
// The reason an execution service needs this read at all: its own completion
// handling can end ambiguously (a timeout, a lost response, a restart between
// the completion and the settlement). A timeout is not a confirmed failure,
// and guessing either way is how a job gets paid twice or never. So the
// service asks the money authority what happened instead.
package managedsettlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Canonical source-service names, as Fleet and Wallet record them.
const (
	SourceTaxi     = "TAXI"
	SourceDispatch = "DISPATCH"
	SourceHauling  = "HAULING"
)

// Settlement statuses, mirroring payment-wallet-service's own vocabulary.
//
// These are not re-derived or re-interpreted: an execution service compares
// against them and does nothing else with them.
const (
	StatusPending                = "PENDING"
	StatusPosted                 = "POSTED"
	StatusFailedRetryable        = "FAILED_RETRYABLE"
	StatusFailedTerminal         = "FAILED_TERMINAL"
	StatusReconciliationRequired = "RECONCILIATION_REQUIRED"
	StatusReversed               = "REVERSED"
)

// Settlement is one posted (or attempted) managed settlement, as Wallet holds
// it.
//
// Only the fields an execution service can legitimately act on are decoded.
// The shares are present because a service reconciling its own job may need to
// log what moved; they are never used to compute anything.
type Settlement struct {
	InstructionID      string `json:"instruction_id"`
	InstructionVersion int64  `json:"instruction_version"`

	OrganizationID string `json:"organization_id"`
	MembershipID   string `json:"membership_id"`
	ProviderID     string `json:"provider_id"`

	SourceService      string `json:"source_service"`
	SourceJobID        string `json:"source_job_id"`
	SourceAssignmentID string `json:"source_assignment_id,omitempty"`

	Currency               string `json:"currency"`
	GrossBasisMinor        int64  `json:"gross_basis_minor"`
	ProviderShareMinor     int64  `json:"provider_share_minor"`
	OrganizationShareMinor int64  `json:"organization_share_minor"`
	PlatformShareMinor     int64  `json:"platform_share_minor"`

	Status              string `json:"status"`
	FailureCode         string `json:"failure_code,omitempty"`
	LedgerTransactionID string `json:"ledger_transaction_id,omitempty"`
	IdempotencyKey      string `json:"idempotency_key"`
}

// Settled reports whether this settlement moved money that still stands.
//
// A reversed settlement is deliberately not settled: its money was moved and
// then moved back, so a caller asking "is this job paid?" must hear no.
func (s Settlement) Settled() bool { return s.Status == StatusPosted }

// Outcome is what an execution service learns about one completed job.
//
// It separates three things that are easy to collapse and must not be: the
// money moved, the money definitively did not move, and Wallet could not be
// asked. The third is not a failure and must never be treated as one.
type Outcome struct {
	// Settled is true when at least one settlement for this job is POSTED.
	Settled bool

	// Pending is true when a settlement exists but has not posted -- Fleet has
	// instructed and Wallet has not finished, or a reconciliation is open.
	Pending bool

	// Unknown is true when Wallet could not be reached or answered
	// ambiguously. Nothing may be concluded from it, and in particular no
	// independent payout may be made on the strength of it.
	Unknown bool

	// Settlements is everything Wallet holds for the job, newest first as
	// Wallet returns them. Empty means Fleet has not instructed yet, which is
	// a normal state moments after completion.
	Settlements []Settlement
}

// Resolved reports whether the outcome is a definite answer either way.
func (o Outcome) Resolved() bool { return !o.Unknown }

// Client reads managed settlements from payment-wallet-service.
//
// BaseURL is the service's bare origin (for example http://localhost:8105).
// The client appends the full internal path itself, so a caller cannot
// accidentally double the /api/v1/payment-wallet prefix -- a mistake this
// platform has actually shipped before.
type Client struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
}

// Configured reports whether this client can be used at all.
//
// A service with managed settlement switched on but no credentials must not
// silently behave as though every job is unsettled.
func (c Client) Configured() bool {
	return strings.TrimSpace(c.BaseURL) != "" &&
		strings.TrimSpace(c.ServiceName) != "" && len(c.Secret) > 0
}

// ErrNotConfigured is returned when a read is attempted without credentials.
var ErrNotConfigured = errors.New("managedsettlement: client is not configured")

// ForJob asks Wallet what happened to one completed job.
//
// A transport or ambiguous failure returns an Unknown outcome and a non-nil
// error, never an empty "nothing settled" answer: the caller must be able to
// tell "Wallet says no settlement exists" from "Wallet could not be asked".
func (c Client) ForJob(ctx context.Context, sourceService, sourceJobID string) (Outcome, error) {
	if !c.Configured() {
		return Outcome{Unknown: true}, ErrNotConfigured
	}
	sourceService = strings.TrimSpace(sourceService)
	sourceJobID = strings.TrimSpace(sourceJobID)
	if sourceService == "" || sourceJobID == "" {
		return Outcome{Unknown: true}, errors.New("managedsettlement: source service and job id are required")
	}

	var body struct {
		Settlements []Settlement `json:"settlements"`
	}
	path := "/api/v1/payment-wallet/internal/finance/v1/managed-settlements/by-job/" +
		url.PathEscape(sourceService) + "/" + url.PathEscape(sourceJobID)
	if err := c.get(ctx, path, &body); err != nil {
		return Outcome{Unknown: true}, err
	}
	return outcomeOf(body.Settlements), nil
}

// outcomeOf classifies what Wallet returned.
//
// One POSTED settlement makes the job settled even if other rows failed: a
// retried instruction leaves its earlier failed attempts behind, and those are
// history, not a reason to pay again.
func outcomeOf(settlements []Settlement) Outcome {
	outcome := Outcome{Settlements: settlements}
	for _, settlement := range settlements {
		switch settlement.Status {
		case StatusPosted:
			outcome.Settled = true
		case StatusPending, StatusReconciliationRequired:
			outcome.Pending = true
		}
	}
	if outcome.Settled {
		outcome.Pending = false
	}
	return outcome
}

func (c Client) get(ctx context.Context, path string, data any) error {
	endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if err := serviceauth.SignRequest(request, c.ServiceName, c.Secret, []byte{}, time.Now()); err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("managedsettlement: decode response: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		message := envelope.Error.Message
		if message == "" {
			message = "managed settlement read failed"
		}
		return fmt.Errorf("managedsettlement: %s (status %d)", message, response.StatusCode)
	}
	if data == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, data)
}
