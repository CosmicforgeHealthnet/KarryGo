package cloudkitchen

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

type occurrenceTestScopes struct{}

func (occurrenceTestScopes) ResolveScope(context.Context, string, string) (production.Scope, error) {
	return production.Scope{Global: true}, nil
}

func TestConcurrentMaterializationCreatesOneOccurrenceReservationAndJob(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres occurrence race")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vendor_users,vendor_businesses RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO vendor_users(id,email) VALUES('user_occurrence_race','occurrence-race@example.test');
		INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('staff_occurrence_race','vendor_occurrence_race','user_occurrence_race','active');
		INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone) VALUES('vendor_occurrence_race','Occurrence Race Ltd','ACTIVE','staff_occurrence_race','NGN','Africa/Lagos');
		INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status) VALUES('branch_occurrence_race','vendor_occurrence_race','Occurrence Race Branch','1 Test Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE');
		INSERT INTO vendor_capabilities(id,business_id,capability_type,status) VALUES('cap_occurrence_race','vendor_occurrence_race','CLOUD_KITCHEN','ACTIVE');
		INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status) VALUES('branch_cap_occurrence_race','vendor_occurrence_race','branch_occurrence_race','cap_occurrence_race','CLOUD_KITCHEN','ACTIVE');
		INSERT INTO production_capacity_pools(id,business_id,branch_id,capability,name,code,scope_type,unit_type,default_capacity,status) VALUES('pool_occurrence_race','vendor_occurrence_race','branch_occurrence_race','CLOUD_KITCHEN','Occurrence capacity','OCCURRENCE_CAP','CAPABILITY','WORKLOAD_UNIT',20,'ACTIVE');
		INSERT INTO cloud_kitchen_packages(id,business_id,branch_id,name,status,minimum_quantity,capacity_pool_id) VALUES('package_occurrence_race','vendor_occurrence_race','branch_occurrence_race','Corporate lunch','ACTIVE',1,'pool_occurrence_race');
		INSERT INTO cloud_kitchen_quantity_bands(id,package_id,min_quantity,unit_price_minor,package_price_minor,capacity_units_per_item,status) VALUES('band_occurrence_race','package_occurrence_race',1,1000,1000,1,'ACTIVE');
		INSERT INTO cloud_kitchen_corporate_plans(id,business_id,branch_id,corporate_profile_ref,name,package_id,quantity,recurrence_type,weekdays,timezone,local_time,effective_from,production_minutes,lead_time_minutes,reservation_horizon_days,status) VALUES('plan_occurrence_race','vendor_occurrence_race','branch_occurrence_race','corp-test','Daily lunch','package_occurrence_race',5,'DAILY','{}','Africa/Lagos','23:00','2026-08-14',60,0,1,'ACTIVE')`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	productionService := production.NewService(production.Options{DB: pool, Scopes: occurrenceTestScopes{}, Flags: production.Flags{ProductionEnabled: true, WritesEnabled: true}, Now: func() time.Time { return now }})
	service := NewService(pool, productionService)
	startRace := make(chan struct{})
	results := make(chan struct {
		count int
		err   error
	}, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-startRace
			count, materializeErr := service.Materialize(context.Background(), "vendor_occurrence_race", 10)
			results <- struct {
				count int
				err   error
			}{count, materializeErr}
		}()
	}
	close(startRace)
	wait.Wait()
	close(results)
	totalCreated := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("materialization race failed: %v", result.err)
		}
		totalCreated += result.count
	}
	if totalCreated != 1 {
		t.Fatalf("materialization workers reported %d created occurrences; want 1", totalCreated)
	}

	checks := map[string]string{
		"cloud_kitchen_occurrences": "plan_id='plan_occurrence_race'",
		"production_jobs":           "source_type='CORPORATE_OCCURRENCE'",
		"capacity_reservations":     "source_type='CORPORATE_OCCURRENCE'",
	}
	for table, where := range checks {
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE `+where).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s has %d rows after materialization race; want 1", table, count)
		}
	}

	if _, err = pool.Exec(ctx, `INSERT INTO cloud_kitchen_occurrences(id,plan_id,occurrence_key,occurrence_date,starts_at,ends_at,quantity,package_id,state) VALUES('occurrence_reschedule_race','plan_occurrence_race','reschedule-race','2026-08-20','2026-08-20 09:00:00+00','2026-08-20 10:00:00+00',5,'package_occurrence_race','PLANNED')`); err != nil {
		t.Fatal(err)
	}
	rescheduleStart := make(chan struct{})
	rescheduleResults := make(chan error, 2)
	windows := [][2]time.Time{
		{time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC), time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)},
		{time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC), time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC)},
	}
	for _, window := range windows {
		window := window
		go func() {
			<-rescheduleStart
			rescheduleResults <- service.RescheduleOccurrence(context.Background(), "vendor_occurrence_race", "plan_occurrence_race", "occurrence_reschedule_race", window[0], window[1], 1)
		}()
	}
	close(rescheduleStart)
	succeeded := 0
	for range 2 {
		if raceErr := <-rescheduleResults; raceErr == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent reschedules produced %d successful writes; want exactly 1", succeeded)
	}
	var occurrenceVersion int64
	if err = pool.QueryRow(ctx, `SELECT version FROM cloud_kitchen_occurrences WHERE id='occurrence_reschedule_race'`).Scan(&occurrenceVersion); err != nil {
		t.Fatal(err)
	}
	if occurrenceVersion != 2 {
		t.Fatalf("reschedule race left occurrence version %d; want 2", occurrenceVersion)
	}
}
