package usecases

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	CancelInvitation(ctx context.Context, q database.Querier, invitationID string) error
	ClaimDueSagas(ctx context.Context, limit int) ([]models.ProvisioningSaga, error)
	ClaimInvitationResend(ctx context.Context, tx pgx.Tx, invitationID string, expectedVersion int64, tokenHash string, expiresAt *time.Time) error
	ConfirmInvitationResend(ctx context.Context, tx pgx.Tx, invitationID string, reference string) error
	CountLiveMemberships(ctx context.Context, organizationID string) (int, error)
	CreateInvitation(ctx context.Context, q database.Querier, organizationID string, invitation *models.Invitation) error
	CreateMembership(ctx context.Context, q database.Querier, organizationID string, membership *models.Membership) error
	CreateSaga(ctx context.Context, q database.Querier, organizationID string, saga *models.ProvisioningSaga) error
	EnsureSubWalletLink(ctx context.Context, q database.Querier, organizationID string, membershipID string, idempotencyKey string) error
	ExpireInvitation(ctx context.Context, tx pgx.Tx, invitationID string) error
	FindLiveByProvider(ctx context.Context, q database.Querier, providerService string, providerID string) (*models.Membership, error)
	GetInvitationByMembership(ctx context.Context, organizationID string, membershipID string) (*models.Invitation, error)
	GetMembership(ctx context.Context, organizationID string, membershipID string) (*models.Membership, error)
	GetMembershipForUpdate(ctx context.Context, tx pgx.Tx, organizationID string, membershipID string) (*models.Membership, error)
	GetPolicy(ctx context.Context, organizationID string, membershipID string) (*models.Policy, error)
	GetSaga(ctx context.Context, organizationID string, membershipID string) (*models.ProvisioningSaga, error)
	GetSubWalletLink(ctx context.Context, organizationID string, membershipID string) (*models.SubWalletLink, error)
	ListCapabilities(ctx context.Context, organizationID string, membershipID string) ([]models.Capability, error)
	ListHistory(ctx context.Context, organizationID string, membershipID string) ([]models.HistoryEntry, error)
	ListHistoryPage(ctx context.Context, organizationID string, membershipID string, limit int, afterOccurredAt *time.Time, afterID string) ([]models.HistoryEntry, error)
	ListMemberships(ctx context.Context, query tenant.Query) ([]models.Membership, error)
	LockLatestInvitation(ctx context.Context, tx pgx.Tx, organizationID string, membershipID string) (*models.Invitation, error)
	RecordInvitationDelivery(ctx context.Context, invitationID string, status string, reference string) error
	ReleaseInvitationResend(ctx context.Context, tx pgx.Tx, invitationID string, previousLastSentAt *time.Time) error
	ReplaceCapabilities(ctx context.Context, q database.Querier, organizationID string, membershipID string, capabilities []models.Capability) error
	SetInvitationOutcome(ctx context.Context, q database.Querier, invitationID string, status string) error
	SetProviderID(ctx context.Context, q database.Querier, organizationID string, membershipID string, providerID string, sourceVersion int64, onboardingSource string) error
	SetSagaAssociation(ctx context.Context, q database.Querier, membershipID string, associationID string, sourceVersion int64) error
	SetSubWalletStatus(ctx context.Context, q database.Querier, membershipID string, status string, accountID string, sourceVersion int64) error
	TransitionMembership(ctx context.Context, q database.Querier, organizationID string, membershipID string, toStatus string, expectedVersion int64, entry models.HistoryEntry) error
	UpdateSagaStep(ctx context.Context, q database.Querier, membershipID string, column string, value string, overall string, errorCode string, nextAttemptAt *time.Time) error
	UpsertPolicy(ctx context.Context, q database.Querier, organizationID string, policy *models.Policy) error
}
