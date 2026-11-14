package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Archive(ctx context.Context, input usecases.ArchiveInput) (*models.Document, error)
	AuthorizeCustomerDownload(ctx context.Context, organizationID string, orderID string, customerID string, documentID string, correlationID string, causationID string, requestID string) (*usecases.DownloadGrant, error)
	AuthorizeDownload(ctx context.Context, actor usecases.Actor, documentID string, correlationID string, causationID string, requestID string) (*usecases.DownloadGrant, error)
	AuthorizeProviderDownload(ctx context.Context, providerService string, providerID string, documentID string, correlationID string, causationID string, requestID string) (*usecases.DownloadGrant, error)
	Cases(ctx context.Context, organizationID string, documentID string) ([]models.ReconciliationCase, error)
	Get(ctx context.Context, organizationID string, documentID string) (*models.Document, error)
	GetCustomerDocument(ctx context.Context, organizationID string, orderID string, customerID string, documentID string, correlationID string) (*models.Document, error)
	GetProviderDocument(ctx context.Context, providerService string, providerID string, documentID string, correlationID string) (*models.Document, error)
	History(ctx context.Context, organizationID string, documentID string) ([]models.HistoryEntry, error)
	IngestMediaEvent(ctx context.Context, raw []byte) (string, error)
	LatestDeletion(ctx context.Context, organizationID string, documentID string) (*models.DeletionRequest, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Document, error)
	ListCustomerDocuments(ctx context.Context, organizationID string, orderID string, customerID string, correlationID string) ([]models.Document, error)
	ListProviderDocuments(ctx context.Context, providerService string, providerID string, correlationID string) ([]models.Document, error)
	Reject(ctx context.Context, input usecases.VerificationInput) (*models.Document, error)
	Replace(ctx context.Context, input usecases.ReplaceInput) (*usecases.UploadTicket, error)
	ReplayDeletion(ctx context.Context, organizationID string, requestID string) error
	ReplayReconciliationCase(ctx context.Context, organizationID string, caseID string) error
	RequestUpload(ctx context.Context, input usecases.RequestUploadInput) (*usecases.UploadTicket, error)
	RequestVerification(ctx context.Context, input usecases.VerificationInput) (*models.Document, error)
	Revoke(ctx context.Context, input usecases.RevokeInput) (*models.Document, error)
	ScheduleDeletion(ctx context.Context, input usecases.ScheduleDeletionInput) (*models.DeletionRequest, error)
	SetRetention(ctx context.Context, input usecases.RetentionInput) (*models.Document, error)
	Verifications(ctx context.Context, organizationID string, documentID string) ([]models.VerificationDecision, error)
	Verify(ctx context.Context, input usecases.VerificationInput) (*models.Document, error)
}
