package vendorownedhandover

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/notifications"
)

type Service struct {
	repo     *Repository
	policy   Policy
	notifier notifications.Notifier
	limiter  interface {
		Allow(context.Context, string, int, time.Duration) error
	}
	now func() time.Time
}

func NewService(repo *Repository, policy Policy) *Service {
	return &Service{repo: repo, policy: policy.normalized(), now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) WithNotifier(n notifications.Notifier) *Service { s.notifier = n; return s }
func (s *Service) WithRateLimiter(l interface {
	Allow(context.Context, string, int, time.Duration) error
}) *Service {
	s.limiter = l
	return s
}
func (s *Service) Enabled() bool { return s != nil && s.repo != nil && s.policy.Enabled }

func (s *Service) Bind(ctx context.Context, b MissionBinding) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(ctx, "vendor-owned:bind:"+b.BusinessID, 30, time.Minute); err != nil {
			return Observation{}, err
		}
	}
	issue, replay, err := s.repo.Bind(ctx, b, s.now())
	if err != nil {
		return Observation{}, err
	}
	if !replay {
		if err = s.deliver(ctx, issue, false); err != nil {
			_ = s.repo.Invalidate(ctx, issue.HandoverRef, "NOTIFICATION_FAILED", s.now())
			return Observation{}, err
		}
	}
	return issue.Observation, nil
}
func (s *Service) Read(ctx context.Context, mission string, stage Stage) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	return s.repo.Read(ctx, mission, stage)
}
func (s *Service) Confirm(ctx context.Context, c ConfirmCommand, caller, correlation string) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(ctx, "vendor-owned:verify:"+c.MissionRef+":"+c.ProviderRef, 20, time.Minute); err != nil {
			return Observation{}, err
		}
	}
	o, _, next, err := s.repo.Confirm(ctx, c, caller, correlation, s.now())
	if err != nil {
		return Observation{}, err
	}
	if next != nil {
		if err = s.deliver(ctx, *next, false); err != nil {
			_ = s.repo.Invalidate(ctx, next.HandoverRef, "NOTIFICATION_FAILED", s.now())
			return Observation{}, err
		}
	}
	return o, nil
}
func (s *Service) IssueReturn(ctx context.Context, mission, operation, correlation string) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	issue, _, err := s.repo.IssueReturn(ctx, strings.TrimSpace(mission), strings.TrimSpace(operation), correlation, s.now())
	if err != nil {
		return Observation{}, err
	}
	if err = s.deliver(ctx, issue, false); err != nil {
		_ = s.repo.Invalidate(ctx, issue.HandoverRef, "NOTIFICATION_FAILED", s.now())
		return Observation{}, err
	}
	return issue.Observation, nil
}
func (s *Service) Resend(ctx context.Context, mission string, stage Stage, provider, operation, caller, correlation string) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	if operation == "" {
		return Observation{}, ErrInvalid
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(ctx, "vendor-owned:resend:"+mission, 10, time.Hour); err != nil {
			return Observation{}, err
		}
	}
	issue, err := s.repo.Resend(ctx, mission, stage, provider, operation, caller, correlation, s.now())
	if err != nil {
		return Observation{}, err
	}
	if err = s.deliver(ctx, issue, true); err != nil {
		_ = s.repo.Invalidate(ctx, issue.HandoverRef, "NOTIFICATION_FAILED", s.now())
		return Observation{}, err
	}
	return issue.Observation, nil
}
func (s *Service) Reassign(ctx context.Context, c ChangeCommand, correlation string) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	issue, err := s.repo.ChangeBinding(ctx, c, true, correlation, s.now())
	if err != nil {
		return Observation{}, err
	}
	if issue.OTP == "" && issue.QR == "" {
		return issue.Observation, nil
	}
	if err = s.deliver(ctx, issue, false); err != nil {
		_ = s.repo.Invalidate(ctx, issue.HandoverRef, "NOTIFICATION_FAILED", s.now())
		return Observation{}, err
	}
	return issue.Observation, nil
}
func (s *Service) ChangeReceiver(ctx context.Context, c ChangeCommand, correlation string) (Observation, error) {
	if !s.Enabled() {
		return Observation{}, ErrDisabled
	}
	issue, err := s.repo.ChangeBinding(ctx, c, false, correlation, s.now())
	if err != nil {
		return Observation{}, err
	}
	if issue.OTP == "" && issue.QR == "" {
		return issue.Observation, nil
	}
	if err = s.deliver(ctx, issue, false); err != nil {
		_ = s.repo.Invalidate(ctx, issue.HandoverRef, "NOTIFICATION_FAILED", s.now())
		return Observation{}, err
	}
	return issue.Observation, nil
}

func (s *Service) Cancel(ctx context.Context, mission, operation, correlation string) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	return s.repo.Cancel(ctx, mission, operation, correlation, s.now())
}

func (s *Service) deliver(ctx context.Context, i secretIssue, resend bool) error {
	if s.notifier == nil {
		return errors.New("approved notification channel is unavailable")
	}
	event := "dispatch.vendor_delivery.handover_challenge"
	if resend {
		event = "dispatch.vendor_delivery.handover_resend"
	}
	recipient := notifications.Recipient{Type: notifications.RecipientStaff, ID: i.RecipientRef}
	channels := []string{notifications.ChannelInApp}
	if i.Stage == Delivery {
		recipient.Type = "receiver"
		recipient.Phone = strings.TrimSpace(i.RecipientPhone)
		channels = []string{notifications.ChannelInApp}
		if recipient.Phone != "" {
			channels = append(channels, notifications.ChannelPush)
		}
	}
	_, err := s.notifier.Send(ctx, notifications.Request{IDempotencyKey: notifications.IdempotencyKey("driver-dispatch-delivery-service", event, i.HandoverRef), SourceService: "driver-dispatch-delivery-service", SourceAggregateType: "vendor_handover_challenge", SourceAggregateID: i.HandoverRef, SourceVersion: i.SourceVersion, EventType: event, Recipient: recipient, Channels: channels, TemplateKey: "booking.delivery_code_ready", TemplateData: map[string]any{"code": i.OTP, "qr_token": i.QR, "expires_at": i.ExpiresAt, "stage": i.Stage}, Priority: notifications.PriorityHigh})
	return err
}
