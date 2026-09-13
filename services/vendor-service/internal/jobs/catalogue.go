package jobs

import (
	"fmt"
	"slices"
	"strings"
)

const (
	WorkerActive          = "ACTIVE"
	WorkerDependencyGated = "DEPENDENCY_GATED"
	WorkerNotApplicable   = "NOT_APPLICABLE"
	WorkerFutureDisabled  = "FUTURE_DISABLED"
)

// CatalogueWorker is the operational contract for one Section 45 requirement.
// Implementations may be a coordinator-owned periodic job or a domain-row
// queue whose rows are themselves durable work.
type CatalogueWorker struct {
	Name                string   `json:"name"`
	Implementations     []string `json:"implementations"`
	Package             string   `json:"package"`
	Status              string   `json:"status"`
	Trigger             string   `json:"trigger"`
	JobType             string   `json:"jobType"`
	AuthorityDependency string   `json:"authorityDependency"`
	Durable             bool     `json:"durable"`
	Lease               string   `json:"lease"`
	Heartbeat           string   `json:"heartbeat"`
	MaxAttempts         string   `json:"maxAttempts"`
	Backoff             string   `json:"backoff"`
	DeadLetter          string   `json:"deadLetter"`
	Replay              string   `json:"replay"`
	KillSwitch          string   `json:"killSwitch"`
	ReadinessSignal     string   `json:"readinessSignal"`
	Metrics             []string `json:"metrics"`
	Tests               []string `json:"tests"`
	Notes               string   `json:"notes,omitempty"`
}

// Section45Catalogue maps the specification's worker names to repository-native
// implementations. It does not invent a running job for a policy or authority
// integration that is not configured.
func Section45Catalogue() []CatalogueWorker {
	workers := []CatalogueWorker{
		coordinated("checkout reservation expiry", []string{"commerce-checkout-recovery", "inventory-reservation-expiry"}, "internal/features/checkout; internal/features/inventory", "expiry timestamp", "stock and checkout reservations", "Vendor Inventory", "COMMERCE_CHECKOUT_RECOVERY_ENABLED and COMMERCE_FULFILMENT_WORKERS_ENABLED", []string{"checkout recovery integration", "durable Postgres coordinator"}),
		notApplicable("menu/product scheduler", "internal/features/food; internal/features/shop", "Availability is evaluated from versioned schedules at read time; no mutation worker is required."),
		coordinated("stock alerts", []string{"inventory-stock-alerts", "shop-inventory-alerts-compat"}, "internal/features/inventory; internal/features/shop", "periodic bounded sweep", "convergent LOW_STOCK/OUT_OF_STOCK/EXPIRY/REORDER alerts", "Vendor Inventory", "VENDOR_INVENTORY_ENABLED and COMMERCE_FULFILMENT_WORKERS_ENABLED", []string{"inventory alerts integration", "durable Postgres coordinator"}),
		coordinated("inventory expiry", []string{"inventory-lot-expiry", "shop-inventory-expiry-compat"}, "internal/features/inventory; internal/features/shop", "lot/reservation expiry timestamp", "lot quarantine/expiry and immutable movement", "Vendor Inventory", "VENDOR_INVENTORY_ENABLED and COMMERCE_FULFILMENT_WORKERS_ENABLED", []string{"inventory expiry integration", "inventory commit/release concurrency"}),
		domainQueue("scheduled order activation", []string{"commerce-recurring-feeding", "cloud-kitchen-corporate-materializer"}, "internal/features/orders; internal/features/cloud_kitchen", "scheduled occurrence/window", "version-guarded occurrence materialization", "Vendor commerce and production", "COMMERCE_BULK_RFQ_ENABLED or VENDOR_PRODUCTION_ENABLED", []string{"recurring feeding", "cloud-kitchen occurrence recovery"}),
		domainQueue("capacity release", []string{"pastry-quote-expiry"}, "internal/features/pastry; internal/features/production", "quote/capacity expiry timestamp", "capacity hold release", "Vendor Production", "VENDOR_PRODUCTION_ENABLED", []string{"production capacity concurrency", "pastry quote expiry"}),
		domainQueue("RFQ expiry", []string{"commerce-rfq-expiry"}, "internal/features/orders", "RFQ expiry timestamp", "RFQ lifecycle close", "Vendor commerce", "COMMERCE_BULK_RFQ_ENABLED", []string{"Section 21 RFQ expiry", "RFQ acceptance race"}),
		gatedWorker("order acceptance timeout", "internal/features/orders", "Vendor timeout/replan/refund policy has no enabled canonical policy; no rejection is fabricated."),
		gatedWorker("substitution timeout", "internal/features/orders", "No universal fallback exists; the stored customer-policy path remains authoritative."),
		gatedWorker("fulfilment planner worker", "internal/features/fulfilment; internal/features/orders", "Planning is synchronous and idempotent; no separate readiness-triggered planner queue is configured."),
		gatedWorker("platform mission reconciliation", "internal/features/fulfilmentgroup; internal/features/orders", "Authority-specific mission operations retain stable identities, but no generic cross-authority polling consumer is configured."),
		gatedWorker("handover integration reconciliation", "internal/integrations/handover; internal/features/orders", "Authority adapters perform immediate canonical read-back; no standalone polling worker is configured."),
		coordinated("settlement reconciliation", []string{"commerce-payment-recovery"}, "internal/features/orders", "unresolved Wallet operation", "Wallet status read-back and Vendor projection", "Payment Wallet", "PAYMENT_WALLET_COMMERCE_ENABLED and COMMERCE_FULFILMENT_WORKERS_ENABLED", []string{"Section 17F recovery", "refund recovery"}),
		domainQueue("CRM segment refresh", []string{"crm-segment-materializer"}, "internal/features/crm", "durable dependency-scoped invalidation", "generation build and atomic swap", "Vendor CRM projection", "VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED", []string{"CRM materialization concurrency", "privacy merge fence"}),
		domainQueue("reliability aggregation", []string{"crm-intelligence-materializer"}, "internal/features/crm", "durable evaluation row", "versioned reliability/intelligence generation", "Vendor projection", "VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED", []string{"CRM intelligence generation", "privacy-safe metrics"}),
		domainQueue("search index update", []string{"search-projection-rebuild", "search-projection-validate-activate"}, "internal/features/searchprojection; internal/searchrebuild", "durable projection job", "build-beside-live and atomic generation activation", "Search projection only", "VENDOR_SEARCH_ENABLED", []string{"search generation recovery", "activation race"}),
		coordinated("quote expiry / price-lock release", []string{"commerce-fulfilment-plan-expiry", "commerce-basket-proposal-expiry", "pastry-quote-expiry"}, "internal/features/fulfilment; internal/features/basket; internal/features/pastry", "immutable expiry timestamp", "expire without deleting historical evidence", "Vendor commerce; Wallet only for Wallet holds", "COMMERCE_FULFILMENT_WORKERS_ENABLED", []string{"plan expiry", "basket proposal expiry", "pastry quote expiry"}),
		coordinated("payment reconciliation", []string{"commerce-payment-recovery", "commerce-rfq-payment-recovery"}, "internal/features/orders", "UNKNOWN/unresolved Wallet operation", "same-operation authority read-back", "Payment Wallet", "PAYMENT_WALLET_COMMERCE_ENABLED", []string{"Section 17F recovery", "RFQ payment recovery"}),
		gatedWorker("hybrid delivery fallback", "internal/features/fulfilmentgroup", "The policy-backed command exists, but no automatic SLA-triggered worker is enabled."),
		notApplicable("confidence ETA updater", "internal/features/orders", "ETA updates are event-driven from authoritative logistics revisions and preserve versioned history; a polling scheduler would duplicate that path."),
		domainQueue("verification expiry/recheck", []string{"verification-expiry", "verification-reconciliation"}, "internal/features/vendors", "projection expiry or unresolved authority operation", "expiry enforcement and authoritative recheck", "Verification Compliance", "VENDOR_VERIFICATION_INTEGRATION_ENABLED for recheck", []string{"Section 7 expiry", "verification unknown-outcome recovery"}),
		gatedWorker("variable-weight timeout", "internal/features/orders", "No enabled canonical timeout/fallback policy exists; financial caps remain synchronously enforced."),
		coordinated("recurring basket generator", []string{"commerce-recurring-basket-generation", "commerce-recurring-feeding"}, "internal/features/basket; internal/features/orders", "scheduled occurrence", "unique template/occurrence proposal generation", "Vendor commerce", "COMMERCE_RECURRING_BASKET_ENABLED or COMMERCE_BULK_RFQ_ENABLED", []string{"recurring basket idempotency", "recurring feeding"}),
		domainQueue("vendor intelligence aggregation", []string{"crm-intelligence-materializer"}, "internal/features/crm", "durable evaluation row", "privacy-safe versioned aggregate generation", "Analytics/intelligence projection only", "VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED", []string{"CRM intelligence generation", "privacy fences"}),
		domainQueue("search projection rebuild", []string{"search-projection-rebuild", "search-projection-validate-activate"}, "internal/features/searchprojection; internal/searchrebuild", "operator/invalidation projection job", "checkpointed rebuild and atomic activation", "Search projection only", "VENDOR_SEARCH_ENABLED", []string{"search projection PostgreSQL tests", "failed generation retains live generation"}),
		coordinated("budget/optimization proposal expiry", []string{"commerce-basket-proposal-expiry"}, "internal/features/basket", "proposal expiry timestamp", "stale proposal invalidation", "Vendor commerce", "COMMERCE_BASKET_OPTIMIZATION_ENABLED", []string{"basket proposal expiry", "stale acceptance rejection"}),
		futureWorker("order guarantee evaluator"),
	}
	for i := range workers {
		slices.Sort(workers[i].Implementations)
		slices.Sort(workers[i].Metrics)
		slices.Sort(workers[i].Tests)
	}
	return workers
}

func coordinated(name string, implementations []string, pkg, trigger, jobType, authority, killSwitch string, tests []string) CatalogueWorker {
	return CatalogueWorker{Name: name, Implementations: implementations, Package: pkg, Status: WorkerActive, Trigger: trigger, JobType: jobType, AuthorityDependency: authority, Durable: true, Lease: "vendor_job_controls owner lease with expired-lease recovery", Heartbeat: "renewed every lease/3 by DurableJob", MaxAttempts: "5 (bounded; hard cap 25)", Backoff: "bounded exponential, maximum 5 minutes", DeadLetter: "vendor_job_controls DEAD_LETTER", Replay: "DurableCoordinator.Replay retains job identity", KillSwitch: killSwitch, ReadinessSignal: "DurableCoordinator.Health: READY/DEGRADED/BLOCKED/DISABLED/BACKLOGGED", Metrics: workerMetrics(), Tests: tests}
}

func domainQueue(name string, implementations []string, pkg, trigger, jobType, authority, killSwitch string, tests []string) CatalogueWorker {
	return CatalogueWorker{Name: name, Implementations: implementations, Package: pkg, Status: WorkerActive, Trigger: trigger, JobType: jobType, AuthorityDependency: authority, Durable: true, Lease: "domain-row lease or FOR UPDATE SKIP LOCKED", Heartbeat: "renewed for long generation work; short row transactions do not require renewal", MaxAttempts: "bounded by domain row policy where remote/rebuild work occurs", Backoff: "bounded domain retry schedule", DeadLetter: "domain DEAD_LETTER/FAILED state where retryable work exists", Replay: "domain code-level replay or unchanged due-row recovery", KillSwitch: killSwitch, ReadinessSignal: "domain backlog/staleness/failure state", Metrics: workerMetrics(), Tests: tests}
}

func gatedWorker(name, pkg, note string) CatalogueWorker {
	return CatalogueWorker{Name: name, Package: pkg, Status: WorkerDependencyGated, Trigger: "not enabled", JobType: "reserved compatibility boundary", AuthorityDependency: "existing domain/foreign authority policy", Durable: false, Lease: "N/A while disabled", Heartbeat: "N/A while disabled", MaxAttempts: "N/A while disabled", Backoff: "N/A while disabled", DeadLetter: "N/A while disabled", Replay: "N/A while disabled", KillSwitch: "disabled because no canonical policy/consumer is configured", ReadinessSignal: "DISABLED", Metrics: []string{"vendor_worker_blocked_total"}, Tests: []string{"catalogue validation"}, Notes: note}
}

func notApplicable(name, pkg, note string) CatalogueWorker {
	return CatalogueWorker{Name: name, Package: pkg, Status: WorkerNotApplicable, Trigger: "N/A", JobType: "synchronous/query-time invariant", AuthorityDependency: "existing domain", Durable: false, Lease: "N/A", Heartbeat: "N/A", MaxAttempts: "N/A", Backoff: "N/A", DeadLetter: "N/A", Replay: "deterministic re-read", KillSwitch: "feature flag", ReadinessSignal: "domain readiness", Metrics: []string{}, Tests: []string{"catalogue validation"}, Notes: note}
}

func futureWorker(name string) CatalogueWorker {
	return CatalogueWorker{Name: name, Package: "none", Status: WorkerFutureDisabled, Trigger: "none", JobType: "reserved only", AuthorityDependency: "future Order Guarantee authority", Durable: false, Lease: "N/A", Heartbeat: "N/A", MaxAttempts: "N/A", Backoff: "N/A", DeadLetter: "N/A", Replay: "N/A", KillSwitch: "permanently disabled", ReadinessSignal: "FUTURE_DISABLED", Metrics: []string{}, Tests: []string{"catalogue validation"}, Notes: "No scan, compensation, Wallet call, table, route, or production worker."}
}

func workerMetrics() []string {
	return []string{"vendor_worker_pending_total", "vendor_worker_retry_total", "vendor_worker_dead_letter_total", "vendor_worker_lease_starvation_total", "vendor_worker_oldest_pending_seconds"}
}

func ValidateSection45Catalogue(catalogue []CatalogueWorker) error {
	seen := map[string]bool{}
	for _, worker := range catalogue {
		name := strings.TrimSpace(worker.Name)
		if name == "" || strings.TrimSpace(worker.Package) == "" || strings.TrimSpace(worker.Status) == "" {
			return fmt.Errorf("worker catalogue: incomplete worker %q", name)
		}
		if seen[name] {
			return fmt.Errorf("worker catalogue: duplicate worker requirement %s", name)
		}
		seen[name] = true
		if worker.Status == WorkerActive {
			if !worker.Durable || len(worker.Implementations) == 0 || worker.Lease == "" || worker.Replay == "" || worker.KillSwitch == "" || worker.ReadinessSignal == "" {
				return fmt.Errorf("worker catalogue: active worker %s lacks durable controls", name)
			}
		}
		if worker.Status == WorkerFutureDisabled && len(worker.Implementations) != 0 {
			return fmt.Errorf("worker catalogue: future worker %s claims an implementation", name)
		}
	}
	return nil
}
