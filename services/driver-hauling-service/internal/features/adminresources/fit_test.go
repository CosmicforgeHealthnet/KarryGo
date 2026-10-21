package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func TestHaulingFitDecisionReadsBindHistoricalCandidateParents(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	const provider = "11111111-1111-4111-8111-111111111111"
	const truck = "22222222-2222-4222-8222-222222222222"
	const booking = "33333333-3333-4333-8333-333333333333"
	const org = "44444444-4444-4444-8444-444444444444"
	for _, sql := range []string{
		`INSERT INTO truck_providers(id,first_name) VALUES('11111111-1111-4111-8111-111111111111','PRIVATE_NAME'),('55555555-5555-4555-8555-555555555555','PRIVATE_OTHER')`,
		`INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number) VALUES('22222222-2222-4222-8222-222222222222','11111111-1111-4111-8111-111111111111','flatbed',2000,'PRIVATE_PLATE')`,
		`INSERT INTO haulage_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,corporate_organization_id) VALUES('33333333-3333-4333-8333-333333333333',gen_random_uuid(),'PRIVATE_PICKUP',0,0,'PRIVATE_DROP',0,0,'general',100,'44444444-4444-4444-8444-444444444444')`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	native := truckcapability.NewRepository(db)
	if err := native.RecordDecisions(ctx, []truckcapability.MatchDecision{{BookingID: booking, ProviderID: provider, TruckID: truck, Decision: "ELIGIBLE", ReasonCodes: "PRIVATE_REASON", Evidence: map[string]string{"note": "PRIVATE_EVIDENCE"}, DistanceM: 987654, CapabilityVersion: 7, PolicyVersion: "TEST_ONLY_POLICY", Shadow: true}}); err != nil {
		t.Fatal(err)
	}
	var ref string
	if err := db.QueryRow(ctx, `SELECT id::text FROM hauling_match_decisions WHERE booking_id=$1`, booking).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	source := Source()
	def := source.Specs["fit-decisions"].Definition
	if def.SourceContract != "hauling.admin-fit-decision.v1" {
		t.Fatal(def)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "fit-decisions", ref)
	if err != nil {
		t.Fatal(err)
	}
	item, err := source.Read(ctx, tx, "fit-decisions", ref, true)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(item, def, true); err != nil {
		t.Fatal(err)
	}
	if item.SourceVersion != nil || item.Status == nil || *item.Status != "ELIGIBLE" {
		t.Fatal("historical outcome/revision changed", item)
	}
	if binding.Canonical.Organization != org || binding.Canonical.Tenant != org {
		t.Fatal(binding)
	}
	for _, d := range item.Dimensions {
		if d.Name == "shadow" && (d.Flag == nil || !*d.Flag) {
			t.Fatal(d)
		}
	}
	for _, c := range item.Counters {
		if c.Name == "captured_capability_revision" && (c.Value == nil || *c.Value != 7) {
			t.Fatal(c)
		}
	}
	raw, _ := json.Marshal(item)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "987654") {
		t.Fatal("candidate private evidence leaked")
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	approval := issuer.Approval("TEST_ONLY_FIT_CONTEXT", "fit-decisions", ref, def.Capability, binding, scope)
	old := approval
	old.SourceContract = "hauling.admin-resources.v1"
	if err := store.Import(ctx, issuer.Sign(t, old), time.Now()); err == nil {
		t.Fatal("broad old contract authorized candidate disclosure")
	}
	wrong := approval
	wrong.Scope.Tenant = "foreign-tenant"
	if err := store.Import(ctx, issuer.Sign(t, wrong), time.Now()); err == nil {
		t.Fatal("supplied tenant replaced canonical organization")
	}
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	policy := policyfixture.New(t, scope, Owner, def.Capability, def.SourceContract)
	profile, _ := adminowner.ForOwner(Owner)
	key := []byte(strings.Repeat("r", 32))
	h := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: key, Enabled: true, Definitions: profile.Definitions}
	engine := gin.New()
	h.Register(engine)
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Second), Limit: 10}
	request := httptest.NewRequest("GET", profile.Path+"/resources/fit-decisions/"+ref+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, key, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE_") {
		t.Fatal("signed restriction read", response.Code, response.Body.String())
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "hauling-fit-decision.json"), response.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("unknown captured metadata", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `UPDATE hauling_match_decisions SET policy_version='',truck_id=NULL,capability_version=0 WHERE id=$1`, ref); err != nil {
			t.Fatal(err)
		}
		item, err := source.Read(ctx, tx, "fit-decisions", ref, true)
		if err != nil {
			t.Fatal(err)
		}
		if item.SourceVersion != nil {
			t.Fatal("captured zero counter became complete revision")
		}
		for _, d := range item.Dimensions {
			if d.Name == "policy_revision" && (d.Value != nil || d.Flag != nil) {
				t.Fatal("empty policy became known", d)
			}
		}
		for _, r := range item.References {
			if r.Kind == "truck" || r.Kind == "mission" {
				t.Fatal("missing historical reference inferred", r)
			}
		}
		if _, err := store.Resolve(ctx, tx, "fit-decisions", ref, def.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
			t.Fatal("old context followed changed candidate parent", err)
		}
	})
	for name, sql := range map[string]string{
		"contradictory organization":    `UPDATE haulage_bookings SET winner_organization_id='55555555-5555-4555-8555-555555555555' WHERE id=$2`,
		"foreign truck parent":          `UPDATE trucks SET provider_id='55555555-5555-4555-8555-555555555555' WHERE id='22222222-2222-4222-8222-222222222222' AND $1::text<>'' AND $2::text<>''`,
		"foreign mission":               `UPDATE hauling_match_decisions SET mission_ref='foreign-mission' WHERE id=$1 AND $2::text<>''`,
		"orphan booking":                `UPDATE hauling_match_decisions SET booking_id=gen_random_uuid() WHERE id=$1 AND $2::text<>''`,
		"orphan provider":               `UPDATE hauling_match_decisions SET provider_id=gen_random_uuid() WHERE id=$1 AND $2::text<>''`,
		"changed planning organization": `UPDATE haulage_bookings SET corporate_organization_id='55555555-5555-4555-8555-555555555555' WHERE id=$2 AND $1::text<>''`,
	} {
		t.Run(name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			// All parameters are typed, including the unused opaque decision reference.
			if name == "contradictory organization" {
				sql += ` AND $1::text<>''`
			}
			if _, err := tx.Exec(ctx, sql, ref, booking); err != nil {
				t.Fatal(err)
			}
			if name != "changed planning organization" {
				if _, err := source.Read(ctx, tx, "fit-decisions", ref, true); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal("invalid native parent accepted", err)
				}
			}
			if _, err := store.Resolve(ctx, tx, "fit-decisions", ref, def.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
				t.Fatal("old context followed changed native parent", err)
			}
		})
	}
}
