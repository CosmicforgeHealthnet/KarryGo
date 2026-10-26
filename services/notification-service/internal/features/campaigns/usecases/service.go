// Package campaignusecases is notification-service's campaign fulfilment:
// intake of approved HQ intents (admincampaign contract), and a leased,
// bounded engine that validates, plans from customer-service's consent
// authority, re-checks consent immediately before each send, enqueues
// through the existing message pipeline and records truthful outcomes.
package campaignusecases

import (
	"context"
	"errors"
	"time"

	campaignmodels "cosmicforge/logistics/services/notification-service/internal/features/campaigns/models"
	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminevents"
)

var (
	ErrInvalid  = admincampaign.ErrInvalid
	ErrConflict = admincampaign.ErrConflict
	ErrNotFound = campaignrepositories.ErrNotFound
)

// Service handles the private campaign contract.
type Service struct {
	Repo *campaignrepositories.Repository
	Now  func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// Accept decodes, validates and stores an intent. created reports whether a
// new revision was accepted (false for an idempotent replay).
func (s *Service) Accept(ctx context.Context, raw []byte) (admincampaign.Status, bool, error) {
	intent, err := admincampaign.Decode(raw)
	if err != nil {
		return admincampaign.Status{}, false, ErrInvalid
	}
	if err := intent.Validate(s.now()); err != nil {
		return admincampaign.Status{}, false, ErrInvalid
	}
	rev, created, err := s.Repo.Accept(ctx, intent, intent.Digest())
	if err != nil {
		return admincampaign.Status{}, false, err
	}
	return rev.Status(s.now()), created, nil
}

// Status reads one revision's truthful status.
func (s *Service) Status(ctx context.Context, campaignID string, revision int64) (admincampaign.Status, error) {
	rev, err := s.Repo.Get(ctx, campaignID, revision)
	if err != nil {
		return admincampaign.Status{}, err
	}
	return rev.Status(s.now()), nil
}

// Cancel validates and applies a cancellation.
func (s *Service) Cancel(ctx context.Context, c admincampaign.Cancellation, campaignID string) (admincampaign.Status, error) {
	if err := c.Validate(); err != nil || c.CampaignID != campaignID {
		return admincampaign.Status{}, ErrInvalid
	}
	rev, err := s.Repo.Cancel(ctx, c)
	if err != nil {
		return admincampaign.Status{}, err
	}
	return rev.Status(s.now()), nil
}

// Snapshot builds one owner admin snapshot page of campaign revisions.
func (s *Service) Snapshot(ctx context.Context, cursor string, limit int) (adminevents.SnapshotPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	revs, next, watermark, err := s.Repo.Snapshot(ctx, cursor, limit)
	if err != nil {
		if errors.Is(err, campaignrepositories.ErrConflict) {
			return adminevents.SnapshotPage{}, ErrInvalid
		}
		return adminevents.SnapshotPage{}, err
	}
	page := adminevents.SnapshotPage{Contract: adminevents.SnapshotContract, Source: campaignmodels.SourceService, EntityType: "campaign_revision",
		Watermark: watermark, Events: []adminevents.SourceEvent{}, NextCursor: next, Complete: next == ""}
	for _, rev := range revs {
		ev, err := campaignrepositories.Event(rev)
		if err != nil {
			return adminevents.SnapshotPage{}, err
		}
		page.Events = append(page.Events, ev)
	}
	return page, page.Validate(campaignmodels.SourceService)
}
