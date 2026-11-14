package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	AppendHistory(ctx context.Context, q database.Querier, h models.History) error
	Create(ctx context.Context, q database.Querier, p models.Package) (models.Package, error)
	CreateCustody(ctx context.Context, q database.Querier, e models.CustodyEvent) (models.CustodyEvent, error)
	CreateScan(ctx context.Context, q database.Querier, s models.Scan) (models.Scan, error)
	CustodySubjectBranch(ctx context.Context, q database.Querier, org string, subjectType string, subjectRef string) (string, error)
	Get(ctx context.Context, q database.Querier, org string, id string) (models.Package, error)
	GetCustody(ctx context.Context, q database.Querier, org string, pkg string, id string) (models.CustodyEvent, error)
	GetCustodyEvent(ctx context.Context, q database.Querier, org string, id string) (models.CustodyEvent, error)
	GetScanByClientID(ctx context.Context, q database.Querier, org string, clientID string) (models.Scan, error)
	IncrementShipmentPackageCount(ctx context.Context, q database.Querier, org string, shipment string) error
	LatestCustody(ctx context.Context, q database.Querier, org string, pkg string) (models.CustodyEvent, error)
	LatestScan(ctx context.Context, q database.Querier, org string, pkg string) (models.Scan, error)
	LatestSubjectCustody(ctx context.Context, q database.Querier, org string, subjectType string, subjectRef string) (models.CustodyEvent, error)
	ListByShipment(ctx context.Context, q database.Querier, org string, shipment string) ([]models.Package, error)
	ListCustody(ctx context.Context, org string, pkg string) ([]models.CustodyEvent, error)
	ListHistory(ctx context.Context, org string, pkg string) ([]models.History, error)
	ListSubjectCustody(ctx context.Context, org string, subjectType string, subjectRef string) ([]models.CustodyEvent, error)
	Lock(ctx context.Context, q database.Querier, org string, id string) (models.Package, error)
	LockCustodySubject(ctx context.Context, q database.Querier, org string, subjectType string, subjectRef string) error
	Transition(ctx context.Context, q database.Querier, org string, id string, version int64, status string, condition string, actor string) (models.Package, error)
}
