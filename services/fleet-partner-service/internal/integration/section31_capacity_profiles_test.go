package integration_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const section31ProfileRows = 2000

type queryProfile struct {
	name       string
	dataset    int
	query      string
	args       []any
	seedTime   time.Duration
	requireIdx bool
}

func TestSection31Capacity_OperationalProfiles(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	ctx := context.Background()
	pool := f.pool
	organizationID := f.tenant.OrganizationID
	packageRow := f.createPackage(t)

	profiles := []queryProfile{
		seedProviderFleetProfile(t, pool, organizationID),
		seedDispatchBoardProfile(t, pool, organizationID),
		seedPackageScanProfile(t, pool, organizationID, packageRow.ID),
		seedExportProfile(t, pool, organizationID),
		seedEventBacklogProfile(t, pool, organizationID),
	}
	if _, err := pool.Exec(ctx, `ANALYZE fleet_partner_organization_provider_memberships, fleet_partner_marketplace_request_projections, fleet_partner_package_scans, fleet_partner_export_jobs, fleet_partner_outbox`); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	baselineAcquired := pool.Stat().AcquiredConns() // testPool deliberately holds one advisory-lock connection.
	runtime.ReadMemStats(&before)
	for _, profile := range profiles {
		runOperationalProfile(t, pool, profile)
	}
	runtime.ReadMemStats(&after)
	stats := pool.Stat()
	memoryDelta := int64(after.TotalAlloc - before.TotalAlloc)
	t.Logf("CAPACITY development-machine-only summary: profiles=%d rowsPerProfile=%d totalAllocatedBytes=%d poolTotal=%d poolAcquired=%d poolIdle=%d; these observations are smoke evidence, not a production SLA",
		len(profiles), section31ProfileRows, memoryDelta, stats.TotalConns(), stats.AcquiredConns(), stats.IdleConns())
	if stats.AcquiredConns() != baselineAcquired {
		t.Fatalf("connection pool leak after capacity profiles: acquired=%d baseline=%d", stats.AcquiredConns(), baselineAcquired)
	}
}

func seedProviderFleetProfile(t *testing.T, pool *pgxpool.Pool, organizationID string) queryProfile {
	t.Helper()
	started := time.Now()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_organization_provider_memberships
			(id, organization_id, provider_service, service_category, provider_id,
			 onboarding_source, relationship_type, status)
		SELECT 'pmem_capacity_'||g, $1, 'driver-dispatch-delivery-service', 'DISPATCH',
			'provider_capacity_'||g, 'EXISTING_PROVIDER_LINKED', 'CONTRACTOR',
			CASE WHEN g % 20 = 0 THEN 'SUSPENDED' ELSE 'ACTIVE' END
		FROM generate_series(1, $2) AS g`, organizationID, section31ProfileRows)
	if err != nil {
		t.Fatal(err)
	}
	return queryProfile{name: "large-provider-fleet membership-list-and-lookup", dataset: section31ProfileRows,
		query: `SELECT id, provider_id FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND status='ACTIVE' ORDER BY id LIMIT 50`,
		args:  []any{organizationID}, seedTime: time.Since(started), requireIdx: true}
}

func seedDispatchBoardProfile(t *testing.T, pool *pgxpool.Pool, organizationID string) queryProfile {
	t.Helper()
	started := time.Now()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_marketplace_request_projections
			(id, organization_id, source_service, platform_request_id, service_type,
			 projection_status, offered_at)
		SELECT 'mpr_capacity_'||g, $1, 'DISPATCH', 'request_capacity_'||g, 'DELIVERY',
			CASE WHEN g % 10 = 0 THEN 'CANCELLED' ELSE 'OFFERED' END,
			now() - (g||' milliseconds')::interval
		FROM generate_series(1, $2) AS g`, organizationID, section31ProfileRows)
	if err != nil {
		t.Fatal(err)
	}
	return queryProfile{name: "live-dispatch-board active-request-refresh", dataset: section31ProfileRows,
		query: `SELECT id, platform_request_id FROM fleet_partner_marketplace_request_projections WHERE organization_id=$1 AND projection_status='OFFERED' ORDER BY offered_at DESC LIMIT 50`,
		args:  []any{organizationID}, seedTime: time.Since(started), requireIdx: true}
}

func seedPackageScanProfile(t *testing.T, pool *pgxpool.Pool, organizationID string, packageID string) queryProfile {
	t.Helper()
	started := time.Now()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_package_scans
			(id, organization_id, package_id, scan_type, barcode_value, qr_value,
			 location_reference, device_reference, client_scan_id, request_fingerprint,
			 disposition, actor_type, actor_reference, occurred_at)
		SELECT 'pscan_capacity_'||g, $1, $2, 'LOCATION', 'barcode_capacity', '',
			'location_capacity', 'device_capacity', 'client_scan_capacity_'||g,
			'fingerprint_capacity_'||g, 'APPLIED', 'ORGANIZATION_USER', 'capacity-user',
			now() - (g||' milliseconds')::interval
		FROM generate_series(1, $3) AS g`, organizationID, packageID, section31ProfileRows)
	if err != nil {
		t.Fatal(err)
	}
	return queryProfile{name: "high-volume-package-scans recent-scan-and-duplicate-index", dataset: section31ProfileRows,
		query: `SELECT id, client_scan_id FROM fleet_partner_package_scans WHERE organization_id=$1 AND package_id=$2 ORDER BY occurred_at DESC LIMIT 50`,
		args:  []any{organizationID, packageID}, seedTime: time.Since(started), requireIdx: true}
}

func seedExportProfile(t *testing.T, pool *pgxpool.Pool, organizationID string) queryProfile {
	t.Helper()
	started := time.Now()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_export_jobs
			(id, organization_id, export_type, format, status, request_fingerprint,
			 idempotency_key, created_by, expires_at, created_at)
		SELECT 'exp_capacity_'||g, $1, 'ORDERS', 'CSV',
			CASE WHEN g % 10 = 0 THEN 'READY' ELSE 'EXPIRED' END,
			'fingerprint_capacity_'||g, 'export_capacity_'||g, 'capacity-user',
			now() - interval '1 minute', now() - (g||' milliseconds')::interval
		FROM generate_series(1, $2) AS g`, organizationID, section31ProfileRows)
	if err != nil {
		t.Fatal(err)
	}
	return queryProfile{name: "large-export listing-and-expiry", dataset: section31ProfileRows,
		query: `SELECT id, status FROM fleet_partner_export_jobs WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 50`,
		args:  []any{organizationID}, seedTime: time.Since(started), requireIdx: true}
}

func seedEventBacklogProfile(t *testing.T, pool *pgxpool.Pool, organizationID string) queryProfile {
	t.Helper()
	started := time.Now()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_outbox
			(id, event_type, schema_version, aggregate_type, aggregate_id,
			 organization_id, payload, status, available_at, created_at)
		SELECT 'evt_capacity_'||g, 'capacity.profile.v1', 1, 'capacity_profile',
			'aggregate_capacity_'||g, $1, '{}'::jsonb,
			CASE WHEN g % 10 = 0 THEN 'PENDING' ELSE 'PUBLISHED' END,
			now() - interval '1 second', now() - (g||' milliseconds')::interval
		FROM generate_series(1, $2) AS g`, organizationID, section31ProfileRows)
	if err != nil {
		t.Fatal(err)
	}
	return queryProfile{name: "event-backlog pending-publication-claim", dataset: section31ProfileRows,
		query:    `SELECT id FROM fleet_partner_outbox WHERE status='PENDING' AND available_at<=now() ORDER BY available_at LIMIT 100`,
		seedTime: time.Since(started), requireIdx: true}
}

func runOperationalProfile(t *testing.T, pool *pgxpool.Pool, profile queryProfile) {
	t.Helper()
	ctx := context.Background()
	planRows, err := pool.Query(ctx, "EXPLAIN "+profile.query, profile.args...)
	if err != nil {
		t.Fatalf("%s explain: %v", profile.name, err)
	}
	var plan strings.Builder
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	planRows.Close()
	if err := planRows.Err(); err != nil {
		t.Fatal(err)
	}
	if profile.requireIdx && strings.Contains(plan.String(), "Seq Scan") {
		t.Fatalf("%s uses sequential scan at dataset=%d:\n%s", profile.name, profile.dataset, plan.String())
	}

	latencies := make([]time.Duration, 0, 30)
	rowsRead := 0
	started := time.Now()
	for range 30 {
		queryStarted := time.Now()
		rows, err := pool.Query(ctx, profile.query, profile.args...)
		if err != nil {
			t.Fatalf("%s query: %v", profile.name, err)
		}
		for rows.Next() {
			rowsRead++
			values, err := rows.Values()
			if err != nil || len(values) == 0 {
				rows.Close()
				t.Fatalf("%s read: values=%d err=%v", profile.name, len(values), err)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		latencies = append(latencies, time.Since(queryStarted))
	}
	duration := time.Since(started)
	p50, p95, p99 := percentile(latencies, .50), percentile(latencies, .95), percentile(latencies, .99)
	throughput := float64(rowsRead) / duration.Seconds()
	seedThroughput := float64(profile.dataset) / profile.seedTime.Seconds()
	t.Logf("CAPACITY development-machine-only profile=%q dataset=%d seed=%s seedThroughput=%.1f rows/s p50=%s p95=%s p99=%s readThroughput=%.1f rows/s plan=%q",
		profile.name, profile.dataset, profile.seedTime, seedThroughput, p50, p95, p99, throughput, strings.TrimSpace(plan.String()))
	if p99 > time.Second {
		t.Fatalf("%s p99=%s exceeds generous 1s local smoke guard", profile.name, p99)
	}
	if rowsRead == 0 {
		t.Fatalf("%s returned no rows", profile.name)
	}
}
