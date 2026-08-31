package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/models"
	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/repositories"
	"cosmicforge/logistics/shared/go/commerceanalytics"
)

type Service struct {
	repo *repositories.Repository
	now  func() time.Time
}

func NewService(repo *repositories.Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Ingest(ctx context.Context, event commerceanalytics.Envelope) (bool, error) {
	return s.repo.Ingest(ctx, event)
}

func (s *Service) Report(ctx context.Context, businessID, branchID, window string, start, end time.Time) (models.OperationalReport, error) {
	w, err := models.ResolveWindow(window, start, end, s.now())
	if err != nil {
		return models.OperationalReport{}, err
	}
	return s.repo.Report(ctx, businessID, branchID, w)
}

func (s *Service) Reliability(ctx context.Context, businessID, branchID, window string, start, end time.Time) (models.ReliabilityScore, error) {
	w, err := models.ResolveWindow(window, start, end, s.now())
	if err != nil {
		return models.ReliabilityScore{}, err
	}
	return s.repo.Reliability(ctx, businessID, branchID, w)
}
