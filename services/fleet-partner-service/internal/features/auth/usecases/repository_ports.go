package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
)

// ChallengeStore lists the repositories.ChallengeRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type ChallengeStore interface {
	Consume(ctx context.Context, q database.Querier, challengeID string) error
	Create(ctx context.Context, q database.Querier, challenge *models.Challenge) error
	GetByID(ctx context.Context, challengeID string) (*models.Challenge, error)
	Latest(ctx context.Context, q database.Querier, emailHash string, phoneHash string, purpose string, sessionID string, actionClass string) (*models.Challenge, error)
	LockForVerification(ctx context.Context, q database.Querier, challengeID string) (*models.Challenge, error)
	RecordDelivery(ctx context.Context, challengeID string, status string, reference string) error
	RecordFailedAttempt(ctx context.Context, q database.Querier, challengeID string) error
}

// IdentityStore lists the repositories.IdentityRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type IdentityStore interface {
	Create(ctx context.Context, q database.Querier, identity *models.Identity) error
	FindByEmail(ctx context.Context, q database.Querier, value string) (*models.Identity, error)
	FindByPhone(ctx context.Context, q database.Querier, value string) (*models.Identity, error)
	GetByID(ctx context.Context, identityID string) (*models.Identity, error)
	GetByIDUsing(ctx context.Context, q database.Querier, identityID string) (*models.Identity, error)
	MarkContactVerified(ctx context.Context, q database.Querier, identityID string, expectedVersion int64) error
	ResolvePair(ctx context.Context, q database.Querier, emailNormalized string, phoneE164 string) (*models.Identity, error)
}

// SecurityEventStore lists the repositories.SecurityEventRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type SecurityEventStore interface {
	Record(ctx context.Context, q database.Querier, event models.SecurityEvent) error
}

// SessionStore lists the repositories.SessionRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type SessionStore interface {
	AppendHistory(ctx context.Context, q database.Querier, sessionID string, identityID string, fromStatus string, toStatus string, reason string, requestID string, correlationID string) error
	Create(ctx context.Context, q database.Querier, session *models.Session) error
	FamilyCompromised(ctx context.Context, q database.Querier, familyID string) (bool, error)
	GetByID(ctx context.Context, sessionID string) (*models.Session, error)
	ListForIdentity(ctx context.Context, identityID string) ([]models.Session, error)
	LockByID(ctx context.Context, q database.Querier, sessionID string) (*models.Session, error)
	LockByRefreshHash(ctx context.Context, q database.Querier, refreshHash string) (*models.Session, error)
	LockFamily(ctx context.Context, q database.Querier, familyID string) error
	LogoutFamily(ctx context.Context, q database.Querier, familyID string) (int64, error)
	Revoke(ctx context.Context, q database.Querier, sessionID string, reason string) error
	RevokeActiveFamily(ctx context.Context, q database.Querier, familyID string, reason string) (int64, error)
	RevokeFamily(ctx context.Context, q database.Querier, familyID string, reason string) (int64, error)
	RevokeForIdentity(ctx context.Context, q database.Querier, identityID string, reason string) (int64, error)
	RotateRefresh(ctx context.Context, q database.Querier, current *models.Session, nextHash string, refreshExpiresAt time.Time) (*models.Session, error)
}
