package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

// SupportMessagePort is support-dispute-service's organization-case message
// contract: create once per client message id, and read back by that id.
type SupportMessagePort interface {
	CreateMessage(ctx context.Context, request models.SupportMessageRequest) (models.SupportMessage, error)
	ReadMessage(ctx context.Context, organizationID, supportCaseID, clientMessageID string) (models.SupportMessage, error)
	Configured() bool
}

// SupportMessageStore is Fleet's submission ledger.
type SupportMessageStore interface {
	SupportCaseRefs(ctx context.Context, organizationID, caseID string) (projectionID, supportCaseID string, err error)
	ClaimSupportMessage(ctx context.Context, tx pgx.Tx, l models.SupportMessageLedger) (models.SupportMessageLedger, error)
	RecordSupportMessageAttempt(ctx context.Context, q database.Querier, organizationID, id, status, supportMessageID, code string) (bool, error)
	GetSupportMessage(ctx context.Context, organizationID, id string) (models.SupportMessageLedger, error)
	SupportMessagesNeedingReadBack(ctx context.Context, organizationID string, before time.Time) ([]models.SupportMessageLedger, error)
}

// MediaFiles validates attachments with media-file-service.
type MediaFiles interface {
	GetFile(ctx context.Context, meta platformclients.CallMeta, fileID string) (platformclients.FileMetadata, error)
	Configured() bool
}

const (
	maxSupportMessageRunes = 4000
	maxSupportMessageFiles = 10
	// supportMessageReadBackAfter is how long an ambiguous submission rests
	// before the reconcile job reads it back; supportMessageGiveUpAfter is
	// when an authoritative "not received" closes it so the client can resend
	// with the same client message id.
	supportMessageReadBackAfter = time.Minute
	supportMessageGiveUpAfter   = time.Hour
)

func supportMessageHash(sender, text, visibility string, fileIDs []string) string {
	body, _ := json.Marshal(map[string]any{"sender": sender, "text": text, "visibility": visibility, "fileIds": fileIDs})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func normalizeSupportMessage(in models.SupportMessageInput) (models.SupportMessageInput, error) {
	in.Text = strings.TrimSpace(in.Text)
	in.Visibility = strings.ToUpper(strings.TrimSpace(in.Visibility))
	if in.Visibility == "" {
		in.Visibility = models.SupportMessageShared
	}
	var fields []apperrors.FieldViolation
	if id, err := uuid.Parse(strings.TrimSpace(in.ClientMessageID)); err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "clientMessageId", Message: "clientMessageId must be a UUID."})
	} else {
		in.ClientMessageID = id.String()
	}
	if in.Text == "" || len([]rune(in.Text)) > maxSupportMessageRunes {
		fields = append(fields, apperrors.FieldViolation{Field: "text", Message: "text is required and at most 4000 characters."})
	}
	if in.Visibility != models.SupportMessageShared && in.Visibility != models.SupportMessageInternal {
		fields = append(fields, apperrors.FieldViolation{Field: "visibility", Message: "visibility must be SHARED or INTERNAL."})
	}
	seen := map[string]bool{}
	files := make([]string, 0, len(in.FileIDs))
	for _, f := range in.FileIDs {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			fields = append(fields, apperrors.FieldViolation{Field: "fileIds", Message: "fileIds must be non-empty and unique."})
			break
		}
		seen[f] = true
		files = append(files, f)
	}
	if len(files) > maxSupportMessageFiles {
		fields = append(fields, apperrors.FieldViolation{Field: "fileIds", Message: "At most 10 files may be attached."})
	}
	in.FileIDs = files
	if len(fields) > 0 {
		return in, apperrors.Validation("Check the support message.", fields)
	}
	return in, nil
}

func (s *Service) supportMessagesReady() error {
	if s.o.SupportMessages == nil || !s.o.SupportMessages.Configured() || s.o.MessageStore == nil {
		return apperrors.Unavailable("Support service is unavailable.", nil)
	}
	return nil
}

func (s *Service) validateSupportFiles(ctx context.Context, t tenant.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("File validation is unavailable.", nil)
	}
	for _, id := range fileIDs {
		m, err := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, id)
		if err != nil {
			var ce *platformclients.Error
			if errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeNotFound {
				return apperrors.Forbidden("A file is not owned by this organization or is not ready.", nil)
			}
			return apperrors.Unavailable("File validation failed.", err)
		}
		if !m.Ready() || m.OwnerID != t.OrganizationID {
			return apperrors.Forbidden("A file is not owned by this organization or is not ready.", nil)
		}
	}
	return nil
}

// PostSupportMessage writes a message on one of the organization's platform
// support cases (spec 59.10). support-dispute-service stores the message;
// Fleet records the submission so that a client message id is delivered at
// most once, an ambiguous outcome is read back rather than resent, and
// support_message.created is announced exactly once.
func (s *Service) PostSupportMessage(ctx context.Context, t tenant.Context, caseID string, in models.SupportMessageInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return nil, err
	}
	in, err := normalizeSupportMessage(in)
	if err != nil {
		return nil, err
	}
	if err := s.supportMessagesReady(); err != nil {
		return nil, err
	}
	projectionID, supportCaseID, err := s.o.MessageStore.SupportCaseRefs(ctx, t.OrganizationID, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("Support case not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	if err := s.validateSupportFiles(ctx, t, in.FileIDs); err != nil {
		return nil, err
	}
	hash := supportMessageHash(t.ActorID, in.Text, in.Visibility, in.FileIDs)
	var ledger models.SupportMessageLedger
	newID := ids.New("spm")
	if err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		ledger, err = s.o.MessageStore.ClaimSupportMessage(ctx, tx, models.SupportMessageLedger{ID: newID, OrganizationID: t.OrganizationID,
			ProjectionID: projectionID, SupportCaseID: supportCaseID, ClientMessageID: in.ClientMessageID, RequestHash: hash,
			Visibility: in.Visibility, FileIDs: in.FileIDs, CreatedBy: t.ActorID})
		return err
	}); err != nil {
		return nil, err
	}
	if ledger.RequestHash != hash {
		return nil, apperrors.Conflict("This clientMessageId was already used for a different message.", nil)
	}
	request := models.SupportMessageRequest{OrganizationID: t.OrganizationID, SupportCaseID: supportCaseID, ClientMessageID: in.ClientMessageID,
		SenderReference: t.ActorID, Text: in.Text, Visibility: in.Visibility, FileIDs: in.FileIDs}
	switch {
	case ledger.Status == models.SupportMessageConfirmed:
		msg, readErr := s.o.SupportMessages.ReadMessage(ctx, t.OrganizationID, supportCaseID, in.ClientMessageID)
		if readErr != nil {
			return supportMessageView(ledger, nil), nil
		}
		return supportMessageView(ledger, &msg), nil
	case ledger.ID != newID && ledger.Status != models.SupportMessageRejected:
		// Another attempt is in flight, crashed mid-flight or ended with an
		// unknown outcome: the authority decides, never a blind resend.
		msg, readErr := s.o.SupportMessages.ReadMessage(ctx, t.OrganizationID, supportCaseID, in.ClientMessageID)
		if readErr == nil {
			return s.confirmSupportMessage(ctx, t, ledger, msg)
		}
		if !isPlatformNotFound(readErr) {
			return nil, s.markSupportMessageUnknown(ctx, t.OrganizationID, ledger, readErr)
		}
	}
	return s.submitSupportMessage(ctx, t, ledger, request)
}

func (s *Service) submitSupportMessage(ctx context.Context, t tenant.Context, ledger models.SupportMessageLedger, request models.SupportMessageRequest) (map[string]any, error) {
	msg, err := s.o.SupportMessages.CreateMessage(ctx, request)
	if err == nil {
		return s.confirmSupportMessage(ctx, t, ledger, msg)
	}
	if code, public, definite := definiteSupportFailure(err); definite {
		if _, recErr := s.o.MessageStore.RecordSupportMessageAttempt(ctx, nil, t.OrganizationID, ledger.ID, models.SupportMessageRejected, "", code); recErr != nil {
			return nil, recErr
		}
		return nil, public
	}
	// Ambiguous: the message may have been stored. Read it back once now.
	if back, readErr := s.o.SupportMessages.ReadMessage(ctx, t.OrganizationID, request.SupportCaseID, request.ClientMessageID); readErr == nil {
		return s.confirmSupportMessage(ctx, t, ledger, back)
	}
	return nil, s.markSupportMessageUnknown(ctx, t.OrganizationID, ledger, err)
}

func (s *Service) markSupportMessageUnknown(ctx context.Context, organizationID string, ledger models.SupportMessageLedger, cause error) error {
	if _, err := s.o.MessageStore.RecordSupportMessageAttempt(ctx, nil, organizationID, ledger.ID, models.SupportMessageOutcomeUnknown, "", "outcome_unknown"); err != nil {
		return err
	}
	return apperrors.Unavailable("The message outcome is unknown; retry with the same clientMessageId, it will not be posted twice.", cause)
}

// confirmSupportMessage marks the submission CONFIRMED and, only for the
// attempt that did so, writes the audit record and support_message.created.
func (s *Service) confirmSupportMessage(ctx context.Context, t tenant.Context, ledger models.SupportMessageLedger, msg models.SupportMessage) (map[string]any, error) {
	if msg.ID == "" || msg.ClientMessageID != ledger.ClientMessageID {
		return nil, apperrors.Unavailable("Support service returned an invalid message.", nil)
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		confirmed, err := s.o.MessageStore.RecordSupportMessageAttempt(ctx, tx, t.OrganizationID, ledger.ID, models.SupportMessageConfirmed, msg.ID, "")
		if err != nil || !confirmed {
			return err
		}
		return s.record(ctx, tx, t, "support_message.created.v1", "support_message", ledger.ID, ledger.Version+1,
			map[string]any{"messageId": ledger.ID, "supportCaseId": ledger.SupportCaseID, "supportCaseProjectionId": ledger.ProjectionID,
				"supportMessageId": msg.ID, "clientMessageId": ledger.ClientMessageID, "visibility": ledger.Visibility, "fileCount": len(ledger.FileIDs)})
	})
	if err != nil {
		return nil, err
	}
	current, err := s.o.MessageStore.GetSupportMessage(ctx, t.OrganizationID, ledger.ID)
	if err != nil {
		return nil, err
	}
	return supportMessageView(current, &msg), nil
}

func supportMessageView(l models.SupportMessageLedger, msg *models.SupportMessage) map[string]any {
	v := map[string]any{"id": l.ID, "organizationId": l.OrganizationID, "supportCaseId": l.SupportCaseID, "supportCaseProjectionId": l.ProjectionID,
		"clientMessageId": l.ClientMessageID, "visibility": l.Visibility, "fileIds": l.FileIDs, "status": l.Status,
		"supportMessageId": l.SupportMessageID, "createdBy": l.CreatedBy, "version": l.Version, "createdAt": l.CreatedAt, "updatedAt": l.UpdatedAt}
	if l.ConfirmedAt != nil {
		v["confirmedAt"] = l.ConfirmedAt
	}
	if msg != nil {
		v["text"] = msg.Text
		v["postedAt"] = msg.CreatedAt
	}
	return v
}

func isPlatformNotFound(err error) bool {
	var ce *platformclients.Error
	return errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeNotFound
}

// definiteSupportFailure classifies an error that proves the message was not
// stored. Anything else (timeouts, 5xx, unknown outcomes) may have been
// stored and is read back instead.
func definiteSupportFailure(err error) (code string, public error, definite bool) {
	if errors.Is(err, platformclients.ErrNotConfigured) || errors.Is(err, platformclients.ErrCircuitOpen) {
		return "not_sent", apperrors.Unavailable("Support service is unavailable.", err), true
	}
	var ce *platformclients.Error
	if !errors.As(err, &ce) {
		return "", nil, false
	}
	switch ce.Outcome {
	case platformclients.OutcomeValidation:
		return string(ce.Outcome), apperrors.Validation("Support service rejected the message.", nil), true
	case platformclients.OutcomeNotFound:
		return string(ce.Outcome), apperrors.NotFound("Support case not found.", nil), true
	case platformclients.OutcomeConflict:
		return string(ce.Outcome), apperrors.Conflict("Support service refused the message: the case is closed or the clientMessageId was used for a different message.", nil), true
	case platformclients.OutcomeForbidden, platformclients.OutcomeUnauthenticated, platformclients.OutcomeRateLimited:
		return string(ce.Outcome), apperrors.Unavailable("Support service is unavailable.", err), true
	}
	return "", nil, false
}

// reconcileSupportMessages reads back submissions left with an unknown
// outcome. A message the authority holds is confirmed (and announced once);
// one it still does not hold after supportMessageGiveUpAfter is closed as
// not received so the client can resend it under the same client message id.
// Fleet never resends on its own: it does not keep the message text.
func (s *Service) reconcileSupportMessages(ctx context.Context, organizationID string) error {
	if s.o.MessageStore == nil || s.o.SupportMessages == nil || !s.o.SupportMessages.Configured() {
		return nil
	}
	now := s.o.Now().UTC()
	pending, err := s.o.MessageStore.SupportMessagesNeedingReadBack(ctx, organizationID, now.Add(-supportMessageReadBackAfter))
	if err != nil {
		return err
	}
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "support-message-reconciler", CorrelationID: ids.New("cor")}
	for _, l := range pending {
		msg, err := s.o.SupportMessages.ReadMessage(ctx, organizationID, l.SupportCaseID, l.ClientMessageID)
		if err == nil {
			if _, err := s.confirmSupportMessage(ctx, t, l, msg); err != nil {
				return err
			}
			continue
		}
		if !isPlatformNotFound(err) {
			return err
		}
		if now.Sub(l.CreatedAt) >= supportMessageGiveUpAfter {
			if _, err := s.o.MessageStore.RecordSupportMessageAttempt(ctx, nil, organizationID, l.ID, models.SupportMessageRejected, "", "not_received"); err != nil {
				return err
			}
		}
	}
	return nil
}
