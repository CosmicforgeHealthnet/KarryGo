package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	ActiveDelegationsTo(ctx context.Context, q database.Querier, organizationID string, delegateID string, now time.Time) ([]models.Delegation, error)
	ActiveOrganizationPolicies(ctx context.Context, q database.Querier, organizationID string, resourceType string, action string) ([]models.Policy, error)
	AdvanceStep(ctx context.Context, q database.Querier, organizationID string, requestID string, expectedVersion int64, nextStep int) error
	AppendHistory(ctx context.Context, q database.Querier, organizationID string, requestID string, fromStatus string, toStatus string, actorType string, actorID string, reasonCode string, correlationID string) error
	AppendPolicyVersion(ctx context.Context, q database.Querier, policy *models.Policy, changedBy string, reason string) error
	ArchivePolicy(ctx context.Context, q database.Querier, organizationID string, policyID string, expectedVersion int64, actor string) error
	BranchExists(ctx context.Context, q database.Querier, organizationID string, branchID string) (bool, error)
	CompleteStep(ctx context.Context, q database.Querier, stepID string, status string) error
	CountStepApprovals(ctx context.Context, q database.Querier, stepID string) (int, error)
	CreateDelegation(ctx context.Context, q database.Querier, organizationID string, delegation *models.Delegation) error
	CreatePolicy(ctx context.Context, q database.Querier, organizationID string, policy *models.Policy) error
	CreateRequest(ctx context.Context, q database.Querier, organizationID string, request *models.Request) error
	CurrentStep(ctx context.Context, q database.Querier, organizationID string, requestID string, stepNumber int) (*models.Step, error)
	EnsurePolicy(ctx context.Context, q database.Querier, organizationID string, resourceType string, action string, template repositories.PolicyTemplate) (string, int64, error)
	ExpireOverdue(ctx context.Context, q database.Querier, limit int) (int64, error)
	FindByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, idempotencyKey string) (*models.Request, error)
	FindDecisionByApprover(ctx context.Context, q database.Querier, requestID string, approverID string) (*models.Decision, error)
	FindDecisionByClientID(ctx context.Context, q database.Querier, requestID string, clientDecisionID string) (*models.Decision, error)
	FindDecisionOnBehalfOf(ctx context.Context, q database.Querier, requestID string, stepID string, person string) (*models.Decision, error)
	FindDelegationByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, key string) (*models.Delegation, error)
	FindExecutionByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, idempotencyKey string) (*models.Execution, error)
	FindOpenForResource(ctx context.Context, q database.Querier, organizationID string, resourceType string, resourceID string, action string) (*models.Request, error)
	FindPolicyByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, key string) (*models.Policy, error)
	GetDelegation(ctx context.Context, q database.Querier, organizationID string, delegationID string) (*models.Delegation, error)
	GetPolicy(ctx context.Context, q database.Querier, organizationID string, policyID string) (*models.Policy, error)
	GetRequest(ctx context.Context, q database.Querier, organizationID string, requestID string) (*models.Request, error)
	HasOverlappingDelegation(ctx context.Context, q database.Querier, candidate *models.Delegation) (bool, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Request, error)
	ListDecisions(ctx context.Context, q database.Querier, requestID string) ([]models.Decision, error)
	ListDelegations(ctx context.Context, organizationID string, filter repositories.DelegationFilter, now time.Time) ([]models.Delegation, error)
	ListHistory(ctx context.Context, organizationID string, requestID string) ([]repositories.HistoryEntry, error)
	ListPolicies(ctx context.Context, organizationID string, filter repositories.PolicyFilter) ([]models.Policy, error)
	ListPolicyVersions(ctx context.Context, organizationID string, policyID string) ([]models.PolicyVersion, error)
	ListSteps(ctx context.Context, q database.Querier, organizationID string, requestID string) ([]models.Step, error)
	LockDelegation(ctx context.Context, q database.Querier, organizationID string, delegationID string) (*models.Delegation, error)
	LockDelegator(ctx context.Context, q database.Querier, organizationID string, delegatorID string) error
	LockPolicy(ctx context.Context, q database.Querier, organizationID string, policyID string) (*models.Policy, error)
	LockRequest(ctx context.Context, q database.Querier, organizationID string, requestID string) (*models.Request, error)
	MarkExecuted(ctx context.Context, q database.Querier, organizationID string, requestID string, expectedVersion int64, executionReference string) error
	MemberStatus(ctx context.Context, q database.Querier, organizationID string, memberID string) (string, error)
	RecordDecision(ctx context.Context, q database.Querier, organizationID string, decision *models.Decision) error
	RecordExecution(ctx context.Context, q database.Querier, organizationID string, execution *models.Execution) error
	RevokeDelegation(ctx context.Context, q database.Querier, organizationID string, delegationID string, expectedVersion int64, revokedBy string, reason string) error
	RoleUsable(ctx context.Context, q database.Querier, organizationID string, roleID string) (bool, error)
	UpdatePolicy(ctx context.Context, q database.Querier, organizationID string, policy *models.Policy, expectedVersion int64) error
	UpdateStatus(ctx context.Context, q database.Querier, organizationID string, requestID string, expectedVersion int64, status string, invalidationReason string) error
}
