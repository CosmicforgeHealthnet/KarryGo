package migrations_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAssignmentAndManifestHardRaceConstraints(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	group, mission := uuid.NewString(), uuid.NewString()
	provider1, provider2 := uuid.NewString(), uuid.NewString()
	for i, id := range []string{provider1, provider2} {
		if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone) VALUES($1,$2)`, id, fmt.Sprintf("+15550000%d", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,source_type,source_ref,source_version,state) VALUES($1,'PARTNER_API',$2,1,'READY_FOR_DISPATCH')`, group, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,source_version,state) VALUES($1,$2,'PARTNER_API',$3,1,'PLANNED')`, mission, group, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	c1, c2 := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_assignment_candidates(id,mission_id,provider_id,rank,distance_meters) VALUES($1,$2,$3,1,100),($4,$2,$5,2,200)`, c1, mission, provider1, c2, provider2); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_assignment_offers(mission_id,candidate_id,provider_id,offered_at,expires_at) VALUES($1,$2,$3,$4,$5)`, mission, c1, provider1, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_assignment_offers(mission_id,candidate_id,provider_id,offered_at,expires_at) VALUES($1,$2,$3,$4,$5)`, mission, c2, provider2, now, now.Add(time.Minute)); err == nil {
		t.Fatal("second live offer accepted")
	}
	parcel := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_parcels(id,delivery_group_id,parcel_ref,karrygo_parcel_ref,scannable_code,package_policy_version) VALUES($1,$2,'P-1',$3,$4,'v1')`, parcel, group, "KGP-"+parcel, "kgp_scan-"+parcel); err != nil {
		t.Fatal(err)
	}
	manifest := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_parcel_manifests(id,mission_id,manifest_ref,version) VALUES($1,$2,'M-1',1)`, manifest, mission); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_active_custody_manifest_parcels(parcel_id,manifest_id,mission_id) VALUES($1,$2,$3)`, parcel, manifest, mission); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_active_custody_manifest_parcels(parcel_id,manifest_id,mission_id) VALUES($1,$2,$3)`, parcel, manifest, mission); err == nil {
		t.Fatal("parcel admitted to two active custody memberships")
	}
	auditID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_assignment_audit(id,mission_id,provider_id,strategy,actor_ref,reason_code,result,correlation_id) VALUES($1,$2,$3,'ADMIN_ASSISTED','admin:test','RECOVERY','OFFERING','corr')`, auditID, mission, provider1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dispatch_assignment_audit SET result='CHANGED' WHERE id=$1`, auditID); err == nil {
		t.Fatal("assignment audit update was not rejected")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM dispatch_assignment_audit WHERE id=$1`, auditID); err == nil {
		t.Fatal("assignment audit delete was not rejected")
	}
}
