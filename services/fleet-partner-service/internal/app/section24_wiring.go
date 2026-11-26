package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	analyticsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	analyticsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/repositories"
	analyticsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/usecases"
	communicationsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	communicationsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/usecases"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	engagementusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/usecases"
	governancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	governancerepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/repositories"
	governanceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// NewAnalyticsService wires the Section 24/54 analytics use cases. Automation
// actions reach other features only through these two adapters: raising an
// operational exception (engagement) and queueing a notification intent
// (communications). Neither can move money, change an assignment or lift a
// safety or compliance restriction.
func NewAnalyticsService(cfg config.Config, pool *pgxpool.Pool, publisher analyticsusecases.Events,
	engagement *engagementusecases.Service, communications *communicationsusecases.Service) *analyticsusecases.Service {
	return analyticsusecases.NewService(analyticsusecases.Options{
		Store: analyticsrepos.NewRepository(pool), Events: publisher, Queue: jobs.NewQueue(pool),
		Exceptions: automationExceptionAdapter{engagement: engagement},
		Notices:    automationNoticeAdapter{communications: communications},
		Flags: analyticsmodels.Flags{Reports: cfg.AnalyticsReportsEnabled, Recommendations: cfg.RecommendationsEnabled,
			Automation: cfg.AutomationEnabled, Workers: cfg.Section24WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch},
	})
}

type automationExceptionAdapter struct{ engagement *engagementusecases.Service }

func (a automationExceptionAdapter) RaiseAutomationException(ctx context.Context, organizationID, executionID, exceptionType, severity, summary string) (string, error) {
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "automation-executor", CorrelationID: executionID}
	v, err := a.engagement.RaiseException(ctx, t, engagementmodels.ExceptionInput{ExceptionType: exceptionType, Severity: severity, SourceType: "AUTOMATION_EXECUTION", SourceReference: executionID, Summary: summary})
	if err != nil {
		return "", err
	}
	id, _ := v["id"].(string)
	return id, nil
}

type automationNoticeAdapter struct {
	communications *communicationsusecases.Service
}

func (a automationNoticeAdapter) QueueAutomationNotice(ctx context.Context, tx pgx.Tx, organizationID, executionID, recipientRole, templateCode, idempotencyKey string, payload json.RawMessage) (string, error) {
	return a.communications.QueueIntentTx(ctx, tx, communicationsmodels.IntentInput{
		OrganizationID: organizationID, SourceType: "AUTOMATION_EXECUTION", SourceReference: executionID,
		RecipientType: "ORGANIZATION_ROLE", RecipientReference: recipientRole, Channels: json.RawMessage(`["IN_APP"]`),
		TemplateCode: templateCode, SafePayload: payload, IdempotencyKey: idempotencyKey, IgnoreDuplicate: true})
}

// StartSection24Workers periodically queues scheduled report runs,
// automation evaluation and analytics housekeeping when
// SECTION24_WORKERS_ENABLED is set.
func StartSection24Workers(ctx context.Context, cfg config.Config, c *Components, wg *sync.WaitGroup) {
	if c == nil || c.Analytics == nil || wg == nil || cfg.IntegrationsKillSwitch || !cfg.Section24WorkersEnabled {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			_ = c.Analytics.ScheduleMaintenance(ctx, nil)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// NewGovernanceService wires the Section 28/56 privacy-request workflow and
// the organization security-event read model.
func NewGovernanceService(cfg config.Config, pool *pgxpool.Pool, publisher governanceusecases.Events) *governanceusecases.Service {
	return governanceusecases.NewService(governanceusecases.Options{
		Store: governancerepos.NewRepository(pool), Events: publisher,
		Flags: governancemodels.Flags{PrivacyRequests: cfg.PrivacyRequestsEnabled, SecurityEvents: cfg.SecurityEventsEnabled, KillSwitch: cfg.IntegrationsKillSwitch},
	})
}
