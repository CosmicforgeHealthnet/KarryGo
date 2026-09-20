package assetcapability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// AuthorityEndpoint is one driver service's certification ingress.
//
// Path is stored whole rather than derived, because the three services do not
// share one. Taxi and Hauling mount their internal groups under their own API
// base; Dispatch mounts its commerce-internal group at the root. Deriving a
// path from a base URL would have to encode that asymmetry as a rule, and the
// rule would be wrong the first time a service moved its group.
type AuthorityEndpoint struct {
	BaseURL string
	Path    string
	Secret  []byte
}

// Configured reports whether this authority can actually be reached.
func (e AuthorityEndpoint) Configured() bool {
	return strings.TrimSpace(e.BaseURL) != "" && len(e.Secret) > 0
}

// URL is the absolute endpoint.
func (e AuthorityEndpoint) URL() string {
	return strings.TrimRight(e.BaseURL, "/") + e.Path
}

// DefaultPaths are each authority's certification ingress, as mounted today.
func DefaultPaths() map[Authority]string {
	return map[Authority]string{
		AuthorityTaxi:     "/api/v1/taxi/internal/verification/certification-events",
		AuthorityDispatch: "/api/v1/internal/verification/certification-events",
		AuthorityHauling:  "/api/v1/hauling/internal/verification/certification-events",
	}
}

// CallbackWorker delivers certification decisions to the authority that owns
// the asset.
//
// Delivery is durable rather than fire-and-forget for a specific reason: the
// decision has already been committed here, so a lost callback does not undo
// it, it desynchronises it. The asset would be suspended in the certifying
// authority and eligible in the dispatching one, and no later read would
// reconcile them because neither side is polling the other.
type CallbackWorker struct {
	db        *pgxpool.Pool
	endpoints map[Authority]AuthorityEndpoint
	client    *http.Client
}

// NewCallbackWorker builds the delivery worker.
func NewCallbackWorker(db *pgxpool.Pool, endpoints map[Authority]AuthorityEndpoint, timeout time.Duration) *CallbackWorker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	configured := map[Authority]AuthorityEndpoint{}
	for authority, endpoint := range endpoints {
		if endpoint.Configured() {
			configured[authority] = endpoint
		}
	}
	return &CallbackWorker{db: db, endpoints: configured, client: &http.Client{Timeout: timeout}}
}

// Targets lists the authorities this worker can actually reach.
func (w *CallbackWorker) Targets() []Authority {
	out := []Authority{}
	for _, authority := range Authorities() {
		if _, ok := w.endpoints[authority]; ok {
			out = append(out, authority)
		}
	}
	return out
}

type deliverable struct {
	ID       string
	Target   string
	Payload  []byte
	Attempts int
}

// maxDeliveryAttempts bounds retries before a row is dead-lettered for an
// operator. Eight attempts on the backoff below spans roughly four hours, which
// is long enough to ride out a deploy and short enough that a genuinely broken
// endpoint surfaces the same working day.
const maxDeliveryAttempts = 8

// errNotDeliverable means the authority's ingress has no representation for
// this status. It is a terminal, non-retryable outcome, distinct from a
// delivery that failed and should be tried again.
var errNotDeliverable = errors.New("assetcapability: authority has no ingress word for this status")

// Process claims a batch of undelivered certification events and delivers them.
func (w *CallbackWorker) Process(ctx context.Context, owner string, limit int) error {
	if w == nil || w.db == nil || len(w.endpoints) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 25
	}
	rows, err := w.db.Query(ctx, `WITH due AS (
		SELECT id FROM verification_outbox
		 WHERE event_type=$1
		   AND delivery_status IN ('PENDING','FAILED')
		   AND next_attempt_at<=now()
		   AND (lease_expires_at IS NULL OR lease_expires_at<now())
		 ORDER BY created_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE verification_outbox o
		   SET lease_owner=$3, lease_expires_at=now()+interval '30 seconds',
		       delivery_attempts=o.delivery_attempts+1, updated_at=now()
		  FROM due WHERE o.id=due.id
		RETURNING o.id::text, o.delivery_target, o.payload, o.delivery_attempts`,
		EventServiceTierStatusChanged, limit, owner)
	if err != nil {
		return err
	}
	var batch []deliverable
	for rows.Next() {
		var record deliverable
		if err = rows.Scan(&record.ID, &record.Target, &record.Payload, &record.Attempts); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, record := range batch {
		deliverErr := w.deliver(ctx, record)
		if errors.Is(deliverErr, errNotDeliverable) {
			// The authority's ingress has no word for this status, so there is
			// nothing honest to send. Retrying would never succeed, and leaving
			// it pending would make a healthy outbox look stuck.
			_, _ = w.db.Exec(ctx, `UPDATE verification_outbox
				SET delivery_status='NOT_REQUIRED', lease_owner=NULL, lease_expires_at=NULL,
				    last_error=NULL, updated_at=now()
				WHERE id=$1`, record.ID)
			continue
		}
		if deliverErr == nil {
			_, _ = w.db.Exec(ctx, `UPDATE verification_outbox
				SET delivery_status='DELIVERED', delivered_at=now(), lease_owner=NULL,
				    lease_expires_at=NULL, last_error=NULL, updated_at=now()
				WHERE id=$1`, record.ID)
			continue
		}
		status := "FAILED"
		if record.Attempts >= maxDeliveryAttempts {
			status = "DEAD_LETTER"
		}
		backoff := time.Duration(1<<min(record.Attempts, 10)) * time.Minute
		_, _ = w.db.Exec(ctx, `UPDATE verification_outbox
			SET delivery_status=$2, next_attempt_at=now()+$3::interval, lease_owner=NULL,
			    lease_expires_at=NULL, last_error=$4, updated_at=now()
			WHERE id=$1`, record.ID, status,
			fmt.Sprintf("%d seconds", int64(backoff.Seconds())), safeError(deliverErr))
	}
	return nil
}

// safeError keeps a downstream service's error text out of this service's
// stored state. The operator needs to know delivery failed and to which
// authority; the remote body may carry detail that is not ours to retain.
func safeError(err error) string {
	if err == nil {
		return ""
	}
	return "CERTIFICATION_CALLBACK_FAILED"
}

func (w *CallbackWorker) deliver(ctx context.Context, record deliverable) error {
	authority, err := ParseAuthority(record.Target)
	if err != nil {
		return err
	}
	endpoint, ok := w.endpoints[authority]
	if !ok {
		return fmt.Errorf("assetcapability: %s has no configured certification endpoint", authority)
	}
	var canonical map[string]any
	if err = json.Unmarshal(record.Payload, &canonical); err != nil {
		return err
	}
	canonical["event_id"] = record.ID

	adapted := AdaptPayload(authority, canonical)
	if adapted == nil {
		return errNotDeliverable
	}
	body, err := json.Marshal(adapted)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if correlation, _ := canonical["correlation_id"].(string); correlation != "" {
		request.Header.Set("X-Correlation-ID", correlation)
	}
	if err = serviceauth.SignRequest(request, ServiceName, endpoint.Secret, body, time.Now()); err != nil {
		return err
	}
	response, err := w.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("assetcapability: %s certification ingress returned %d", authority, response.StatusCode)
	}
	return nil
}

// AdaptPayload converts the canonical event into the body one authority's
// ingress actually accepts.
//
// The three services shipped three shapes: Taxi keys the asset as vehicle_id
// and speaks a decision word (CERTIFIED/REVOKED/...), while Dispatch and
// Hauling key it as bike_id and truck_id and take the eligibility status
// directly. None of them is wrong and all three are already live, so the
// difference is absorbed here rather than forced on them -- renaming a shipped
// internal contract to tidy up this function would break a working integration
// for cosmetic symmetry.
func AdaptPayload(authority Authority, canonical map[string]any) map[string]any {
	status, _ := canonical["status"].(string)
	adapted := map[string]any{
		authority.AssetFieldName(): canonical["asset_id"],
		"product_code":             canonical["product_code"],
		"policy_version":           canonical["policy_version"],
		"certification_ref":        canonical["certification_ref"],
		"reason_code":              canonical["reason_code"],
		"source_version":           canonical["source_version"],
	}
	if value, ok := canonical["effective_at"]; ok {
		adapted["effective_at"] = value
	}
	if value, ok := canonical["expires_at"]; ok {
		adapted["expires_at"] = value
	}

	if authority == AuthorityTaxi {
		word := CertificationStatus(status).TaxiDecisionWord()
		if word == "" {
			// Taxi has no PENDING in its ingress vocabulary. Sending a
			// substitute would tell Taxi something the reviewer never decided,
			// so nothing is sent and the row is not deliverable.
			return nil
		}
		adapted["decision"] = word
		adapted["event_id"] = canonical["event_id"]
		if facts, ok := canonical["verified_facts"]; ok {
			adapted["verified_facts"] = facts
		}
		return adapted
	}

	adapted["status"] = status
	return adapted
}

// Replay makes an operator-selected dead letter eligible immediately.
func (w *CallbackWorker) Replay(ctx context.Context, eventID string) error {
	tag, err := w.db.Exec(ctx, `UPDATE verification_outbox
		SET delivery_status='PENDING', delivery_attempts=0, next_attempt_at=now(),
		    lease_owner=NULL, lease_expires_at=NULL, last_error=NULL, updated_at=now()
		WHERE id=$1 AND event_type=$2 AND delivery_status='DEAD_LETTER'`,
		eventID, EventServiceTierStatusChanged)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
