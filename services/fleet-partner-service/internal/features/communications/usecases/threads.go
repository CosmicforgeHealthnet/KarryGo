package usecases

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// CreateThread opens a thread with the caller as its first participant.
func (s *Service) CreateThread(ctx context.Context, t tenant.Context, in models.ThreadInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	in, err := models.NormalizeThread(in)
	if err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	participants, err := models.ThreadParticipants(t.ActorID, in.Participants)
	if err != nil {
		return nil, err
	}
	id := ids.New("cth")
	err = s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertThread(ctx, tx, id, t.OrganizationID, t.ActorID, in); err != nil {
			return err
		}
		for _, p := range participants {
			if err := s.o.Store.InsertParticipant(ctx, tx, id, t.OrganizationID, t.ActorID, p); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "communication_thread.created.v1", "communication_thread", id, 1, map[string]any{"threadId": id, "subjectType": in.SubjectType, "subjectReference": in.SubjectReference, "visibility": in.Visibility})
	})
	if err != nil {
		return nil, err
	}
	return s.GetThread(ctx, t, id)
}

func (s *Service) requireParticipant(ctx context.Context, t tenant.Context, threadID string) error {
	ok, err := s.o.Store.IsUserParticipant(ctx, t.OrganizationID, threadID, t.ActorID)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

// GetThread reads a thread the caller participates in.
func (s *Service) GetThread(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, false); err != nil {
		return nil, err
	}
	if err := s.requireParticipant(ctx, t, id); err != nil {
		return nil, err
	}
	return s.o.Store.GetThread(ctx, t.OrganizationID, id)
}

// ListThreads lists the threads the caller participates in.
func (s *Service) ListThreads(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListThreadsForUser(ctx, t.OrganizationID, t.ActorID, t.PermittedBranchIDs)
}

// UpdateThread edits an open thread at the expected version.
func (s *Service) UpdateThread(ctx context.Context, t tenant.Context, id string, in models.ThreadInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	if err := s.requireParticipant(ctx, t, id); err != nil {
		return nil, err
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, version, err := s.o.Store.LockThread(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Communication thread version changed.", nil)
		}
		newStatus, err := models.NextThreadStatus(status, in.Status)
		if err != nil {
			return err
		}
		nv, err := s.o.Store.UpdateThread(ctx, tx, t.OrganizationID, id, in.Title, in.OwnerUserID, newStatus)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"threadId": id, "status": newStatus, "title": in.Title, "version": nv})
		if err := s.o.Store.InsertThreadHistory(ctx, tx, t.OrganizationID, id, status, newStatus, snapshot, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "communication_thread.updated.v1", "communication_thread", id, nv, map[string]any{"threadId": id, "status": newStatus})
	})
	if err != nil {
		return nil, err
	}
	return s.GetThread(ctx, t, id)
}

// CloseThread closes a thread; closing a closed thread is a no-op.
func (s *Service) CloseThread(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if err := s.requireParticipant(ctx, t, id); err != nil {
		return nil, err
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, version, err := s.o.Store.LockThread(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != expected {
			return apperrors.Conflict("Communication thread version changed.", nil)
		}
		if status == "CLOSED" {
			return nil
		}
		nv, err := s.o.Store.CloseThread(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"threadId": id, "status": "CLOSED", "version": nv})
		if err := s.o.Store.InsertThreadHistory(ctx, tx, t.OrganizationID, id, status, "CLOSED", snapshot, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "communication_thread.closed.v1", "communication_thread", id, nv, map[string]any{"threadId": id})
	})
	if err != nil {
		return nil, err
	}
	return s.GetThread(ctx, t, id)
}

// SendMessage posts a message. External channels create one notification
// intent per customer or provider participant; delivery runs afterwards so
// a notification-service outage never blocks the message itself.
func (s *Service) SendMessage(ctx context.Context, t tenant.Context, threadID, key string, in models.MessageInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	in.Channel = strings.ToUpper(in.Channel)
	if !models.ValidMessageChannel(in.Channel) || strings.TrimSpace(in.Text) == "" {
		return nil, apperrors.Validation("Message channel and safe text are required.", nil)
	}
	if strings.TrimSpace(key) == "" {
		return nil, apperrors.Validation("Message idempotency key is required.", nil)
	}
	if existing, err := s.o.Store.MessageByIdempotencyKey(ctx, t.OrganizationID, key); err == nil {
		return existing, nil
	}
	if in.SourceTimestamp.IsZero() {
		in.SourceTimestamp = s.o.Now().UTC()
	}
	if err := s.validateMedia(ctx, t, in.MediaFileIDs); err != nil {
		return nil, err
	}
	messageID := ids.New("cmsg")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, visibility, err := s.o.Store.LockParticipantThread(ctx, tx, t.OrganizationID, threadID, t.ActorID)
		if err != nil {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if status == "CLOSED" || status == "ARCHIVED" {
			return apperrors.Conflict("Messages cannot be sent to a closed thread.", nil)
		}
		media, _ := json.Marshal(in.MediaFileIDs)
		deliveryStatus := "QUEUED"
		if in.Channel == "INTERNAL_NOTE" {
			deliveryStatus = "NOT_APPLICABLE"
		}
		if err := s.o.Store.InsertMessage(ctx, tx, messageID, t.OrganizationID, threadID, t.ActorID, in, media, deliveryStatus, key); err != nil {
			return err
		}
		if in.Channel != "INTERNAL_NOTE" {
			recipients, err := s.o.Store.ExternalRecipients(ctx, tx, t.OrganizationID, threadID)
			if err != nil {
				return err
			}
			created := 0
			for _, item := range recipients {
				safe := map[string]any{"threadId": threadID, "messageId": messageID}
				if visibility != "RESTRICTED" {
					safe["text"] = in.Text
				}
				payload, _ := json.Marshal(safe)
				channels, _ := json.Marshal([]string{in.Channel})
				if _, err := s.QueueIntentTx(ctx, tx, models.IntentInput{OrganizationID: t.OrganizationID, SourceType: "COMMUNICATION_MESSAGE", SourceReference: messageID, RecipientType: item.Type, RecipientReference: item.Reference, Channels: channels, TemplateCode: "fleet.communication.message", SafePayload: payload, IdempotencyKey: key + ":" + item.Type + ":" + item.Reference}); err != nil {
					return err
				}
				created++
			}
			if created == 0 {
				return apperrors.Validation("External messages require an authorized customer or provider participant.", nil)
			}
		}
		if err := s.o.Store.MarkThreadPendingResponse(ctx, tx, t.OrganizationID, threadID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "communication_message.created.v1", "communication_message", messageID, 1, map[string]any{"messageId": messageID, "threadId": threadID, "channel": in.Channel, "deliveryStatus": deliveryStatus})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetMessage(ctx, t.OrganizationID, messageID)
}

// ListMessages lists the messages of a thread the caller participates in.
func (s *Service) ListMessages(ctx context.Context, t tenant.Context, threadID string) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, false); err != nil {
		return nil, err
	}
	if err := s.requireParticipant(ctx, t, threadID); err != nil {
		return nil, err
	}
	return s.o.Store.ListMessages(ctx, t.OrganizationID, threadID)
}

// QueueIntentTx records a notification intent and schedules its delivery in
// the caller's transaction. It returns the intent ID.
func (s *Service) QueueIntentTx(ctx context.Context, tx pgx.Tx, in models.IntentInput) (string, error) {
	intentID := ids.New("nti")
	if err := s.applyPreference(ctx, tx, &in); err != nil {
		return "", err
	}
	if err := s.o.Store.InsertIntent(ctx, tx, intentID, in); err != nil {
		return "", err
	}
	if _, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: in.OrganizationID, JobType: jobNotificationSubmit, DedupeKey: intentID, Payload: map[string]any{"organizationId": in.OrganizationID, "intentId": intentID}, MaxAttempts: 8}); err != nil {
		return "", err
	}
	return intentID, nil
}
