package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"fmt"
	"time"
)

type WorkerConfig struct {
	Owner                  string
	Lease                  time.Duration
	BatchSize, MaxAttempts int
}
type WorkerResult struct{ Claimed, Generated, Failed int }

func (s *Service) RunGenerationPass(ctx context.Context, c WorkerConfig) (WorkerResult, error) {
	var result WorkerResult
	if !s.opts.Flags.WorkerEnabled || s.opts.Flags.KillSwitch {
		return result, nil
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.Lease <= 0 {
		c.Lease = time.Minute
	}
	templates, e := s.opts.Repository.ClaimDue(ctx, c.Owner, c.Lease, c.BatchSize)
	if e != nil {
		return result, e
	}
	result.Claimed = len(templates)
	for _, tpl := range templates {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		freq, interval, _, x := models.ParseRule(tpl.RecurrenceType, tpl.NormalizedRRule)
		if x != nil {
			s.opts.Repository.Fail(ctx, c.Owner, tpl.ID, "invalid_schedule", c.MaxAttempts)
			result.Failed++
			continue
		}
		scheduled := tpl.NextRunAt
		if tpl.EndDate != nil && scheduled.After(*tpl.EndDate) {
			_, _ = s.opts.Repository.SetStatus(ctx, nil, tpl.OrganizationID, tpl.ID, tpl.Version, models.StatusCompleted, "system")
			continue
		}
		occ := models.Occurrence{OrganizationID: tpl.OrganizationID, TemplateID: tpl.ID, OccurrenceKey: models.OccurrenceKey(tpl.ID, scheduled), ScheduledFor: scheduled, LeaseOwner: c.Owner}
		occ, created, x := s.opts.Repository.CreateOccurrence(ctx, nil, occ)
		if x != nil {
			s.opts.Repository.Fail(ctx, c.Owner, tpl.ID, "occurrence_create_failed", c.MaxAttempts)
			result.Failed++
			continue
		}
		if !created && occ.GeneratedOrderID != "" {
			next, _ := models.Next(scheduled, tpl.Timezone, freq, interval)
			_ = s.opts.Repository.CompleteOccurrence(ctx, nil, occ, occ.GeneratedOrderID, next)
			continue
		}
		tc := tenant.Context{OrganizationID: tpl.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "recurring-order-worker", PermittedBranchIDs: []string{tpl.BranchID}, CorrelationID: occ.OccurrenceKey}
		stopHeartbeat := startRecurringHeartbeat(ctx, s.opts.Repository, c.Owner, tpl.ID, c.Lease)
		orderID, x := s.opts.Orders.CreateRecurring(ctx, tc, OrderInput{BranchID: tpl.BranchID, CustomerID: tpl.CustomerID, CustomerReference: tpl.Data.CustomerReference, ServiceCatalogItemID: tpl.ServiceCatalogItemID, ContractID: tpl.ContractID, Timezone: tpl.Timezone, Priority: tpl.Data.Priority, SpecialInstructions: tpl.Data.SpecialInstructions, SourceReference: occ.OccurrenceKey, ServiceFields: tpl.Data.ServiceFields, ScheduleStart: scheduled})
		stopHeartbeat()
		if x != nil {
			_ = s.opts.Repository.Attempt(ctx, nil, tpl.OrganizationID, occ.ID, tpl.Attempts, "FAILED", "order_create_failed")
			_ = s.opts.Repository.FailOccurrence(ctx, c.Owner, tpl, occ, "order_create_failed", c.MaxAttempts)
			result.Failed++
			continue
		}
		next, x := models.Next(scheduled, tpl.Timezone, freq, interval)
		if x != nil {
			return result, fmt.Errorf("advance schedule: %w", x)
		}
		if x = s.opts.Repository.CompleteOccurrence(ctx, nil, occ, orderID, next); x != nil {
			return result, x
		}
		_ = s.opts.Repository.Attempt(ctx, nil, tpl.OrganizationID, occ.ID, 1, "GENERATED", "")
		result.Generated++
	}
	return result, nil
}

func startRecurringHeartbeat(ctx context.Context, repo *repositories.Repository, owner, id string,
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
