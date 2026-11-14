package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Package, error)
	Custody(ctx context.Context, t tenant.Context, id string) ([]models.CustodyEvent, error)
	Get(ctx context.Context, t tenant.Context, id string) (models.Package, error)
	GetCustodyEvent(ctx context.Context, t tenant.Context, id string) (models.CustodyEvent, error)
	History(ctx context.Context, t tenant.Context, id string) ([]models.History, error)
	List(ctx context.Context, t tenant.Context, shipment string) ([]models.Package, error)
	RecordCustody(ctx context.Context, t tenant.Context, id string, in usecases.CustodyInput) (models.CustodyEvent, error)
	RecordSubjectCustody(ctx context.Context, t tenant.Context, in usecases.SubjectCustodyInput) (models.CustodyEvent, error)
	Scan(ctx context.Context, t tenant.Context, id string, in usecases.ScanInput) (models.Scan, error)
	SubjectCustody(ctx context.Context, t tenant.Context, subjectType string, subjectRef string) ([]models.CustodyEvent, error)
	Transition(ctx context.Context, t tenant.Context, id string, in usecases.TransitionInput) (models.Package, error)
}
