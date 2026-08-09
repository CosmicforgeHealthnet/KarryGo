// Package retention implements the Section 27 "data retention" required
// worker as a report-only capability scoped to Section 27's own operational
// tables (fleet_partner_jobs, fleet_partner_job_attempts,
// fleet_partner_scheduled_task_runs).
//
// It deliberately does NOT implement destructive anonymization or deletion
// against business-domain tables (customers, orders, documents, financial
// records, and so on): no owning domain in this codebase defines a
// retention policy (what counts as eligible, the legal/retention basis, the
// retention window) for those tables, and Section 27's own architecture
// rule is that a generic background worker must call an owning-domain
// capability rather than invent one. Report/dry-run mode for those broader
// business tables would still require the same missing per-domain policy
// input to decide what to report on, so it is out of scope here for the
// same reason -- this is the same dependency-gating posture the master
// specification explicitly allows for the automation runner.
//
// What this package DOES do for real: count aged, already-terminal rows in
// the three tables Section 27 itself owns and generates, on an interval,
// entirely read-only, so operators get visibility into operational data
// growth without this pass inventing a business retention policy it has no
// authority to define.
package retention

import (
	"context"
	"fmt"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// TableReport is the count of aged, terminal rows found in one table.
type TableReport struct {
	Table         string
	EligibleCount int64
	CutoffDays    int
}

// Report is the immutable result of one scan pass. Never contains row
// contents, payloads, or any other potentially sensitive field -- only
// table names and counts.
type Report struct {
	GeneratedAt time.Time
	Tables      []TableReport
}

// Service scans Section 27's own operational tables for retention
// visibility. It never deletes or mutates anything -- every method is a
// read.
type Service struct {
	db database.Querier
}

// NewService wires the retention scan service over a pool or transaction.
func NewService(db database.Querier) *Service {
	return &Service{db: db}
}

// ScanAndReport counts terminal rows older than cutoffDays in each
// Section-27-owned table, capped at batchLimit per table (a count against
// a LIMIT-bounded subquery, so an unbounded table cannot make a routine
// report pass scan an unbounded number of rows).
func (s *Service) ScanAndReport(ctx context.Context, cutoffDays int, batchLimit int) (Report, error) {
	if cutoffDays <= 0 {
		cutoffDays = 90
	}
	if batchLimit <= 0 || batchLimit > 100000 {
		batchLimit = 10000
	}
	report := Report{GeneratedAt: time.Now().UTC()}

	jobsCount, err := s.countBounded(ctx, batchLimit, `
		SELECT id FROM fleet_partner_jobs
		WHERE status IN ('SUCCEEDED', 'CANCELLED', 'DEAD_LETTERED')
		  AND COALESCE(completed_at, cancelled_at, dead_lettered_at) < now() - make_interval(days => $1)
		LIMIT $2`, cutoffDays)
	if err != nil {
		return Report{}, fmt.Errorf("retention: scan fleet_partner_jobs: %w", err)
	}
	report.Tables = append(report.Tables, TableReport{Table: "fleet_partner_jobs", EligibleCount: jobsCount, CutoffDays: cutoffDays})

	attemptsCount, err := s.countBounded(ctx, batchLimit, `
		SELECT a.id FROM fleet_partner_job_attempts a
		JOIN fleet_partner_jobs j ON j.id = a.job_id
		WHERE j.status IN ('SUCCEEDED', 'CANCELLED', 'DEAD_LETTERED')
		  AND COALESCE(j.completed_at, j.cancelled_at, j.dead_lettered_at) < now() - make_interval(days => $1)
		LIMIT $2`, cutoffDays)
	if err != nil {
		return Report{}, fmt.Errorf("retention: scan fleet_partner_job_attempts: %w", err)
	}
	report.Tables = append(report.Tables, TableReport{Table: "fleet_partner_job_attempts", EligibleCount: attemptsCount, CutoffDays: cutoffDays})

	runsCount, err := s.countBounded(ctx, batchLimit, `
		SELECT id FROM fleet_partner_scheduled_task_runs
		WHERE occurred_at < now() - make_interval(days => $1)
		LIMIT $2`, cutoffDays)
	if err != nil {
		return Report{}, fmt.Errorf("retention: scan fleet_partner_scheduled_task_runs: %w", err)
	}
	report.Tables = append(report.Tables, TableReport{Table: "fleet_partner_scheduled_task_runs", EligibleCount: runsCount, CutoffDays: cutoffDays})

	return report, nil
}

func (s *Service) countBounded(ctx context.Context, limit int, boundedSelect string, cutoffDays int) (int64, error) {
	var count int64
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM (`+boundedSelect+`) bounded`, cutoffDays, limit).Scan(&count)
	return count, err
}
