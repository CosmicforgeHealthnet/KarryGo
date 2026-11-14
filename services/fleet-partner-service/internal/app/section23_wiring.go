package app

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	communicationsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	communicationsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/repositories"
	communicationsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/usecases"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	engagementrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/repositories"
	engagementusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// newSection23Services wires the Section 23 communication and engagement use
// cases to their PostgreSQL stores. Engagement queues escalation
// notifications through the communications feature's intents.
func newSection23Services(cfg config.Config, pool *pgxpool.Pool, publisher engagementusecases.Events,
	media platformclients.MediaContract, support engagementusecases.SupportPort,
	notification communicationsusecases.NotificationPort) (*communicationsusecases.Service, *engagementusecases.Service) {
	queue := jobs.NewQueue(pool)
	communications := communicationsusecases.NewService(communicationsusecases.Options{
		Store: communicationsrepos.NewRepository(pool), Events: publisher, Queue: queue, Media: media,
		Notification: notification,
		Flags: communicationsmodels.Flags{Communications: cfg.CommunicationsEnabled, NotificationRules: cfg.NotificationRulesEnabled,
			Workers: cfg.Section23WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	engagement := engagementusecases.NewService(engagementusecases.Options{
		Store: engagementrepos.NewRepository(pool), Events: publisher, Queue: queue, Support: support,
		Notifications: EscalationNotices(communications),
		Flags: engagementmodels.Flags{Support: cfg.SupportProjectionsEnabled, Quality: cfg.QualityCasesEnabled,
			SLA: cfg.SLAMeasurementsEnabled, Escalations: cfg.EscalationsEnabled,
			Workers: cfg.Section23WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	return communications, engagement
}

// EscalationNotices adapts the communications intents to engagement's
// NotificationIntents port.
func EscalationNotices(communications *communicationsusecases.Service) engagementusecases.NotificationIntents {
	return escalationNoticeAdapter{communications: communications}
}

type escalationNoticeAdapter struct {
	communications *communicationsusecases.Service
}

func (a escalationNoticeAdapter) QueueEscalationNotice(ctx context.Context, tx pgx.Tx, notice engagementusecases.EscalationNotice) (string, error) {
	return a.communications.QueueIntentTx(ctx, tx, communicationsmodels.IntentInput{
		OrganizationID: notice.OrganizationID, SourceType: "ESCALATION", SourceReference: notice.EscalationID,
		RecipientType: notice.RecipientType, RecipientReference: notice.RecipientReference,
		Channels: notice.Channels, TemplateCode: "fleet.escalation.due", SafePayload: notice.SafePayload,
		IdempotencyKey: notice.IdempotencyKey, IgnoreDuplicate: true})
}
