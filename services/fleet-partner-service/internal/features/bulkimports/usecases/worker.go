package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type WorkerConfig struct {
	Owner                  string
	Lease                  time.Duration
	BatchSize, MaxAttempts int
}
type WorkerResult struct{ Claimed, Validated, Completed, Failed int }

func validRow(r models.OrderRow) string {
	if strings.TrimSpace(r.CustomerID) == "" {
		return "customer_required"
	}
	if strings.TrimSpace(r.ServiceCatalogItemID) == "" {
		return "catalog_required"
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return "timezone_invalid"
	}
	return ""
}

func (s *Service) RunWorkerPass(ctx context.Context, cfg WorkerConfig) (WorkerResult, error) {
	var result WorkerResult
	if !s.opts.Flags.WorkerEnabled || s.opts.Flags.KillSwitch {
		return result, nil
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Lease <= 0 {
		cfg.Lease = time.Minute
	}
	imports, err := s.opts.Repository.Claim(ctx, cfg.Owner, cfg.Lease, cfg.BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(imports)
	for _, imp := range imports {
		tc := tenant.Context{OrganizationID: imp.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "bulk-import-worker", PermittedBranchIDs: []string{imp.BranchID}, CorrelationID: imp.ID}
		switch imp.Status {
		case models.StatusValidating:
			if s.opts.Media == nil {
				_ = s.opts.Repository.Fail(ctx, cfg.Owner, imp.ID, "media_unavailable", cfg.MaxAttempts)
				result.Failed++
				continue
			}
			stopHeartbeat := startBulkHeartbeat(ctx, s.opts.Repository, cfg.Owner, imp.ID, cfg.Lease)
			file, loadErr := s.opts.Media.LoadImport(ctx, tc, imp.MediaFileID, s.opts.MaxBytes)
			stopHeartbeat()
			if loadErr != nil {
				_ = s.opts.Repository.Attempt(ctx, imp.OrganizationID, imp.ID, "FAILED", "media_unavailable", imp.Attempts)
				_ = s.opts.Repository.Fail(ctx, cfg.Owner, imp.ID, "media_unavailable", cfg.MaxAttempts)
				result.Failed++
				continue
			}
			if file.SizeBytes > s.opts.MaxBytes || strings.ToUpper(file.Format) != imp.FileFormat {
				_ = s.opts.Repository.SetValidated(ctx, nil, imp.ID, 0, 0, 0, models.StatusValidationFailed)
				_ = s.opts.Repository.Attempt(ctx, imp.OrganizationID, imp.ID, "FAILED", "bulk_import_invalid_file", imp.Attempts)
				result.Failed++
				continue
			}
			rows, parseErr := parse(file.Content, imp.FileFormat)
			if parseErr != nil || len(rows) > s.opts.MaxRows {
				_ = s.opts.Repository.SetValidated(ctx, nil, imp.ID, len(rows), 0, len(rows), models.StatusValidationFailed)
				result.Failed++
				continue
			}
			valid, invalid := 0, 0
			for number, row := range rows {
				code, status, message := validRow(row), "VALID", ""
				if code != "" {
					status, message = "INVALID", "row failed validation"
					invalid++
				} else {
					valid++
				}
				if row.RowReference == "" {
					row.RowReference = fmt.Sprintf("row-%d", number+1)
				}
				if err = s.opts.Repository.InsertRow(ctx, nil, models.Row{ID: ids.New(repositories.PrefixRow), OrganizationID: imp.OrganizationID, ImportID: imp.ID, RowNumber: number + 1, RowReference: row.RowReference, RowFingerprint: rowFingerprint(row), Status: status, SafeErrorCode: code, SafeErrorMessage: message, Data: row}); err != nil {
					break
				}
			}
			if err != nil {
				_ = s.opts.Repository.Fail(ctx, cfg.Owner, imp.ID, "row_persist_failed", cfg.MaxAttempts)
				result.Failed++
				continue
			}
			next := models.StatusReady
			if valid == 0 {
				next = models.StatusValidationFailed
			}
			if err = s.opts.Repository.SetValidated(ctx, nil, imp.ID, len(rows), valid, invalid, next); err != nil {
				return result, err
			}
			result.Validated++
		case models.StatusImporting:
			rows, listErr := s.opts.Repository.ListRows(ctx, nil, imp.OrganizationID, imp.ID, "VALID")
			if listErr != nil {
				return result, listErr
			}
			imported, failed := 0, 0
			errorsOut := []map[string]any{}
			cancelled := false
			for _, row := range rows {
				stopHeartbeat := startBulkHeartbeat(ctx, s.opts.Repository, cfg.Owner, imp.ID, cfg.Lease)
				orderID, createErr := s.opts.Orders.CreateImported(ctx, tc, OrderInput{BranchID: imp.BranchID, Row: row.Data, SourceReference: imp.ID + ":" + row.RowReference})
				stopHeartbeat()
				if createErr != nil {
					failed++
					_ = s.opts.Repository.MarkRow(ctx, nil, row.ID, "FAILED", "", "order_validation_failed")
					errorsOut = append(errorsOut, map[string]any{"rowReference": row.RowReference, "code": "order_validation_failed"})
					continue
				}
				imported++
				_ = s.opts.Repository.MarkRow(ctx, nil, row.ID, "IMPORTED", orderID, "")
				cancelled, err = s.opts.Repository.IsCancelled(ctx, imp.OrganizationID, imp.ID)
				if err != nil {
					return result, err
				}
				if cancelled {
					break
				}
			}
			if cancelled {
				continue
			}
			status := models.StatusCompleted
			if failed > 0 || imp.InvalidRows > 0 {
				status = models.StatusPartiallyCompleted
			}
			reportID := ""
			if len(errorsOut) > 0 || imp.InvalidRows > 0 {
				invalidRows, _ := s.opts.Repository.ListRows(ctx, nil, imp.OrganizationID, imp.ID, "INVALID")
				for _, row := range invalidRows {
					errorsOut = append(errorsOut, map[string]any{"rowReference": row.RowReference, "code": row.SafeErrorCode})
				}
				raw, _ := json.Marshal(map[string]any{"importId": imp.ID, "errors": errorsOut})
				if s.opts.Media != nil {
					reportID, _ = s.opts.Media.WriteErrorReport(ctx, tc, imp.ID, raw)
				}
			}
			if err = s.opts.Repository.Finalize(ctx, nil, imp.ID, status, reportID, imported, failed); err != nil {
				return result, err
			}
			_ = s.opts.Repository.Attempt(ctx, imp.OrganizationID, imp.ID, "COMPLETED", "", imp.Attempts)
			result.Completed++
		}
	}
	return result, nil
}

func startBulkHeartbeat(ctx context.Context, repo *repositories.Repository, owner, id string,
	lease time.Duration) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	interval := lease / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = repo.Heartbeat(ctx, owner, id, lease)
			}
		}
	}()
	return func() { close(stop); <-done }
}
