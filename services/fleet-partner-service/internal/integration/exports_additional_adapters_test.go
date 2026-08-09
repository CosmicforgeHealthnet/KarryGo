package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	exportsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	exportsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/repositories"
	exportsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	safetyfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	warehousefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// TestExports_WarehouseInventoryAdapterGeneratesRealCSV proves the WP5
// gap-close end to end: a real warehouse inventory row, read through the
// real warehouse feature's own List call (not a fake Exporter), lands in a
// generated export file with the mapped columns.
//
// The warehouse/location/item rows are seeded directly in SQL rather than
// through the full receipt workflow: that write path is pre-existing,
// already-tested warehouse feature code this change did not touch. What
// this test is responsible for proving is new: the export adapter's
// Rows/Columns mapping and its wiring into the real exports service.
func TestExports_WarehouseInventoryAdapterGeneratesRealCSV(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Exports Warehouse Co", "RC-EXP-WH-1")
	ctx := context.Background()

	branchID := "brh_" + randomHex()
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_branches (id, organization_id, code, name, timezone)
		VALUES ($1, $2, $1, 'Main Branch', 'Africa/Lagos')`, branchID, org.ID); err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	warehouseID := "wh_" + randomHex()
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_warehouses
			(id, organization_id, branch_id, code, name, address_reference, timezone, status)
		VALUES ($1, $2, $3, $1, 'Main Warehouse', 'Lagos', 'Africa/Lagos', 'ACTIVE')`,
		warehouseID, org.ID, branchID); err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	locationID := "loc_" + randomHex()
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_storage_locations
			(id, organization_id, warehouse_id, code, name, location_type, status)
		VALUES ($1, $2, $3, $1, 'Bin 1', 'BIN', 'ACTIVE')`,
		locationID, org.ID, warehouseID); err != nil {
		t.Fatalf("seed location: %v", err)
	}
	itemID := "itm_" + randomHex()
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_inventory_items
			(id, organization_id, warehouse_id, location_id, owner_type, owner_reference, sku,
			 quantity_projection, reserved_quantity, unit, status)
		VALUES ($1, $2, $3, $4, 'ORGANIZATION', $2, 'SKU-EXPORT-TEST', 42, 10, 'EACH', 'AVAILABLE')`,
		itemID, org.ID, warehouseID, locationID); err != nil {
		t.Fatalf("seed inventory item: %v", err)
	}

	metrics := observability.NewMetrics()
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	warehouseService := warehousefeature.NewService(warehousefeature.Options{
		Pool: pool, Events: publisher,
		Flags: warehousefeature.Flags{Warehouses: true, Inventory: true},
	})

	// The real adapter (internal/app.NewWarehouseInventoryExporter), wired
	// directly rather than through app.BuildExportsService, so the media
	// backend can be the existing in-package fake instead of a real
	// media-file-service HTTP stub — the media write/download path itself is
	// exercised elsewhere (TestExports_WorkerGeneratesFileAndDownloadReauthorizes);
	// what this test proves is new: the WAREHOUSE_INVENTORY adapter's own
	// Rows/Columns mapping against a real warehouse-feature read.
	media := newFakeExportMedia()
	exportsService := exportsusecases.NewService(exportsusecases.Options{
		Pool:       pool,
		Repository: exportsrepos.NewRepository(pool),
		Exporters: map[string]exportsusecases.Exporter{
			exportsmodels.TypeWarehouseInventory: app.NewWarehouseInventoryExporter(warehouseService),
		},
		Media:   media,
		Metrics: metrics,
		Flags:   exportsusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true},
		MaxRows: 100, RetentionTTL: 24 * time.Hour,
	})

	member := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "wh-export-requester")
	// PermittedBranchIDs must be a non-nil empty slice, not the zero value: a
	// real HTTP-resolved tenant.Context (middleware.ResolveTenant) always
	// produces one, and the warehouse feature's ListItems query binds it
	// unconditionally as ANY($n::text[]) — cardinality(NULL::text[]) is NULL,
	// not 0, so a nil slice there silently excludes every row instead of
	// meaning "no branch restriction."
	actor := tenant.Context{OrganizationID: org.ID, OrganizationUserID: member,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: member, PermittedBranchIDs: []string{}}

	job, err := exportsService.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: actor, ExportType: exportsmodels.TypeWarehouseInventory, IdempotencyKey: "wh-export-1",
	})
	if err != nil {
		t.Fatalf("request warehouse inventory export: %v", err)
	}

	result, err := exportsService.RunGenerationPass(ctx, exportsusecases.WorkerConfig{
		Lease: time.Minute, MaxAttempts: 8,
	})
	if err != nil {
		t.Fatalf("generation pass: %v", err)
	}
	if result.Claimed != 1 || result.Completed != 1 {
		t.Fatalf("generation result = %+v, want 1 claimed and 1 completed", result)
	}

	completed, err := exportsService.Get(ctx, org.ID, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if completed.Status != exportsmodels.StatusReady {
		t.Fatalf("status = %s, want READY", completed.Status)
	}
	if completed.RowCount != 1 {
		t.Fatalf("rowCount = %d, want 1", completed.RowCount)
	}

	content := media.content(completed.MediaFileID)
	if content == nil {
		t.Fatal("no file content was written to media")
	}
	got := string(content)
	wantHeader := "id,warehouseId,locationId,sku,quantityProjection,reservedQuantity,unit,status,createdAt,updatedAt"
	wantFields := itemID + "," + warehouseID + "," + locationID + ",SKU-EXPORT-TEST,42,10,EACH,AVAILABLE,"
	if !strings.Contains(got, wantHeader) || !strings.Contains(got, wantFields) {
		t.Fatalf("csv content = %q, want header %q and fields %q", got, wantHeader, wantFields)
	}
}

func TestSection25RemainingExportAdaptersAreScopedBoundedAndMinimized(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Exports Remaining Co", "RC-EXP-REST-1")
	other := createOrganization(t, pool, "Exports Other Co", "RC-EXP-REST-2")
	ctx := context.Background()
	branch := "brh_" + randomHex()
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone) VALUES($1,$2,$1,'Export Branch','UTC')`, branch, org.ID); err != nil {
		t.Fatal(err)
	}
	customer := "cus_" + randomHex()
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_company_customers(id,organization_id,branch_id,customer_type,legal_name,display_name,normalized_name,duplicate_fingerprint,status,default_currency,created_by) VALUES($1,$2,$3,'BUSINESS','Export Customer','Export Customer',$4,$5,'ACTIVE','NGN','seed')`, customer, org.ID, branch, "export customer "+customer, customer); err != nil {
		t.Fatal(err)
	}
	warehouseID := "wh_" + randomHex()
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_warehouses(id,organization_id,branch_id,code,name,address_reference,timezone,status) VALUES($1,$2,$3,$1,'Export Warehouse','address','UTC','ACTIVE')`, warehouseID, org.ID, branch); err != nil {
		t.Fatal(err)
	}
	incidentID := "inc_" + randomHex()
	invoiceID := "inv_" + randomHex()
	fulfilmentID := "ful_" + randomHex()
	supportID := "scp_" + randomHex()
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_incidents(id,organization_id,branch_id,incident_type,severity,status,occurred_at,description,reported_by,external_reference) VALUES($1,$2,$3,'OPERATIONAL','HIGH','OPEN',now(),'safe summary','operator','INC-EXPORT')`, incidentID, org.ID, branch); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_invoices(id,organization_id,branch_id,customer_id,currency,subtotal_minor,tax_total_minor,adjustment_total_minor,total_minor,due_date,status,created_by) VALUES($1,$2,$3,$4,'NGN',1000,0,0,1000,current_date,'ISSUED','seed')`, invoiceID, org.ID, branch, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_receivable_entries(id,organization_id,invoice_id,original_amount_minor,outstanding_amount_minor,currency,due_at) VALUES($1,$2,$3,1000,1000,'NGN',now()+interval '7 days')`, "rcv_"+randomHex(), org.ID, invoiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_fulfilment_orders(id,organization_id,warehouse_id,customer_reference,source_order_id,priority,status) VALUES($1,$2,$3,$4,'ORD-EXPORT','HIGH','RELEASED')`, fulfilmentID, org.ID, warehouseID, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_support_case_projections(id,organization_id,support_service_case_id,case_type,status,priority,source_version,last_synced_at) VALUES($1,$2,'CASE-EXPORT','PLATFORM','OPEN','HIGH',1,now()),($3,$4,'CASE-OTHER','PLATFORM','OPEN','LOW',1,now())`, supportID, org.ID, "scp_"+randomHex(), other.ID); err != nil {
		t.Fatal(err)
	}

	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	safetyService := safetyfeature.NewOperationsService(safetyfeature.OperationsOptions{Pool: pool, Events: publisher, Flags: safetyfeature.OperationsFlags{Incidents: true}})
	financeService := financeops.NewService(financeops.Options{Pool: pool, Events: publisher, Flags: financeops.Flags{Invoices: true, Receivables: true}})
	warehouseService := warehousefeature.NewService(warehousefeature.Options{Pool: pool, Events: publisher, Flags: warehousefeature.Flags{Warehouses: true, Fulfilment: true}})
	engagementService := engagement.NewService(engagement.Options{Pool: pool, Events: publisher, Flags: engagement.Flags{Support: true}})
	actor := tenant.Context{OrganizationID: org.ID, PermittedBranchIDs: []string{}}
	exporters := []exportsusecases.Exporter{
		app.NewComplianceIncidentsExporter(safetyService), app.NewInvoicesReceivablesExporter(financeService),
		app.NewFulfilmentOrdersExporter(warehouseService), app.NewSupportCasesExporter(engagementService),
	}
	for _, exporter := range exporters {
		rows, err := exporter.Rows(ctx, actor, nil, 1)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%s rows=%v err=%v", exporter.Type(), rows, err)
		}
		header := strings.Join(exporter.Columns(), ",")
		for _, forbidden := range []string{"evidence", "privateMessage", "walletOperationReference", "bank", "signedUrl", "credential", "description"} {
			if strings.Contains(strings.ToLower(header), strings.ToLower(forbidden)) {
				t.Fatalf("%s exposes forbidden column %s in %s", exporter.Type(), forbidden, header)
			}
		}
		empty, err := exporter.Rows(ctx, tenant.Context{OrganizationID: other.ID, PermittedBranchIDs: []string{}}, nil, 1)
		if err != nil {
			t.Fatalf("%s other tenant: %v", exporter.Type(), err)
		}
		if exporter.Type() != exportsmodels.TypeSupportCases && len(empty) != 0 {
			t.Fatalf("%s leaked owning-tenant rows: %v", exporter.Type(), empty)
		}
		if exporter.Type() == exportsmodels.TypeSupportCases && (len(empty) != 1 || strings.Contains(strings.Join(empty[0], ","), "CASE-EXPORT")) {
			t.Fatalf("support export tenant isolation failed: %v", empty)
		}
	}
}
