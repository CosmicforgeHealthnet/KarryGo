package section31contract

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type coverageMatrix struct {
	SchemaVersion int             `json:"schemaVersion"`
	Suites        []coverageSuite `json:"suites"`
}

type coverageSuite struct {
	ID                       string             `json:"id"`
	Suite                    string             `json:"suite"`
	TestLevels               []string           `json:"testLevels"`
	OwningPackages           []string           `json:"owningPackages"`
	Cases                    []string           `json:"cases"`
	Evidence                 []coverageEvidence `json:"evidence"`
	DatabaseRequired         bool               `json:"databaseRequired"`
	RedisRequired            bool               `json:"redisRequired"`
	ExternalServiceRequired  bool               `json:"externalServiceRequired"`
	FailureInjectionRequired bool               `json:"failureInjectionRequired"`
	Mandatory                bool               `json:"mandatory"`
	LatestResult             string             `json:"latestResult"`
}

type coverageEvidence struct {
	Kind     string `json:"kind"`
	TestName string `json:"testName"`
	Path     string `json:"path"`
}

var mandatoryCases = map[string][]string{
	"tenancy-authorization":   split("Cross-organization access denied|Cross-branch access denied|Object-level authorization|Nested-resource authorization|Field-level redaction|Suspended membership denied operational access|Removed membership denied managed access|Independent provider denied organization-managed endpoints|Company user denied platform/internal endpoints|Provider relationship authorization|Download/export/replay IDOR|Safe 404 behavior"),
	"organization-onboarding": split("New organization onboarding|Resume interrupted onboarding|Duplicate submit|Idempotent submission|Verification status changes|Activation prerequisites|Missing prerequisite blocks activation|Rejected verification|Re-submission after rejection|Branch setup where required|Initial role/membership state|Organization suspension|Controlled organization closure|Legal hold during closure|Closure preserves immutable evidence|Closure revokes operational access"),
	"provider-provisioning":   split("New provider identity|Existing provider identity link|Existing provider plus new organization membership|Duplicate phone|Duplicate email|Duplicate external provider reference|Active membership|Suspended membership|Removed membership|Provisioning rollback|Partial provisioning|Reconciliation|Unknown external outcome|Wallet/sub-wallet relationship provisioning|Verification dependency|Idempotent retry|No competing unverified provider identity"),
	"acceptance-decline":      split("Offer/request receive|Assignment receive|Acknowledge where required|Accept|Decline|Every configured decline reason|Timeout|Assignment expiry|Duplicate accept|Duplicate decline|Accept after decline|Decline after accept|Dispatcher/provider race|Two providers racing for conflicting work|Stale version|Cancellation before response|Cancellation during response|Idempotency|Remote acceptance unknown outcome|Read-back confirmed success|Read-back confirmed failure|Independent-provider flow compatibility"),
	"safety-decline":          split("UNSAFE_VEHICLE provider decline|No automatic punishment|Safety alert created|Maintenance/work-order draft created|Asset/vehicle restriction propagates|No automatic fine|No automatic debit|No automatic suspension solely for decline|Safe dispatch/reassignment continues|Duplicate decline has no duplicate work order|Safe audit history"),
	"assignment-reassignment": split("Candidate eligibility|Ineligible provider exclusion|Verification restriction|Asset restriction|Capacity conflict|Existing assignment conflict|Provider decline|Provider timeout|Reassignment|Rescue workflow|No eligible candidate|Excluded previous provider|Concurrent reassignment|Duplicate reassignment prevention|Assignment expiry versus accept race|Trip creation|Trip creation unknown outcome|Read-back before retry|No fabricated trip success"),
	"pricing-visibility":      split("Customer fare absent|Customer contract price absent|Quotation price absent|Company charge absent|Company margin absent|Organization profit absent|Platform commission absent|Organization settlement absent|Company Wallet balance absent|Internal approval notes absent|Internal finance notes absent|Independent provider earnings remain available|Structural DTO serialization enforcement"),
	"wallet-integration":      split("Company settlement reference|Provider sub-wallet allocation|Wallet projection freshness|Idempotent allocation|Duplicate command|Insufficient balance|Approval-required operation|Approved operation|Rejected operation|Unknown outcome|Authoritative read-back|Wallet service unavailable|No fabricated balance|No local Fleet ledger|Provider cannot see organization settlement fields|Independent provider settlement unchanged"),
	"orders-shipments":        split("Valid state transitions|Invalid state transition|Idempotent transition|Order confirmation|Shipment creation|Shipment status progression|Package lifecycle|Package scan idempotency|Duplicate scan|Partial completion|Cancellation|Recurring order generation|Duplicate schedule occurrence prevention|Bulk import validation|Bulk import commit|Corrected-row import|Partial import success|Export|Search projection|Event emission"),
	"dispatch-routes":         split("Candidate generation|Candidate exclusion reasons|Provider capacity|Asset capacity|Time windows|Branch restrictions|Route planning|Publish concurrency|Two dispatchers racing|Route replan|Assignment regeneration|Execution-service unavailable|Unknown execution outcome|Canonical read-back|No local fabricated acceptance"),
	"fleet-maintenance":       split("Asset creation|Verification state|Inspection|Failed inspection block|Safety block|Work-order creation|Work-order lifecycle|Preventive maintenance|Duplicate maintenance prevention|Parts movement|Parts inventory integrity|Return-to-service approval|Return-to-service denied without requirements|Vehicle verification propagation|External vehicle authority retained"),
	"compliance-safety-proof": split("Document expiry warning|Expiry restriction|Compliance blocking|Evidence authorization|Restricted evidence denial|Provider/customer visibility|Incident creation|Incident lifecycle|Corrective action|Legal hold|Proof submission|Proof integrity hash|Media-authoritative metadata|Custody immutability|Archived evidence|Quarantined Media file denied|Audit history"),
	"warehouse":               split("Receive|Put-away|Pick|Pack|Transfer|Concurrent receive|Concurrent pick|Concurrent transfer|Negative stock prevention|Duplicate scan|Lot/batch handling|Quarantine|Dock conflict|Yard conflict|Branch isolation|Warehouse isolation|Inventory reconciliation"),
	"finance":                 split("Minor-unit arithmetic only|Currency handling|Invoice issue|Issued invoice immutability|Reversal/credit note|Duplicate financial command|Expense|Budget|Approval workflow|Payroll calculation|Payroll approval|Settlement projection|Wallet unknown outcome|No local ledger|No automatic deduction because provider declined|No automatic safety-decline punishment|Financial field authorization"),
	"events":                  split("Domain mutation plus outbox atomicity|Publication|Duplicate delivery|Inbox deduplication|Out-of-order event|Lower source version|Same-version conflict|Missing-version gap|Unknown schema version|Unknown event type|Producer allowlist|HMAC nonce replay|Dead-letter|Replay|Reconciliation|Redis restart/reconnect|Analytics unavailable|Core transaction still commits|No event silently lost"),
	"jobs-leases":             split("SKIP LOCKED concurrent claim|Priority|Lease owner enforcement|Heartbeat|Lease expiry|Expiry recovery|Lost lease|Graceful shutdown|Retry|Permanent failure|Dead-letter|Replay|Cancellation|Unknown external outcome|Read-back|Scheduled-task duplicate occurrence prevention|Scheduler DST behavior|Worker restart|No duplicate effect"),
	"security":                split("Token revocation|Organization suspension|Provider suspension|Membership removal|Service HMAC authentication|Route scope|Method scope|Event scope|Service credential rotation|Nonce replay|Rate limits|Distributed Redis rate limiting|Trusted proxies|Secret redaction|Token redaction|Signed URL redaction|File authorization|Media quarantine|Object authorization|Field authorization|Audit append-only|Audit chain verification|Legal hold|Retention authorization|Step-up|Company user denied internal routes"),
	"performance":             split("Large provider fleet|Live dispatch board|High-volume package and warehouse scans|Large exports with bounded memory and Media handoff|Event backlog publication consumption lag and replay|Job backlog throughput lease contention queue delay and shutdown|Dataset size recorded|p50 recorded|p95 recorded|p99 recorded where meaningful|Throughput recorded|Index plan recorded|Pool pressure recorded|Memory recorded|Development-machine label and no production SLA claim"),
	"migration":               split("Fresh database replay 000001 to current tip|Sequential numbering|Up/down pairing|Destructive migration guard|Latest rollback/reapply|Recent-range replay|Upgrade with historical data|Compatibility fixture after upgrade|Bounded-backfill framework|Backfill resume where applicable|No tenant leakage during migration/backfill|Supported PostgreSQL version|Index-safety policy|Enum/check evolution|Legacy provider compatibility after migration"),
}

func split(value string) []string { return strings.Split(value, "|") }

func TestCoverageMatrixMapsEveryMandatoryRequirementToExecutableTests(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "docs", "section31-test-coverage.json"))
	if err != nil {
		t.Fatal(err)
	}
	var matrix coverageMatrix
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatal(err)
	}
	if matrix.SchemaVersion != 1 {
		t.Fatalf("schemaVersion=%d, want 1", matrix.SchemaVersion)
	}
	executable := discoverTests(t, root)
	seen := make(map[string]bool)
	for _, suite := range matrix.Suites {
		if seen[suite.ID] {
			t.Errorf("duplicate suite %q", suite.ID)
		}
		seen[suite.ID] = true
		want, ok := mandatoryCases[suite.ID]
		if !ok {
			t.Errorf("unknown suite %q", suite.ID)
			continue
		}
		if !suite.Mandatory || suite.LatestResult != "PASS" || len(suite.TestLevels) == 0 || len(suite.OwningPackages) == 0 {
			t.Errorf("suite %q lacks mandatory metadata or latest PASS result", suite.ID)
		}
		got := append([]string(nil), suite.Cases...)
		sort.Strings(got)
		want = append([]string(nil), want...)
		sort.Strings(want)
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("suite %q mandatory cases differ\n got: %v\nwant: %v", suite.ID, got, want)
		}
		if len(suite.Evidence) == 0 {
			t.Errorf("suite %q has no evidence", suite.ID)
		}
		for _, evidence := range suite.Evidence {
			if evidence.Kind != "existing" && evidence.Kind != "new" {
				t.Errorf("suite %q evidence %q has invalid kind %q", suite.ID, evidence.TestName, evidence.Kind)
			}
			path, ok := executable[evidence.TestName]
			if !ok {
				t.Errorf("suite %q maps to non-executable test %q", suite.ID, evidence.TestName)
			} else if evidence.Path != "" && filepath.ToSlash(path) != evidence.Path {
				t.Errorf("test %q path=%q, matrix=%q", evidence.TestName, filepath.ToSlash(path), evidence.Path)
			}
		}
	}
	for id := range mandatoryCases {
		if !seen[id] {
			t.Errorf("mandatory suite %q is absent", id)
		}
	}
}

func discoverTests(t *testing.T, root string) map[string]string {
	t.Helper()
	tests := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				rel, _ := filepath.Rel(root, path)
				tests[fn.Name.Name] = rel
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tests
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("fleet-partner-service root not found")
		}
		dir = parent
	}
}
