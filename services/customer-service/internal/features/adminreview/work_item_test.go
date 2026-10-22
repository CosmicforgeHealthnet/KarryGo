package adminreview

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/customer-service/internal/features/adminresources"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func workItemFixture(t *testing.T) (reviewFixture, customerreview.Receipt, customerreview.WorkItemCommand) {
	f := setupReview(t)
	ctx := context.Background()
	p := policyfixture.New(t, f.scope, customerreview.Owner, customerreview.WorkItemCapability, customerreview.WorkItemContract)
	for _, entry := range []struct{ cap, contract string }{{customerreview.WorkItemReadCapability, customerreview.WorkItemContract}, {customerreview.RequestCapability, customerreview.Contract}, {customerreview.ReadCapability, customerreview.StateContract}} {
		rule := p.Document.Rules[0]
		rule.Capability = entry.cap
		rule.Permission = entry.cap
		rule.SourceContract = entry.contract
		p.Document.Rules = append(p.Document.Rules, rule)
	}
	p.Sign(t)
	f.service.Policy = p.Engine
	f.actor.Permissions = append(f.actor.Permissions, customerreview.WorkItemCapability, customerreview.WorkItemReadCapability)
	f.approval.Revision++
	f.approval.Capabilities = append(f.approval.Capabilities, customerreview.WorkItemCapability, customerreview.WorkItemReadCapability)
	if err := f.service.Contexts.Import(ctx, f.issuer.Sign(t, f.approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	intake, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input)
	if err != nil {
		t.Fatal(err)
	}
	f.service.WorkItemsEnabled = true
	c := customerreview.WorkItemCommand{ContractVersion: customerreview.WorkItemContract, OperationID: uuid.NewString(), ReviewRequestID: intake.ReviewRequestID, CustomerID: f.input.CustomerID, ExpectedVersion: 1, Action: "ACKNOWLEDGE", IdempotencyKey: uuid.NewString(), ReasonCode: "REVIEW_INTAKE", RequestedAt: time.Now().UTC().Add(-time.Second)}
	return f, intake, c
}
func TestPostgresCustomerWorkItemLifecycleConcurrentIdentityAndOriginalReadback(t *testing.T) {
	f, intake, c := workItemFixture(t)
	ctx := context.Background()
	var group sync.WaitGroup
	results := make(chan customerreview.WorkItemReceipt, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			out, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, c)
			if err != nil {
				t.Error(err)
				return
			}
			results <- out
		}()
	}
	group.Wait()
	close(results)
	var first customerreview.WorkItemReceipt
	for r := range results {
		if first.OperationID == "" {
			first = r
		} else if r != first {
			t.Fatal("changed original proof")
		}
	}
	if first.State != "ACKNOWLEDGED" || first.RecordedVersion != 2 {
		t.Fatal(first)
	}
	bad := c
	bad.ReasonCode = "CHANGED"
	if _, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, bad); err == nil {
		t.Fatal("changed intent accepted")
	}
	bad = c
	bad.OperationID = uuid.NewString()
	bad.IdempotencyKey = uuid.NewString()
	if _, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, bad); err == nil {
		t.Fatal("fresh key bypassed stale version")
	}
	close := c
	close.OperationID = uuid.NewString()
	close.IdempotencyKey = uuid.NewString()
	close.Action = "CLOSE"
	close.ExpectedVersion = 2
	out, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, close)
	if err != nil || out.Outcome != "REVIEW_CLOSED_WITHOUT_ACCOUNT_CHANGE" || out.RecordedVersion != 3 {
		t.Fatal(out, err)
	}
	f.service.WriteEnabled = false
	f.service.WorkItemsEnabled = false
	original, err := f.service.WorkItemReadback(ctx, f.actor, "case-1", c.CustomerID, c.IdempotencyKey, f.scope)
	if err != nil || original != first {
		t.Fatal("write kill lost original readback", original, err)
	}
	old, err := f.service.Receipt(ctx, f.actor, "case-1", c.CustomerID, f.input.IdempotencyKey, f.scope)
	if err != nil || old != intake {
		t.Fatal("original intake changed", old, err)
	}
	var account, queue string
	var revision int64
	err = f.service.DB.QueryRow(ctx, `SELECT c.status,s.status,s.version FROM customers c JOIN customer_admin_review_state s ON s.customer_id=c.id WHERE c.id=$1`, c.CustomerID).Scan(&account, &queue, &revision)
	if err != nil || account != "active" || queue != "IDLE" || revision != 3 {
		t.Fatal("account mutation or incorrect queue revision", account, queue, revision, err)
	}
	tx, err := f.service.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	native := adminresources.Source()
	binding, err := native.Binding(ctx, tx, "review-work-items", intake.ReviewRequestID)
	if err != nil || binding.Canonical != f.scope {
		t.Fatal("work item lost immutable original scope", binding, err)
	}
	resource, err := native.Read(ctx, tx, "review-work-items", intake.ReviewRequestID, true)
	tx.Rollback(ctx)
	if err != nil || resource.Status == nil || *resource.Status != "CLOSED" || resource.SourceVersion == nil || *resource.SourceVersion != 3 {
		t.Fatal("owner queue state/version unavailable", resource, err)
	}
	if _, err := f.service.DB.Exec(ctx, `UPDATE customer_admin_review_transitions SET staff_ref='forged'`); err == nil {
		t.Fatal("mutable transition evidence")
	}
}
func TestPostgresCustomerWorkItemAuditFailureScopeDenialAndTypedTransport(t *testing.T) {
	f, _, c := workItemFixture(t)
	ctx := context.Background()
	foreign := f.scope
	foreign.Tenant = "foreign"
	if _, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", foreign, c); err == nil {
		t.Fatal("cross tenant")
	}
	bad := c
	bad.CustomerID = uuid.NewString()
	if _, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, bad); err == nil {
		t.Fatal("case customer substitution")
	}
	f.service.WorkItemsEnabled = false
	if _, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, c); err == nil {
		t.Fatal("disabled transition")
	}
	f.service.WorkItemsEnabled = true
	if _, err := f.service.DB.Exec(ctx, `CREATE FUNCTION fail_work_item_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'TEST_ONLY_AUDIT_FAILURE';END$$;CREATE TRIGGER fail_work_item_audit BEFORE INSERT ON customer_admin_review_transitions FOR EACH ROW EXECUTE FUNCTION fail_work_item_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.TransitionWorkItem(ctx, f.actor, "case-1", f.scope, c); err == nil {
		t.Fatal("audit failed open")
	}
	var version int
	if err := f.service.DB.QueryRow(ctx, `SELECT version FROM customer_admin_review_work_items WHERE request_id=$1`, c.ReviewRequestID).Scan(&version); err != nil || version != 1 {
		t.Fatal("queue changed on audit failure", version, err)
	}
	f.service.DB.Exec(ctx, `DROP TRIGGER fail_work_item_audit ON customer_admin_review_transitions`)
	writeKey, readKey := bytes.Repeat([]byte{85}, 32), bytes.Repeat([]byte{86}, 32)
	root := gin.New()
	(&Handler{Service: f.service, WriteSecret: writeKey, ReadSecret: readKey}).Register(root)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			r.Body = http.NoBody
		} else {
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		recorded := httptest.NewRecorder()
		root.ServeHTTP(recorded, r)
		for k, values := range recorded.Header() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(recorded.Code)
		w.Write(recorded.Body.Bytes())
		if r.Method == "POST" && recorded.Code == 200 {
			if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
				os.MkdirAll(dir, 0700)
				if err := os.WriteFile(filepath.Join(dir, "customer-work-item-command.json"), body, 0600); err != nil {
					t.Error(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "customer-work-item-receipt.json"), recorded.Body.Bytes(), 0600); err != nil {
					t.Error(err)
				}
			}
		}
	}))
	defer server.Close()
	client, err := customerreview.NewClient(server.URL, writeKey, readKey)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: f.scope.Market, Zone: f.scope.Zone, Organization: f.scope.Organization, Tenant: f.scope.Tenant}, Staff: f.actor.Staff, Role: "staff", Purpose: "case-1", PolicyID: f.service.Policy.ID(), PolicyVersion: f.service.Policy.Version(), PolicyChecksum: f.service.Policy.Checksum(), Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC(), Limit: 1}
	receipt, err := client.TransitionWorkItem(ctx, q, c)
	if err != nil {
		t.Fatal(err)
	}
	read, err := client.WorkItemReadback(ctx, q, c.CustomerID, c.IdempotencyKey)
	if err != nil || read != receipt {
		t.Fatal(read, err)
	}
	f.approval.Revision++
	f.approval.State = "REVOKED"
	if err := f.service.Contexts.Import(ctx, f.issuer.Sign(t, f.approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WorkItemReadback(ctx, q, c.CustomerID, c.IdempotencyKey); err == nil {
		t.Fatal("revoked readback")
	}
	raw, _ := json.Marshal(c)
	for _, body := range []string{strings.Replace(string(raw), `"action":"ACKNOWLEDGE"`, `"action":"ACKNOWLEDGE","action":"CLOSE"`, 1), strings.Replace(string(raw), `"action"`, `"Action"`, 1), strings.TrimSuffix(string(raw), "}") + `,"status":"blocked"}`} {
		if _, err := customerreview.DecodeWorkItemCommand([]byte(body)); err == nil {
			t.Fatal("ambiguous command accepted")
		}
	}
}
