package secureadmin_test

import (
	"cosmicforge/logistics/services/support-dispute-service/internal/features/adminfeed"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/services/support-dispute-service/internal/testdb"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminowner"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/outbox"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func eventGuard(t *testing.T, h *harness, feedPurpose bool) adminfeed.ScopeGuard {
	p := policyfixture.New(t, h.scope, adminfeed.SourceService, "support.cases.read", "support.admin-resources.v1")
	if feedPurpose {
		p.Document.Rules[0].Purposes = append(p.Document.Rules[0].Purposes, "hq_event_sync")
		p.Sign(t)
	}
	return adminfeed.ScopeGuard{Owner: &adminowner.Handler{Owner: adminfeed.SourceService, DB: h.pool, Contexts: h.store, Policy: p.Engine}}
}
func scopedEvent(t *testing.T, h *harness, ref string) adminevents.SourceEvent {
	var raw []byte
	if err := h.pool.QueryRow(h.ctx, `SELECT support_admin_case_event($1)::text`, ref).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	ev, err := adminevents.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}
func createFeedCase(t *testing.T, h *harness) string {
	c, err := h.support.CreateComplaint(h.ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "test-" + uuid.NewString(), ServiceType: supportmodels.ServiceTypeTaxi, Subject: "PRIVATE SUBJECT", Description: "PRIVATE MESSAGE"})
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}
func TestSupportEventScopeRequiresCurrentSignedAllocationAndFeedPurpose(t *testing.T) {
	h := newHarness(t, nil)
	ref := createFeedCase(t, h)
	guard := eventGuard(t, h, true)
	before := scopedEvent(t, h, ref)
	rawBefore, _ := json.Marshal(before)
	if err := guard.Export(h.ctx, outbox.Record{ID: before.EventID, EventType: before.EventType, Payload: rawBefore}); err == nil {
		t.Fatal("unallocated source event was exported")
	}
	h.importContext("cases", ref)
	ev := scopedEvent(t, h, ref)
	raw, _ := json.Marshal(ev)
	if ev.Scope.Market != h.scope.Market || ev.Scope.Tenant != h.scope.Tenant {
		t.Fatal("explicit allocation not selected")
	}
	if err := guard.Export(h.ctx, outbox.Record{ID: ev.EventID, EventType: ev.EventType, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	if err := eventGuard(t, h, false).Export(h.ctx, outbox.Record{ID: ev.EventID, EventType: ev.EventType, Payload: raw}); err == nil {
		t.Fatal("review policy treated as feed permission")
	}
	if err := guard.Export(h.ctx, outbox.Record{ID: "wrong-event", EventType: ev.EventType, Payload: raw}); err == nil {
		t.Fatal("outbox identity did not match its immutable event")
	}
	if err := guard.Export(h.ctx, outbox.Record{ID: ev.EventID, EventType: "wrong.type", Payload: raw}); err == nil {
		t.Fatal("outbox type did not match its immutable event")
	}
	var organization *string
	if err := h.pool.QueryRow(h.ctx, `SELECT organization_reference FROM complaints WHERE id=$1`, ref).Scan(&organization); err != nil || organization != nil {
		t.Fatalf("business ownership was changed: %v", err)
	}
	var envelope []byte
	if err := h.pool.QueryRow(h.ctx, `SELECT v.envelope FROM admin_owner_context_heads h JOIN admin_owner_context_history v USING(owner,context_id,revision,checksum) WHERE h.resource_kind='cases' AND h.resource_ref=$1`, ref).Scan(&envelope); err != nil {
		t.Fatal(err)
	}
	var e admincontext.Envelope
	var a admincontext.Approval
	if err := json.Unmarshal(envelope, &e); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(e.Document, &a); err != nil {
		t.Fatal(err)
	}
	a.Revision++
	a.State = "REVOKED"
	if err := h.store.Import(h.ctx, h.issuer.Sign(t, a), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := guard.Export(h.ctx, outbox.Record{ID: ev.EventID, EventType: ev.EventType, Payload: raw}); err == nil {
		t.Fatal("revoked allocation remained exportable")
	}
	var oldRaw []byte
	if err := h.pool.QueryRow(h.ctx, `SELECT payload::text FROM admin_event_outbox WHERE aggregate_id=$1 ORDER BY created_at LIMIT 1`, ref).Scan(&oldRaw); err != nil {
		t.Fatal(err)
	}
	old, err := adminevents.Decode(oldRaw)
	if err != nil || old.Scope.Market != "" || old.EventID == ev.EventID {
		t.Fatal("existing source event was rewritten")
	}
}
func TestSupportEventScopeRejectsAmbiguityTamperingAndChangedCanonicalLinks(t *testing.T) {
	h := newHarness(t, nil)
	ref := createFeedCase(t, h)
	h.importContext("cases", ref)
	guard := eventGuard(t, h, true)
	ev := scopedEvent(t, h, ref)
	raw, _ := json.Marshal(ev)
	if _, err := h.pool.Exec(h.ctx, `UPDATE admin_owner_context_heads SET market='FORGED' WHERE resource_ref=$1`, ref); err != nil {
		t.Fatal(err)
	}
	forged := scopedEvent(t, h, ref)
	forgedRaw, _ := json.Marshal(forged)
	if err := guard.Export(h.ctx, outbox.Record{ID: forged.EventID, EventType: forged.EventType, Payload: forgedRaw}); err == nil {
		t.Fatal("head metadata bypassed signature verification")
	}
	if _, err := h.pool.Exec(h.ctx, `UPDATE admin_owner_context_heads SET market=$2 WHERE resource_ref=$1`, ref, h.scope.Market); err != nil {
		t.Fatal(err)
	}
	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := h.store.Reader.Binding(h.ctx, tx, "cases", ref)
	tx.Rollback(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	other := h.scope
	other.Tenant = "OTHER-TEST-TENANT"
	a := h.issuer.Approval("ambiguous-"+uuid.NewString(), "cases", ref, "support.cases.read", binding, other)
	if err = h.store.Import(h.ctx, h.issuer.Sign(t, a), time.Now()); err != nil {
		t.Fatal(err)
	}
	ambiguous := scopedEvent(t, h, ref)
	if ambiguous.Scope.Market != "" {
		t.Fatal("ambiguous allocation selected")
	}
	if _, err = h.pool.Exec(h.ctx, `UPDATE complaints SET organization_reference='DIFFERENT-OWNER' WHERE id=$1`, ref); err != nil {
		t.Fatal(err)
	}
	if err = guard.Export(h.ctx, outbox.Record{ID: ev.EventID, EventType: ev.EventType, Payload: raw}); err == nil {
		t.Fatal("stale record binding remained exportable")
	}
}

func TestSupportEventScopeMigrationCanReapplyAndReverseWithoutBusinessChanges(t *testing.T) {
	h := newHarness(t, nil)
	ref := createFeedCase(t, h)
	h.importContext("cases", ref)
	original := scopedEvent(t, h, ref)
	if original.Scope.Market != h.scope.Market {
		t.Fatal("missing allocated scope")
	}
	down, err := os.ReadFile(filepath.Join(testdb.MigrationsDir(), "down", "010_admin_event_authorization_scope.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.pool.Exec(h.ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if ev := scopedEvent(t, h, ref); ev.Scope.Market != "" || ev.Scope.Tenant != "" {
		t.Fatal("rollback invented native scope")
	}
	up, err := os.ReadFile(filepath.Join(testdb.MigrationsDir(), "010_admin_event_authorization_scope.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = h.pool.Exec(h.ctx, string(up)); err != nil {
			t.Fatal(err)
		}
	}
	restored := scopedEvent(t, h, ref)
	if restored.Scope != original.Scope {
		t.Fatal("reapply did not restore allocated scope")
	}
	if n := h.count(`SELECT count(*) FROM admin_event_outbox WHERE aggregate_id=$1`, ref); n != 1 {
		t.Fatal("migration rewrote or replayed historical events")
	}
}
