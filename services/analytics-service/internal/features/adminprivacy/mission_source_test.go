package adminprivacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/gin-gonic/gin"
)

func TestPostgresMissionSourceCountsDistinctApprovedNativeUnitsAndWithholdsIncompleteCoverage(t *testing.T) {
	db := lineageDB(t)
	ctx := context.Background()
	fixture, actor, q, _ := privacyFixture(t)
	q.Capability = analyticsadmin.MissionSuccessPermission
	fixture.Document.Rules[0].Capability = q.Capability
	fixture.Document.Rules[0].Permission = q.Capability
	actor.Permissions = []string{q.Capability}
	q.Filters["product_code"] = networkanalytics.ProductTaxiPassenger
	owner := "driver-taxi-service"
	issuer := contextfixture.New(t, owner)
	source := &MissionSuccessSource{Authorities: map[string]admincontext.Authority{owner: issuer.Authority}, UnitField: "provider_id"}
	fixture.Document.Rules[0].SourceContract = source.SourceContract()
	fixture.Document.Analytics.AllowedCellKeys = []string{"completed", "failed", "cancelled", "guarantee_breaches"}
	fixture.Document.Analytics.AllowedFilterValues["product_code"] = []string{networkanalytics.ProductTaxiPassenger}
	fixture.Sign(t)
	reader := LineageReader{Owner: owner}
	store := admincontext.Store{DB: db, Reader: reader, Authority: issuer.Authority}
	publishKey := bytes.Repeat([]byte{94}, 32)
	publisher := &LineagePublisher{handlers: map[string]*adminowner.Handler{owner: {Owner: owner, DB: db, PublishEnabled: true, PublishSecret: publishKey, Contexts: &store}}}
	publicationHTTP := gin.New()
	publisher.Register(publicationHTTP)
	publicationServer := httptest.NewServer(publicationHTTP)
	defer publicationServer.Close()
	publicationClient, err := analyticsadmin.NewPublisherClient(publicationServer.URL, owner, publishKey)
	if err != nil {
		t.Fatal(err)
	}
	defer publicationClient.Close()
	kind, _ := LineageKind("mission-success", "provider_id")
	native := usecases.NewService(repositories.NewRepository(db))
	payload, _ := json.Marshal(map[string]any{"provider_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"})
	var lastApproval admincontext.Approval
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("native-mission-%d", i)
		if _, err := native.Ingest(ctx, owner, events.Envelope{EventID: id, EventName: "taxi.booking.completed", SchemaVersion: 1, SourceService: owner, SourceEntityType: "booking", SourceEntityID: id, SourceVersion: 1, OccurredAt: q.Start.Add(10 * time.Minute), Payload: payload}); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		binding, err := reader.Binding(ctx, tx, kind, id)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		approval := issuer.Approval("allocation-"+id, kind, id, "analytics.cohorts.mission-success.export", binding, q.Scope)
		lastApproval = approval
		if err := publicationClient.Publish(ctx, issuer.Sign(t, approval)); err != nil {
			t.Fatal("producer SDK publication", err)
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Snapshot(ctx, tx, q)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Partial || snapshot.Facts.Scope != q.Scope || snapshot.Facts.SourceContract != source.SourceContract() {
		t.Fatal("invented complete coverage or source authorization", snapshot)
	}
	for _, cell := range snapshot.Cells {
		if cell.Key == "completed" && (cell.Count == nil || *cell.Count != 6 || cell.CohortSize == nil || *cell.CohortSize != 1) {
			t.Fatal("event count inflated distinct provider cohort", cell)
		}
	}
	service := Service{DB: db, Policy: fixture.Engine, Source: source}
	release, err := service.Disclose(ctx, actor, q)
	if err != nil {
		t.Fatal(err)
	}
	if release.Coverage != "PARTIAL_COVERAGE" {
		t.Fatal("source membership falsely became complete")
	}
	for _, cell := range release.Cells {
		if cell.Value != nil {
			t.Fatal("incomplete source membership released a value", cell)
		}
	}
	// General source scope still resolves from the other five authorized facts.
	engine := gin.New()
	secret := bytes.Repeat([]byte{81}, 32)
	(&Handler{Enabled: true, Secret: secret, Service: &service}).Register(engine)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := httptest.NewRecorder()
		engine.ServeHTTP(recorded, r)
		for key, values := range recorded.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(recorded.Code)
		w.Write(recorded.Body.Bytes())
		dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR")
		if dir == "" {
			dir = os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR")
		}
		if dir != "" && recorded.Code == 200 {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Error(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "scoped-analytics-owner.json"), recorded.Body.Bytes(), 0600); err != nil {
				t.Error(err)
			}
		}
	}))
	defer server.Close()
	client, err := analyticsadmin.NewClient(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	meta := adminread.Query{Scope: adminread.Scope{Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}, Staff: actor.Staff, Role: actor.Roles[0], Purpose: q.Purpose, PolicyID: fixture.Engine.ID(), PolicyVersion: fixture.Engine.Version(), PolicyChecksum: fixture.Engine.Checksum(), Start: q.Start, End: q.End, Limit: 1}
	wire, err := client.MissionSuccess(ctx, meta, analyticsadmin.Filters{Authority: q.Filters["authority"], ProductCode: q.Filters["product_code"]})
	if err != nil || wire.Coverage != "PARTIAL_COVERAGE" || wire.SourceVersion != release.SourceVersion {
		t.Fatal("actual native scoped HTTP/SDK did not preserve frozen suppression", wire, err)
	}
	wrongRole := meta
	wrongRole.Role = "other"
	if _, err := client.MissionSuccess(ctx, wrongRole, analyticsadmin.Filters{Authority: q.Filters["authority"], ProductCode: q.Filters["product_code"]}); !errors.Is(err, analyticsadmin.ErrDenied) {
		t.Fatal("native report ignored current role", err)
	}
	// General source scope still resolves from the other five authorized facts.
	// The frozen release must separately reauthorize its sixth original member.
	lastApproval.Revision++
	lastApproval.State = "REVOKED"
	if err := store.Import(ctx, issuer.Sign(t, lastApproval), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Disclose(ctx, actor, q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("cached release survived original-member revocation", err)
	}
	var denied int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM analytics_admin_query_audit WHERE outcome='FROZEN_MEMBERSHIP_DENIED'`).Scan(&denied); err != nil || denied != 1 {
		t.Fatal("original-membership denial not audited", denied, err)
	}
	for _, sql := range []string{`UPDATE analytics_admin_disclosure_authorizations SET authorization_payload='{}'`, `DELETE FROM analytics_admin_disclosure_authorizations`, `TRUNCATE analytics_admin_disclosure_authorizations`} {
		if _, err := db.Exec(ctx, sql); err == nil {
			t.Fatal("original membership evidence mutable", sql)
		}
	}
	// Supplying another tenant, product or absent unit field cannot repurpose
	// these native approvals into a different cohort source.
	for label, alter := range map[string]func(*Query){"tenant": func(x *Query) { x.Scope.Tenant = "foreign" }, "product": func(x *Query) { x.Filters = map[string]string{"authority": "TAXI", "product_code": "TAXI_GOODS"} }} {
		t.Run(label, func(t *testing.T) {
			changed := q
			alter(&changed)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := source.Snapshot(ctx, tx, changed); !errors.Is(err, ErrUnmapped) {
				t.Fatalf("unapproved cohort selection accepted: %v", err)
			}
		})
	}
	source.UnitField = ""
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := source.Snapshot(ctx, tx, q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("privacy unit silently defaulted", err)
	}
}
