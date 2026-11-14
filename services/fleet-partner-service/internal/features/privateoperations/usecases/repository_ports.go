package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Attempt(ctx context.Context, q database.Querier, org string, id string, outcome string, code string, n int) error
	ClaimExecutions(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.ExecutionOperation, error)
	ClaimExpired(ctx context.Context, q database.Querier, limit int) ([]models.Assignment, error)
	ClaimReconciliations(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.ReconciliationCase, error)
	CreateAssignment(ctx context.Context, q database.Querier, a models.Assignment) (models.Assignment, error)
	CreateDecision(ctx context.Context, q database.Querier, d models.Decision) (models.Decision, error)
	CreateExecution(ctx context.Context, q database.Querier, o models.ExecutionOperation) (models.ExecutionOperation, error)
	DeadLetterExecution(ctx context.Context, owner string, id string, code string) error
	DeadLetterReconciliation(ctx context.Context, owner string, id string, code string) error
	GetAssignment(ctx context.Context, q database.Querier, org string, id string) (models.Assignment, error)
	GetAssignmentByID(ctx context.Context, q database.Querier, id string) (models.Assignment, error)
	GetDecision(ctx context.Context, q database.Querier, org string, assignment string) (models.Decision, error)
	GetExecution(ctx context.Context, q database.Querier, org string, id string) (models.ExecutionOperation, error)
	GetExecutionByID(ctx context.Context, q database.Querier, id string) (models.ExecutionOperation, error)
	HeartbeatExecution(ctx context.Context, owner string, id string, lease time.Duration) error
	HeartbeatReconciliation(ctx context.Context, owner string, id string, lease time.Duration) error
	History(ctx context.Context, q database.Querier, org string, id string, from string, to string, reason string, actorType string, actor string, corr string) error
	ListAssignments(ctx context.Context, org string, status string) ([]models.Assignment, error)
	LockAssignment(ctx context.Context, q database.Querier, org string, id string) (models.Assignment, error)
	ReconciliationAttempt(ctx context.Context, q database.Querier, item models.ReconciliationCase, outcome string, code string) error
	RecoverExpiredExecutions(ctx context.Context) (int64, error)
	RecoverExpiredReconciliations(ctx context.Context) (int64, error)
	ReplayExecution(ctx context.Context, q database.Querier, org string, id string) error
	ReplayReconciliation(ctx context.Context, q database.Querier, org string, id string) error
	RequestExecutionCancellation(ctx context.Context, q database.Querier, org string, assignmentID string) error
	ResolveExecution(ctx context.Context, q database.Querier, owner string, id string, status string, request string, trip string, code string, sourceVersion int64) error
	ResolveReconciliation(ctx context.Context, q database.Querier, owner string, id string) error
	RetryExecution(ctx context.Context, owner string, id string, status string, code string, next time.Time) error
	RetryReconciliation(ctx context.Context, owner string, id string, code string, next time.Time) error
	SetAssignmentStatus(ctx context.Context, q database.Querier, org string, id string, version int64, status string, reason string, note string, replacement string) (models.Assignment, error)
}
