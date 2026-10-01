package bikecapability

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/policyingest"
)

// PolicyIngestOptions configure Dispatch's publication sink.
type PolicyIngestOptions struct {
	DB *pgxpool.Pool
	// CallerName is this service's identity when reading a referenced
	// catalogue back from the control plane.
	CallerName string
	// AdminSecret is the shared HMAC secret for that read-back.
	AdminSecret []byte
	// AdminHosts is the allowlist a document_ref may name. Empty disables
	// read-back, which parks referenced publications rather than rejecting
	// them -- a missing credential is a deployment gap, not a bad event.
	AdminHosts []string
	// ReconcileInterval is how often parked publications are retried.
	ReconcileInterval time.Duration
	Now               func() time.Time
}

// PolicyIngest is the wired ingestion path.
type PolicyIngest struct {
	ingestor *policyingest.Ingestor
	store    *policyingest.PostgresStore
	interval time.Duration
}

// NewPolicyIngest builds the sink.
//
// It returns nil only when there is no database, because a deployment with no
// Admin credentials must still ACCEPT publications: an inline catalogue needs
// no read-back at all, and a referenced one is parked until the credentials
// arrive. Refusing to mount the route without Admin configured would turn a
// configuration gap into permanently missed policy.
func NewPolicyIngest(options PolicyIngestOptions) *PolicyIngest {
	if options.DB == nil {
		return nil
	}
	store, err := policyingest.NewPostgresStore(options.DB, policyingest.Tables{
		Publications: "dispatch_policy_publications",
		Snapshots:    "dispatch_policy_snapshots",
	})
	if err != nil {
		logging.Error("dispatch-policy", "policy ingest store unavailable: %v", err)
		return nil
	}

	reader := policyingest.NewHTTPReader(policyingest.ReaderOptions{
		CallerName:   options.CallerName,
		Secret:       options.AdminSecret,
		AllowedHosts: options.AdminHosts,
		Now:          options.Now,
	})
	if reader == nil && len(options.AdminHosts) > 0 {
		logging.Notice("dispatch-policy",
			"read-back is not configured; referenced catalogues will be parked until it is")
	}

	ingestor := policyingest.New(policyingest.Options{
		Store:      store,
		KnownKinds: []string{PolicyKindCapability, PolicyKindProduct},
		// A nil *HTTPReader must be passed as a nil INTERFACE, not as a typed
		// nil, or the ingestor's `reader == nil` check never fires and a
		// referenced publication panics instead of parking.
		Reader: readerOrNil(reader),
		Now:    options.Now,
		OnApplied: func(kind, version string) {
			observability.PolicySnapshotApplied(kind)
			logging.Success("dispatch-policy", "applied %s version %s", kind, version)
		},
	})
	return &PolicyIngest{ingestor: ingestor, store: store, interval: options.ReconcileInterval}
}

func readerOrNil(reader *policyingest.HTTPReader) policyingest.DocumentReader {
	if reader == nil {
		return nil
	}
	return reader
}

// Ingestor exposes the sink for route registration.
func (p *PolicyIngest) Ingestor() *policyingest.Ingestor {
	if p == nil {
		return nil
	}
	return p.ingestor
}

// RunReconcileWorker retries publications whose read-back outcome was unknown,
// and publishes the backlog gauge.
//
// The gauge matters as much as the retry: matching keeps working against the
// last good snapshot when the control plane is unreachable, which is correct
// and also completely silent. A growing pending count is the only thing that
// makes that visible.
func (p *PolicyIngest) RunReconcileWorker(ctx context.Context) {
	if p == nil || p.ingestor == nil {
		return
	}
	interval := p.interval
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	p.sweep(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.sweep(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (p *PolicyIngest) sweep(ctx context.Context) {
	if reconciled, err := p.ingestor.ReconcilePending(ctx, 0); err != nil {
		logging.Error("dispatch-policy", "reconcile pending publications: %v", err)
	} else if reconciled > 0 {
		logging.Notice("dispatch-policy", "reconciled %d pending policy publications", reconciled)
	}
	if pending, err := p.store.PendingCount(ctx); err == nil {
		observability.PolicyPublicationsPending(float64(pending))
	}
}

// AdminHostsFrom parses a comma-separated allowlist.
func AdminHostsFrom(raw string) []string {
	var hosts []string
	for _, host := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}
	return hosts
}
