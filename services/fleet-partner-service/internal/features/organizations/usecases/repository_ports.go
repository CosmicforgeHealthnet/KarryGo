package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// BranchStore lists the repositories.BranchRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type BranchStore interface {
	Archive(ctx context.Context, organizationID string, branchID string, expectedVersion int64) error
	Create(ctx context.Context, q database.Querier, organizationID string, branch *models.Branch) error
	GetByID(ctx context.Context, organizationID string, branchID string) (*models.Branch, error)
	List(ctx context.Context, query tenant.Query) ([]models.Branch, error)
	Update(ctx context.Context, q database.Querier, organizationID string, branch *models.Branch, expectedVersion int64) error
}

// DepartmentStore lists the repositories.DepartmentRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type DepartmentStore interface {
	Archive(ctx context.Context, q database.Querier, organizationID string, departmentID string, expectedVersion int64) error
	Create(ctx context.Context, q database.Querier, organizationID string, department *models.Department) error
	GetByID(ctx context.Context, organizationID string, departmentID string) (*models.Department, error)
	List(ctx context.Context, organizationID string, branchID string, includeArchived bool) ([]models.Department, error)
	Update(ctx context.Context, q database.Querier, organizationID string, department *models.Department, expectedVersion int64) error
}

// OnboardingStore lists the repositories.OnboardingRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type OnboardingStore interface {
	AppendVerificationIssues(ctx context.Context, q database.Querier, organizationID string, requestID string, issues []models.VerificationIssue) error
	AttachDocument(ctx context.Context, q database.Querier, organizationID string, document *models.Document) error
	CreateClosureRequest(ctx context.Context, q database.Querier, organizationID string, request *models.ClosureRequest) error
	CreateRepresentative(ctx context.Context, q database.Querier, organizationID string, rep *models.Representative) error
	CreateVerificationRequest(ctx context.Context, q database.Querier, organizationID string, request *models.VerificationRequest) error
	EnsureWalletProvisioning(ctx context.Context, q database.Querier, organizationID string, idempotencyKey string) error
	GetDocument(ctx context.Context, organizationID string, documentID string) (*models.Document, error)
	GetOpenClosureRequest(ctx context.Context, organizationID string) (*models.ClosureRequest, error)
	GetProfile(ctx context.Context, organizationID string) (*models.Profile, error)
	GetWalletProvisioning(ctx context.Context, organizationID string) (*models.WalletProvisioning, error)
	LatestVerificationRequest(ctx context.Context, organizationID string) (*models.VerificationRequest, error)
	ListCapabilities(ctx context.Context, organizationID string) (map[string]string, error)
	ListClosureBlockers(ctx context.Context, organizationID string, closureID string) ([]models.ClosureBlocker, error)
	ListDocuments(ctx context.Context, organizationID string) ([]models.Document, error)
	ListDocumentsTx(ctx context.Context, q database.Querier, organizationID string) ([]models.Document, error)
	ListRepresentatives(ctx context.Context, organizationID string) ([]models.Representative, error)
	ListStages(ctx context.Context, organizationID string) ([]models.OnboardingStage, error)
	ListVerificationIssues(ctx context.Context, organizationID string, requestID string) ([]models.VerificationIssue, error)
	ListVerificationRequestsToReconcile(ctx context.Context, limit int) ([]models.VerificationRequest, error)
	NextSubmissionVersion(ctx context.Context, q database.Querier, organizationID string) (int, error)
	RemoveDocument(ctx context.Context, q database.Querier, organizationID string, documentID string) error
	ReplaceCapabilities(ctx context.Context, q database.Querier, organizationID string, capabilities []string) error
	ReplaceClosureBlockers(ctx context.Context, q database.Querier, organizationID string, closureID string, blockers []models.ClosureBlocker) error
	SetWalletStatus(ctx context.Context, q database.Querier, organizationID string, status string, walletAccountID string, errorCode string, nextAttemptAt *time.Time) error
	UpdateClosureStatus(ctx context.Context, q database.Querier, organizationID string, closureID string, status string, expectedVersion int64) error
	UpdateRepresentative(ctx context.Context, q database.Querier, organizationID string, repID string, rep *models.Representative, expectedVersion int64) error
	UpdateVerificationStatus(ctx context.Context, q database.Querier, organizationID string, requestID string, status string, externalReference string, sourceVersion int64, eventID string) error
	UpsertProfile(ctx context.Context, q database.Querier, organizationID string, profile *models.Profile, expectedVersion int64) error
	UpsertStage(ctx context.Context, q database.Querier, organizationID string, stage string, status string, missing []string, blocking []string) error
}

// OrganizationStore lists the repositories.OrganizationRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type OrganizationStore interface {
	Create(ctx context.Context, q database.Querier, org *models.Organization) error
	GetByID(ctx context.Context, organizationID string) (*models.Organization, error)
	Update(ctx context.Context, q database.Querier, org *models.Organization) error
	UpdateStatus(ctx context.Context, q database.Querier, organizationID string, expectedVersion int64, change models.StatusChange) error
}

// SettingsStore lists the repositories.SettingsRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type SettingsStore interface {
	Create(ctx context.Context, q database.Querier, organizationID string, settings *models.Settings) error
	Get(ctx context.Context, organizationID string) (*models.Settings, error)
	Update(ctx context.Context, q database.Querier, organizationID string, expectedVersion int64, settings *models.Settings) error
}
