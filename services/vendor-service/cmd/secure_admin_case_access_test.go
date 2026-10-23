package main

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVendorSecureAdminNativeCaseMembershipForMedia(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	id := "CASE-" + uuid.NewString()
	e.enroll(t, "branches", e.f.env.branchA)
	command := e.cmd("OPEN_INVESTIGATION", "branches", e.f.env.branchA, id, 0, map[string]string{"category": "PRODUCTION_INCIDENT", "severity": "HIGH"})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, command); err != nil {
		t.Fatal(err)
	}
	spec, _ := admincase.Find("vendor-service", "cases")
	p := policyfixture.New(t, e.scope, spec.Owner, spec.Permission, spec.SourceContract)
	reader := adminowner.CaseSourceReader{Owner: spec.Owner, Source: adminresources.Source()}
	contexts := &admincontext.Store{DB: e.f.env.pool, Authority: e.issuer.Authority, Reader: reader}
	tx, err := e.f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := reader.Binding(ctx, tx, admincase.ContextPrefix+spec.Kind, id)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := e.issuer.Approval("media-case-"+uuid.NewString(), admincase.ContextPrefix+spec.Kind, id, spec.Permission, binding, e.scope)
	if err = contexts.Import(ctx, e.issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte("v"), 32)
	handler := &adminowner.CaseAuthorityHandler{Owner: spec.Owner, Path: spec.Path(), DB: e.f.env.pool, Source: adminresources.Source(), Contexts: contexts, Policy: p.Engine, Secret: key, Enabled: true}
	router := gin.New()
	handler.RegisterOn(router.Group(""))
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincase.NewClient(spec.Owner, server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: e.scope.Market, Zone: e.scope.Zone, Organization: e.scope.Organization, Tenant: e.scope.Tenant}, Staff: e.actor.Staff, Role: "staff", Purpose: "case-1", PolicyID: p.Engine.ID(), PolicyVersion: p.Engine.Version(), PolicyChecksum: p.Engine.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1}
	for _, s := range []struct{ kind, id string }{{"CASE", id}, {"BUSINESS", e.f.env.businessA}, {"BRANCH", e.f.env.branchA}} {
		proof, err := client.Authorize(ctx, q, spec.Kind, id, s.kind, s.id)
		if err != nil {
			t.Fatal("native membership denied", s, err)
		}
		if s.kind == "CASE" {
			captureVendorAdminWire(t, "vendor-case-proof.json", proof)
		}
		if proof.NativeFingerprint == "" || !proof.ExpiresAt.After(time.Now()) {
			t.Fatal("invalid native proof")
		}
	}
	if _, err = client.Authorize(ctx, q, spec.Kind, id, "BRANCH", e.f.env.branchB); err == nil {
		t.Fatal("foreign branch membership accepted")
	}
	bad := q
	bad.Scope.Tenant = "foreign"
	if _, err = client.Authorize(ctx, bad, spec.Kind, id, "CASE", id); err == nil {
		t.Fatal("foreign tenant accepted")
	}
	bad = q
	bad.Staff = e.f.staffB
	if _, err = client.Authorize(ctx, bad, spec.Kind, id, "CASE", id); err == nil {
		t.Fatal("foreign native staff accepted")
	}
	e.f.env.exec(ctx, `UPDATE vendor_staff SET status='suspended' WHERE id=$1`, q.Staff)
	if _, err = client.Authorize(ctx, q, spec.Kind, id, "CASE", id); err == nil {
		t.Fatal("revoked native staff still receives proof")
	}
	e.f.env.exec(ctx, `UPDATE vendor_staff SET status='active' WHERE id=$1`, q.Staff)
	e.f.env.exec(ctx, `UPDATE vendor_admin_cases SET state='CLOSED',version=version+1 WHERE case_ref=$1`, id)
	if _, err = client.Authorize(ctx, q, spec.Kind, id, "CASE", id); err == nil {
		t.Fatal("closed case still receives proof")
	}
}
