package campaignusecases

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"

	campaignclients "cosmicforge/logistics/services/notification-service/internal/features/campaigns/clients"
	campaignmodels "cosmicforge/logistics/services/notification-service/internal/features/campaigns/models"
	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

// Sender is the existing message pipeline entry point.
type Sender interface {
	Send(ctx context.Context, request notifications.Request) (messageusecases.SendResult, error)
}

// Engine is the leased, bounded campaign worker.
type Engine struct {
	Repo     *campaignrepositories.Repository
	Consent  campaignclients.ConsentAuthority
	Sender   Sender
	Owner    string
	Now      func() time.Time
	Interval time.Duration // tick cadence
	// Bounds per tick. Zero values take conservative defaults.
	ClaimLimit     int
	PagesPerTick   int
	BatchesPerTick int
	BatchSize      int
	Lease          time.Duration
	Retry          time.Duration // re-run delay while work is outstanding
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *Engine) defaults() {
	if e.Owner == "" {
		e.Owner = "campaign-engine-" + uuid.NewString()
	}
	if e.Interval <= 0 {
		e.Interval = 5 * time.Second
	}
	if e.ClaimLimit <= 0 {
		e.ClaimLimit = 4
	}
	if e.PagesPerTick <= 0 {
		e.PagesPerTick = 10
	}
	if e.BatchesPerTick <= 0 {
		e.BatchesPerTick = 2
	}
	if e.BatchSize <= 0 || e.BatchSize > campaignclients.MaxBatch {
		e.BatchSize = campaignclients.MaxBatch
	}
	if e.Lease <= 0 {
		e.Lease = 2 * time.Minute
	}
	if e.Retry <= 0 {
		e.Retry = 15 * time.Second
	}
}

// Run ticks until ctx ends. Errors are logged by code only; no message body,
// address or identifier is ever logged here.
func (e *Engine) Run(ctx context.Context) error {
	e.defaults()
	ticker := time.NewTicker(e.Interval)
	defer ticker.Stop()
	for {
		if err := e.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logging.Error("campaign-engine", "tick failed")
		}
		_ = e.Repo.PruneNonces(ctx, 15*time.Minute)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Tick claims due revisions and advances each one bounded step.
func (e *Engine) Tick(ctx context.Context) error {
	e.defaults()
	revs, err := e.Repo.ClaimDue(ctx, e.Owner, e.ClaimLimit, e.Lease)
	if err != nil {
		return err
	}
	for _, rev := range revs {
		next := e.advance(ctx, rev)
		if err := e.Repo.Release(ctx, rev.CampaignID, rev.Revision, e.Owner, next); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) advance(ctx context.Context, rev campaignmodels.Revision) time.Time {
	now := e.now()
	retry := now.Add(e.Retry)
	if rev.State == campaignmodels.StateAccepted {
		if rev.ContentTitle == nil {
			title, body, code, err := e.validate(ctx, rev)
			if err != nil {
				return retry
			}
			if code != "" {
				if err := e.Repo.Reject(ctx, rev.CampaignID, rev.Revision, code); err != nil {
					return retry
				}
				return now
			}
			if err := e.Repo.RecordContent(ctx, rev.CampaignID, rev.Revision, title, body); err != nil {
				return retry
			}
		}
		done, err := e.plan(ctx, rev)
		if err != nil || !done {
			// An unreachable consent authority leaves the revision planning;
			// nothing is assumed about the audience in the meantime.
			if err != nil {
				return retry
			}
			return now
		}
		return now
	}
	return e.send(ctx, rev, now, retry)
}

// validate checks the content reference and audience dimensions. A non-empty
// code is a terminal rejection; an error is transient.
func (e *Engine) validate(ctx context.Context, rev campaignmodels.Revision) (string, string, string, error) {
	i := rev.Intent
	if i.Category != "MARKETING" && i.Category != "PRODUCT_ANNOUNCEMENT" {
		return "", "", campaignmodels.RejectCategoryNotPermitted, nil
	}
	if len(i.Zones) > 0 {
		// No canonical zone is recorded for customers anywhere; a zone-targeted
		// campaign cannot be honoured, and zones are never inferred.
		return "", "", campaignmodels.RejectAudienceDimension, nil
	}
	if !e.now().Before(i.EndAt) {
		return "", "", campaignmodels.RejectWindowClosed, nil
	}
	t, ok, err := e.Repo.GetTemplate(ctx, i.TemplateKey, i.TemplateLocale)
	if err != nil {
		return "", "", "", err
	}
	if !ok || !t.Active || t.Revision != i.TemplateRevision || campaignmodels.ContentSHA256(t.Title, t.Body) != i.ContentSHA256 {
		return "", "", campaignmodels.RejectTemplateMismatch, nil
	}
	return t.Title, t.Body, "", nil
}

// plan pages the consent authority for every (market, channel) pair. Customers
// with an unknown market never match a market query.
func (e *Engine) plan(ctx context.Context, rev campaignmodels.Revision) (bool, error) {
	if e.Consent == nil {
		return false, campaignclients.ErrUnavailable
	}
	pairs := campaignmodels.Pairs(rev.Intent)
	cursor := rev.PlanCursor
	for page := 0; page < e.PagesPerTick; page++ {
		if cursor.Pair >= len(pairs) {
			return true, e.Repo.RecordPlan(ctx, rev.CampaignID, rev.Revision, nil, cursor, true)
		}
		pair := pairs[cursor.Pair]
		res, err := e.Consent.Audience(ctx, pair.Channel, pair.Market, cursor.Cursor, campaignclients.MaxBatch)
		if err != nil {
			return false, err
		}
		members := make([]campaignrepositories.PlannedMember, 0, len(res.Members))
		for _, m := range res.Members {
			if _, err := uuid.Parse(m.CustomerID); err != nil || m.ConsentVersion < 1 {
				continue // malformed authority data is never planned
			}
			members = append(members, campaignrepositories.PlannedMember{RecipientID: m.CustomerID, Channel: pair.Channel, Market: pair.Market, ConsentVersion: m.ConsentVersion})
		}
		if res.NextCursor == "" {
			cursor = campaignmodels.PlanCursor{Pair: cursor.Pair + 1}
		} else {
			cursor.Cursor = res.NextCursor
		}
		done := cursor.Pair >= len(pairs)
		if err := e.Repo.RecordPlan(ctx, rev.CampaignID, rev.Revision, members, cursor, done); err != nil {
			return false, err
		}
		if done {
			return true, nil
		}
	}
	return false, nil
}

func (e *Engine) send(ctx context.Context, rev campaignmodels.Revision, now, retry time.Time) time.Time {
	id, n := rev.CampaignID, rev.Revision
	if err := e.Repo.Reconcile(ctx, id, n); err != nil {
		return retry
	}
	i := rev.Intent
	if now.Before(i.StartAt) {
		return i.StartAt
	}
	if !now.Before(i.EndAt) {
		// The window closed: nothing still planned may go out. Outcomes of
		// already-queued messages keep settling.
		if err := e.Repo.SuppressAllPlanned(ctx, id, n, campaignmodels.SuppressWindowClosed); err != nil {
			return retry
		}
		_ = e.Repo.Reconcile(ctx, id, n)
		return retry
	}
	if quiet, resume := campaignmodels.QuietDeferral(i.QuietHours, now); quiet {
		// Deferred, never dropped.
		if resume.After(i.EndAt) {
			return i.EndAt
		}
		return resume
	}
	for _, channel := range rev.Intent.Channels {
		for batch := 0; batch < e.BatchesPerTick; batch++ {
			planned, err := e.Repo.ListPlanned(ctx, id, n, channel, e.BatchSize)
			if err != nil {
				return retry
			}
			if len(planned) == 0 {
				break
			}
			if err := e.sendBatch(ctx, rev, channel, planned); err != nil {
				if errors.Is(err, campaignrepositories.ErrInactive) {
					return retry
				}
				_ = e.Repo.Reconcile(ctx, id, n)
				return retry
			}
		}
	}
	_ = e.Repo.Reconcile(ctx, id, n)
	return retry
}

func (e *Engine) sendBatch(ctx context.Context, rev campaignmodels.Revision, channel string, planned []campaignrepositories.PlannedRecipient) error {
	if e.Consent == nil || e.Sender == nil || rev.ContentTitle == nil || rev.ContentBody == nil {
		return campaignclients.ErrUnavailable
	}
	ids := make([]string, len(planned))
	for k, p := range planned {
		ids[k] = p.RecipientID
	}
	// Re-check consent immediately before enqueue. An unanswerable check
	// leaves every recipient planned: unknown is never permission to send.
	check, err := e.Consent.Check(ctx, channel, ids)
	if err != nil {
		return err
	}
	answers := map[string]campaignclients.Eligibility{}
	for _, r := range check.Results {
		answers[r.CustomerID] = r
	}
	buckets := map[string][]string{}
	var eligible []struct {
		p     campaignrepositories.PlannedRecipient
		email string
	}
	for _, p := range planned {
		a, ok := answers[p.RecipientID]
		switch {
		case !ok:
			buckets[campaignmodels.SuppressUnknown] = append(buckets[campaignmodels.SuppressUnknown], p.RecipientID)
		case !a.Eligible:
			buckets[suppressionFor(a.Reason)] = append(buckets[suppressionFor(a.Reason)], p.RecipientID)
		case a.Market == nil || *a.Market != p.Market:
			buckets[campaignmodels.SuppressMarketChanged] = append(buckets[campaignmodels.SuppressMarketChanged], p.RecipientID)
		case channel == notifications.ChannelEmail && (a.Email == nil || *a.Email == ""):
			buckets[campaignmodels.SuppressNoContact] = append(buckets[campaignmodels.SuppressNoContact], p.RecipientID)
		default:
			email := ""
			if a.Email != nil {
				email = *a.Email
			}
			eligible = append(eligible, struct {
				p     campaignrepositories.PlannedRecipient
				email string
			}{p, email})
		}
	}
	for reason, recipients := range buckets {
		if err := e.Repo.Suppress(ctx, rev.CampaignID, rev.Revision, channel, recipients, reason); err != nil {
			return err
		}
	}
	limits := campaignrepositories.EnqueueLimits{MaxPerRecipientPerDay: rev.Intent.MaxPerRecipientPerDay, MaxMessages: rev.Intent.MaxMessages}
	for _, el := range eligible {
		request := e.request(rev, channel, el.p.RecipientID, el.email)
		_, err := e.Repo.Enqueue(ctx, rev.CampaignID, rev.Revision, el.p.RecipientID, channel, limits, func(ctx context.Context) (string, error) {
			res, err := e.Sender.Send(ctx, request)
			return res.MessageID, err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func suppressionFor(reason string) string {
	switch reason {
	case "NOT_OPTED_IN":
		return campaignmodels.SuppressConsentWithdrawn
	case "ACCOUNT_INACTIVE", "ACCOUNT_NOT_FOUND":
		return campaignmodels.SuppressAccountInactive
	case "NO_EMAIL_ADDRESS":
		return campaignmodels.SuppressNoContact
	default:
		return campaignmodels.SuppressUnknown
	}
}

func (e *Engine) request(rev campaignmodels.Revision, channel, recipient, email string) notifications.Request {
	ref := rev.CampaignID + ":" + strconv.FormatInt(rev.Revision, 10)
	return notifications.Request{
		IDempotencyKey:      campaignmodels.MessageIdempotencyKey(rev.CampaignID, rev.Revision, recipient, channel),
		SourceService:       campaignmodels.SourceService,
		SourceAggregateType: "campaign_revision",
		SourceAggregateID:   ref,
		CorrelationRef:      rev.Intent.CorrelationID,
		EventType:           campaignmodels.EventType,
		Recipient:           notifications.Recipient{Type: campaignmodels.RecipientType, ID: recipient, Email: email},
		Channels:            []string{channel},
		TemplateKey:         rev.Intent.TemplateKey,
		Locale:              rev.Intent.TemplateLocale,
		Title:               *rev.ContentTitle,
		Body:                *rev.ContentBody,
		Data:                map[string]interface{}{"campaign_id": rev.CampaignID, "campaign_revision": rev.Revision},
		Priority:            rev.Intent.Priority,
	}
}
