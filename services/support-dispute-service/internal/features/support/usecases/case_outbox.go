package supportusecases

import (
	"context"
	"time"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
)

func (s *SupportService) RunCaseOutbox(ctx context.Context) error {
	if s.resolutionPublisher == nil {
		return nil
	}
	repo, ok := s.repo.(*supportrepositories.PostgresSupportRepository)
	if !ok {
		return nil
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			records, err := repo.ClaimCaseOutbox(ctx, 50)
			if err != nil {
				continue
			}
			for _, rec := range records {
				if err = s.resolutionPublisher.PublishResolution(ctx, rec.Event); err != nil {
					_ = repo.FailCaseOutbox(ctx, rec.ID, rec.Attempts+1, err.Error())
					continue
				}
				_ = repo.CompleteCaseOutbox(ctx, rec.ID)
			}
		}
	}
}

var _ supportclients.CommerceResolutionPublisher = (*supportclients.CommerceResolutionClient)(nil)
