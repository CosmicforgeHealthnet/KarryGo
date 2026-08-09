package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

type IndexWorkerConfig struct {
	Owner                  string
	BatchSize, MaxAttempts int
	Lease                  time.Duration
}
type indexJob struct {
	ID, OrganizationID, SubjectType, SubjectReference, Operation string
	SourceVersion                                                int64
	Attempts                                                     int
}
type projectionSpec struct {
	table, permission string
	title             []string
	exact             []string
	terminal          map[string]bool
}

var projectionSpecs = map[string]projectionSpec{
	"CUSTOMER":            {"fleet_partner_company_customers", "customers.read", []string{"company_name", "display_name", "customer_code", "id"}, []string{"customer_code", "external_reference"}, map[string]bool{"ARCHIVED": true}},
	"PROVIDER_WORKFORCE":  {"fleet_partner_workforce_profiles", "workforce.read", []string{"employee_number", "membership_id", "id"}, []string{"employee_number", "membership_id"}, map[string]bool{"ARCHIVED": true, "TERMINATED": true}},
	"SHIPMENT":            {"fleet_partner_shipments", "shipments.read", []string{"shipment_number", "id"}, []string{"shipment_number", "order_id"}, map[string]bool{"CANCELLED": true, "ARCHIVED": true}},
	"PACKAGE":             {"fleet_partner_packages", "packages.read", []string{"package_code", "tracking_reference", "id"}, []string{"package_code", "tracking_reference", "shipment_id"}, map[string]bool{"CANCELLED": true, "ARCHIVED": true}},
	"MANIFEST":            {"fleet_partner_manifests", "manifests.read", []string{"manifest_number", "id"}, []string{"manifest_number", "route_id"}, map[string]bool{"CANCELLED": true, "ARCHIVED": true}},
	"ROUTE":               {"fleet_partner_route_plans", "routes.read", []string{"route_code", "name", "id"}, []string{"route_code", "assignment_id"}, map[string]bool{"CANCELLED": true, "ARCHIVED": true}},
	"ASSET":               {"fleet_partner_assets", "assets.read", []string{"asset_code", "registration_number", "id"}, []string{"asset_code", "registration_number"}, map[string]bool{"ARCHIVED": true, "RETIRED": true}},
	"INCIDENT":            {"fleet_partner_incidents", "incidents.read", []string{"external_reference", "incident_type", "id"}, []string{"external_reference", "route_id", "trip_reference"}, map[string]bool{"ARCHIVED": true}},
	"COMPLIANCE":          {"fleet_partner_compliance_requirements", "compliance.read", []string{"name", "requirement_code", "id"}, []string{"requirement_code", "subject_type"}, map[string]bool{"INACTIVE": true, "ARCHIVED": true}},
	"INVOICE":             {"fleet_partner_invoices", "invoices.read", []string{"invoice_number", "id"}, []string{"invoice_number", "customer_reference"}, map[string]bool{"CANCELLED": true, "ARCHIVED": true}},
	"WAREHOUSE_INVENTORY": {"fleet_partner_inventory_items", "warehouse_inventory.read", []string{"sku", "serial_reference", "id"}, []string{"sku", "batch_reference", "lot_reference", "serial_reference"}, map[string]bool{"ARCHIVED": true, "DEPLETED": true}},
	"FULFILMENT_ORDER":    {"fleet_partner_fulfilment_orders", "fulfilment.read", []string{"source_order_id", "id"}, []string{"source_order_id", "shipment_reference", "manifest_reference"}, map[string]bool{"CANCELLED": true, "ARCHIVED": true}},
	"SUPPORT_CASE":        {"fleet_partner_support_case_projections", "support.read", []string{"support_service_case_id", "id"}, []string{"support_service_case_id", "trip_reference", "order_reference"}, map[string]bool{"ARCHIVED": true}},
}

func (s *Service) RunIndexingPass(ctx context.Context, cfg IndexWorkerConfig) (int, error) {
	if s == nil || s.opts.Pool == nil || !s.opts.Flags.Enabled {
		return 0, nil
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 25
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Lease <= 0 {
		cfg.Lease = time.Minute
	}
	var jobs []indexJob
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id,organization_id,subject_type,subject_reference,source_version,operation,attempts FROM fleet_partner_search_index_jobs WHERE (status='PENDING' OR (status='LEASED' AND lease_expires_at<now())) AND available_at<=now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1`, cfg.BatchSize)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var j indexJob
			if e = rows.Scan(&j.ID, &j.OrganizationID, &j.SubjectType, &j.SubjectReference, &j.SourceVersion, &j.Operation, &j.Attempts); e != nil {
				return e
			}
			jobs = append(jobs, j)
		}
		for _, j := range jobs {
			if _, e = tx.Exec(ctx, `UPDATE fleet_partner_search_index_jobs SET status='LEASED',lease_owner=$2,lease_expires_at=now()+make_interval(secs=>$3),heartbeat_at=now(),attempts=attempts+1,updated_at=now() WHERE id=$1`, j.ID, cfg.Owner, cfg.Lease.Seconds()); e != nil {
				return e
			}
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		s.processIndexJob(ctx, j, cfg)
	}
	return len(jobs), nil
}

func (s *Service) processIndexJob(ctx context.Context, j indexJob, cfg IndexWorkerConfig) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(cfg.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_, _ = s.opts.Pool.Exec(ctx, `UPDATE fleet_partner_search_index_jobs SET heartbeat_at=now(),lease_expires_at=now()+make_interval(secs=>$3) WHERE id=$1 AND lease_owner=$2 AND status='LEASED'`, j.ID, cfg.Owner, cfg.Lease.Seconds())
			}
		}
	}()
	defer close(done)
	err := s.applyIndexJob(ctx, j)
	outcome := "COMPLETED"
	code := ""
	status := "COMPLETED"
	if err != nil {
		code = "INDEXING_FAILED"
		outcome = "RETRY"
		status = "PENDING"
		if j.Attempts+1 >= cfg.MaxAttempts {
			outcome = "DEAD_LETTER"
			status = "DEAD_LETTER"
		}
	}
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO fleet_partner_search_index_attempts(id,job_id,organization_id,attempt,outcome,safe_error_code) VALUES($1,$2,$3,$4,$5,$6)`, ids.New("sia"), j.ID, j.OrganizationID, j.Attempts+1, outcome, code); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE fleet_partner_search_index_jobs SET status=$2,last_error_code=$3,available_at=CASE WHEN $2='PENDING' THEN now()+make_interval(secs=>LEAST(300,power(2,attempts))) ELSE available_at END,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND lease_owner=$4`, j.ID, status, code, cfg.Owner)
		return e
	})
}

func (s *Service) applyIndexJob(ctx context.Context, j indexJob) error {
	if j.Operation == "REMOVE" {
		return s.RemoveFromIndex(ctx, j.OrganizationID, j.SubjectType, j.SubjectReference)
	}
	spec, ok := projectionSpecs[j.SubjectType]
	if !ok {
		return fmt.Errorf("unsupported subject type")
	}
	var raw []byte
	err := s.opts.Pool.QueryRow(ctx, `SELECT to_jsonb(t) FROM `+spec.table+` t WHERE organization_id=$1 AND id=$2`, j.OrganizationID, j.SubjectReference).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.RemoveFromIndex(ctx, j.OrganizationID, j.SubjectType, j.SubjectReference)
	}
	if err != nil {
		return err
	}
	var row map[string]any
	if err = json.Unmarshal(raw, &row); err != nil {
		return err
	}
	status := fmt.Sprint(row["status"])
	if spec.terminal[status] {
		return s.RemoveFromIndex(ctx, j.OrganizationID, j.SubjectType, j.SubjectReference)
	}
	first := func(keys []string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(fmt.Sprint(row[k])); v != "" && v != "<nil>" {
				return v
			}
		}
		return j.SubjectReference
	}
	exact := models.JSONMap{}
	parts := []string{first(spec.title), status}
	for _, k := range spec.exact {
		v := first([]string{k})
		if v != j.SubjectReference || row[k] != nil {
			exact[k] = v
			parts = append(parts, v)
		}
	}
	branch := first([]string{"branch_id"})
	if row["branch_id"] == nil {
		branch = ""
	}
	_, err = s.Index(ctx, j.OrganizationID, IndexInput{BranchID: branch, SubjectType: j.SubjectType, SubjectReference: j.SubjectReference, Title: first(spec.title), SearchableText: strings.Join(parts, " "), ExactReferenceFields: exact, Status: status, RequiredPermission: spec.permission, Visibility: "INTERNAL", SourceVersion: j.SourceVersion})
	return err
}

func (s *Service) ReplayIndexJob(ctx context.Context, organizationID, jobID string) error {
	tag, err := s.opts.Pool.Exec(ctx, `UPDATE fleet_partner_search_index_jobs SET status='PENDING',available_at=now(),lease_owner=NULL,lease_expires_at=NULL,last_error_code='',updated_at=now() WHERE id=$1 AND organization_id=$2 AND status='DEAD_LETTER'`, jobID, organizationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("search indexing job not found")
	}
	return nil
}
