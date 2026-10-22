package testfixture

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// CaseBoundary is invoked with actual owner migrations and actual native case
// rows by the owning packages. It does not replace the source with a stub.
func CaseBoundary(t *testing.T, db *pgxpool.Pool, source adminowner.Source, owner, kind, ref string, active bool) {
	t.Helper()
	ctx := context.Background()
	spec, ok := admincase.Find(owner, kind)
	if !ok {
		t.Fatal("unregistered native case")
	}
	reader := adminowner.CaseSourceReader{Owner: owner, Source: source}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := reader.Binding(ctx, tx, admincase.ContextPrefix+kind, ref)
	item, itemErr := source.Read(ctx, tx, kind, ref, false)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if itemErr != nil {
		t.Fatal(itemErr)
	}
	scope := binding.Canonical
	if scope.Market == "" {
		scope.Market = "M1"
	}
	if scope.Zone == "" {
		scope.Zone = "Z1"
	}
	if scope.Organization == "" {
		scope.Organization = "org-1"
	}
	if scope.Tenant == "" {
		scope.Tenant = "tenant-1"
	}
	policy := policyfixture.New(t, scope, owner, spec.Permission, spec.SourceContract)
	policy.Document.ID = "test-only-case-policy-" + ref
	policy.Sign(t)
	issuer := contextfixture.New(t, owner)
	store := &admincontext.Store{DB: db, Reader: reader, Authority: issuer.Authority}
	key := bytes.Repeat([]byte("c"), 32)
	handler := &adminowner.CaseAuthorityHandler{Owner: owner, Path: spec.Path(), DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: key, Enabled: true}
	router := gin.New()
	handler.RegisterOn(router.Group("/"))
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincase.NewClient(owner, server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1}
	if _, err := client.Authorize(ctx, q, kind, ref, "CASE", ref); err == nil {
		t.Fatal("native existence became source approval")
	}
	approval := issuer.Approval("case-fixture-"+ref, admincase.ContextPrefix+kind, ref, spec.Permission, binding, scope)
	approval.ExpiresAt = time.Now().UTC().Add(2 * time.Minute)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	proof, err := client.Authorize(ctx, q, kind, ref, "CASE", ref)
	if active {
		if err != nil || !proof.ExpiresAt.Equal(approval.ExpiresAt) {
			t.Fatal("actual active case proof unavailable/expiry fabricated", err)
		}
	} else if err == nil {
		t.Fatal("terminal native case authorized")
	}
	if active {
		for subject, roles := range spec.Subjects {
			for _, link := range item.References {
				if admincase.Contains(roles, link.Kind) {
					if _, err := client.Authorize(ctx, q, kind, ref, subject, link.Reference); err != nil {
						t.Fatal("actual native subject membership unavailable", subject, link.Kind, err)
					}
				}
			}
		}
		// EVALUATING is a real open native state in both recovery schemas.
		if kind == "recoveries" && (owner == "driver-taxi-service" || owner == "driver-hauling-service") {
			table := "taxi_recovery_cases"
			if owner == "driver-hauling-service" {
				table = "hauling_recovery_cases"
			}
			if _, err := db.Exec(ctx, `UPDATE `+table+` SET state='EVALUATING' WHERE id::text=$1`, ref); err != nil {
				t.Fatal(err)
			}
			_, evaluationErr := client.Authorize(ctx, q, kind, ref, "CASE", ref)
			_, restoreErr := db.Exec(ctx, `UPDATE `+table+` SET state=$2 WHERE id::text=$1`, ref, *item.Status)
			if evaluationErr != nil || restoreErr != nil {
				t.Fatal("native EVALUATING case proof failed", evaluationErr, restoreErr)
			}
		}
	}
	address := server.URL + spec.Path() + "/proofs/" + kind + "/" + ref + "/CASE/" + ref + "?" + q.Values().Encode()
	for _, caller := range []string{adminread.Caller, admincase.Caller} {
		request, _ := http.NewRequest("GET", address, nil)
		if err := serviceauth.SignRequest(request, caller, key, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		first, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		first.Body.Close()
		if caller == adminread.Caller {
			if first.StatusCode != 401 {
				t.Fatal("Bridge read audience used Media source key")
			}
			continue
		}
		want := 200
		if !active {
			want = 404
		}
		if first.StatusCode != want {
			t.Fatal("native case status denial masked by unrelated failure", first.StatusCode, want)
		}
		replay, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		replay.Body.Close()
		if replay.StatusCode != 409 {
			t.Fatal("source case nonce replay accepted", replay.StatusCode)
		}
	}
	if _, err := client.Authorize(ctx, q, kind, ref, "CASE", "foreign-case"); err == nil {
		t.Fatal("foreign subject authorized")
	}
	wrong := q
	wrong.Scope.Tenant = "foreign-tenant"
	if _, err := client.Authorize(ctx, wrong, kind, ref, "CASE", ref); err == nil {
		t.Fatal("cross-tenant source case authorized")
	}
	wrong = q
	wrong.PolicyVersion++
	if _, err := client.Authorize(ctx, wrong, kind, ref, "CASE", ref); err == nil {
		t.Fatal("stale policy authorized")
	}
	approval.Revision++
	approval.State = "REVOKED"
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Authorize(ctx, q, kind, ref, "CASE", ref); err == nil {
		t.Fatal("revoked native case allocation authorized")
	}
}
