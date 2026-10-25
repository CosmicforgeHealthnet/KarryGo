package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"github.com/jackc/pgx/v5"
)

func TestActualOwnerMigrationsAndSourceDictionary(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	source := Source()
	// Every primary projection must compile against the actual migrated owner
	// schema. An unknown record remains absent; it never becomes a global record.
	for kind, spec := range source.Specs {
		t.Run(kind, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := source.Read(ctx, tx, kind, "00000000-0000-4000-8000-000000000000", false); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("absent source record: %v", err)
			}
			if spec.DimensionQuery != "" {
				var raw []byte
				if err := tx.QueryRow(ctx, spec.DimensionQuery, "00000000-0000-4000-8000-000000000000").Scan(&raw); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestCorporateAdminResourcesAreOwnerBackedAndMinimized(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	org, branch, audit := "org_admin_visibility", "branch_admin_visibility", "audit_admin_visibility"
	for _, statement := range []string{
		`INSERT INTO fleet_partner_organizations(id,legal_name,registration_number,country_code,timezone,default_currency,status) VALUES('org_admin_visibility','Private Legal Name','RC-VISIBILITY','NG','Africa/Lagos','NGN','ACTIVE')`,
		`INSERT INTO fleet_partner_branches(id,organization_id,code,name,address,timezone,status) VALUES('branch_admin_visibility','org_admin_visibility','VIS','Private HQ','private street','Africa/Lagos','ACTIVE')`,
		`INSERT INTO fleet_partner_organization_onboarding_stages(organization_id,stage,status) VALUES('org_admin_visibility','ACTIVATION','COMPLETE')`,
		`INSERT INTO fleet_partner_audit_records(id,organization_id,branch_id,actor_type,actor_id,action,resource_type,resource_id,reason,request_id,correlation_id) VALUES('audit_admin_visibility','org_admin_visibility','branch_admin_visibility','USER','secret-actor','branch.reviewed','branch','branch_admin_visibility','private reason','req-visibility','corr-visibility')`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	source := Source()
	binding, err := source.Binding(ctx, tx, "branches", branch)
	if err != nil || binding.Canonical.Organization != org || binding.Canonical.Tenant != org {
		t.Fatal("branch owner binding", err, binding)
	}
	stage, err := source.Read(ctx, tx, "onboarding-stages", org+":ACTIVATION", false)
	if err != nil || stage.Status == nil || *stage.Status != "COMPLETE" {
		t.Fatal("onboarding stage", err, stage)
	}
	entry, err := source.Read(ctx, tx, "audit-records", audit, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"secret-actor", "private reason", "private street", "Private Legal Name"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("private Fleet field disclosed: %q in %s", forbidden, serialized)
		}
	}
	if source.Specs["manifests"].Definition.Capability != "fleet.private_manifests.read" {
		t.Fatal("private manifests do not have a dedicated capability")
	}
}
