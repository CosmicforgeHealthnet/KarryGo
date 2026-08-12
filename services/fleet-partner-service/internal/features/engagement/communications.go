package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

var validSubjects = map[string]bool{"CUSTOMER": true, "PROVIDER": true, "ORDER": true, "SHIPMENT": true, "ROUTE": true, "TRIP": true, "INCIDENT": true, "CLAIM": true, "INVOICE": true, "SUPPORT_CASE": true, "QUALITY_CASE": true, "OTHER": true}
var validVisibility = map[string]bool{"INTERNAL": true, "CUSTOMER_VISIBLE": true, "PROVIDER_VISIBLE": true, "RESTRICTED": true}
var validChannels = map[string]bool{"IN_APP": true, "EMAIL": true, "SMS": true, "PUSH": true, "WHATSAPP": true, "INTERNAL_NOTE": true}

func (s *Service) CreateThread(ctx context.Context, t tenant.Context, in ThreadInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	in.SubjectType = strings.ToUpper(in.SubjectType)
	in.Visibility = strings.ToUpper(in.Visibility)
	if !validSubjects[in.SubjectType] || !validVisibility[in.Visibility] || strings.TrimSpace(in.SubjectReference) == "" || strings.TrimSpace(in.Title) == "" {
		return nil, apperrors.Validation("Communication thread subject, title and visibility are invalid.", nil)
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	id := ids.New("cth")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_communication_threads(id,organization_id,branch_id,subject_type,subject_reference,title,visibility,created_by,owner_user_id) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9)`, id, t.OrganizationID, in.BranchID, in.SubjectType, in.SubjectReference, in.Title, in.Visibility, t.ActorID, in.OwnerUserID); err != nil {
			return err
		}
		participants := append([]ParticipantInput{{ParticipantType: "USER", ParticipantReference: t.ActorID, AccessLevel: "READ_WRITE"}}, in.Participants...)
		seen := map[string]bool{}
		for _, p := range participants {
			p.ParticipantType = strings.ToUpper(p.ParticipantType)
			p.AccessLevel = strings.ToUpper(p.AccessLevel)
			if p.AccessLevel == "" {
				p.AccessLevel = "READ_WRITE"
			}
			key := p.ParticipantType + ":" + p.ParticipantReference
			if seen[key] {
				continue
			}
			seen[key] = true
			if p.ParticipantReference == "" || (p.ParticipantType != "USER" && p.ParticipantType != "CUSTOMER" && p.ParticipantType != "PROVIDER" && p.ParticipantType != "SERVICE") {
				return apperrors.Validation("Thread participant is invalid.", nil)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_communication_participants(thread_id,organization_id,participant_type,participant_reference,access_level,added_by) VALUES($1,$2,$3,$4,$5,$6)`, id, t.OrganizationID, p.ParticipantType, p.ParticipantReference, p.AccessLevel, t.ActorID); err != nil {
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

func (s *Service) participant(ctx context.Context, t tenant.Context, threadID string) (bool, error) {
	var ok bool
	err := s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_communication_participants WHERE organization_id=$1 AND thread_id=$2 AND participant_type='USER' AND participant_reference=$3)`, t.OrganizationID, threadID, t.ActorID).Scan(&ok)
	return ok, err
}
func (s *Service) GetThread(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, false); err != nil {
		return nil, err
	}
	ok, err := s.participant(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT th.*,COALESCE((SELECT json_agg(row_to_json(p) ORDER BY p.created_at) FROM fleet_partner_communication_participants p WHERE p.organization_id=th.organization_id AND p.thread_id=th.id),'[]'::json) participants FROM fleet_partner_communication_threads th WHERE th.organization_id=$1 AND th.id=$2) x`, t.OrganizationID, id))
}
func (s *Service) ListThreads(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(th) FROM fleet_partner_communication_threads th JOIN fleet_partner_communication_participants p ON p.thread_id=th.id AND p.organization_id=th.organization_id WHERE th.organization_id=$1 AND p.participant_type='USER' AND p.participant_reference=$2 AND (cardinality($3::text[])=0 OR th.branch_id IS NULL OR th.branch_id=ANY($3)) ORDER BY th.updated_at DESC`, t.OrganizationID, t.ActorID, t.PermittedBranchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) UpdateThread(ctx context.Context, t tenant.Context, id string, in ThreadInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	ok, err := s.participant(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_communication_threads WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version); err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Communication thread version changed.", nil)
		}
		if status == "CLOSED" || status == "ARCHIVED" {
			return apperrors.Conflict("Closed communication threads are immutable.", nil)
		}
		newStatus := strings.ToUpper(in.Status)
		if newStatus == "" {
			newStatus = status
		}
		if newStatus != "OPEN" && newStatus != "PENDING_RESPONSE" && newStatus != "RESOLVED" {
			return apperrors.Validation("Thread status is invalid.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_communication_threads SET title=CASE WHEN $3='' THEN title ELSE $3 END,owner_user_id=CASE WHEN $4='' THEN owner_user_id ELSE $4 END,status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, in.Title, in.OwnerUserID, newStatus).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"threadId": id, "status": newStatus, "title": in.Title, "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_communication_thread_history(id,organization_id,thread_id,previous_status,new_status,snapshot,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("cth"), t.OrganizationID, id, status, newStatus, snapshot, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "communication_thread.updated.v1", "communication_thread", id, nv, map[string]any{"threadId": id, "status": newStatus})
	})
	if err != nil {
		return nil, err
	}
	return s.GetThread(ctx, t, id)
}
func (s *Service) CloseThread(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	ok, err := s.participant(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_communication_threads WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version); err != nil {
			return err
		}
		if version != expected {
			return apperrors.Conflict("Communication thread version changed.", nil)
		}
		if status == "CLOSED" {
			return nil
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_communication_threads SET status='CLOSED',closed_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"threadId": id, "status": "CLOSED", "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_communication_thread_history(id,organization_id,thread_id,previous_status,new_status,snapshot,actor_reference,version) VALUES($1,$2,$3,$4,'CLOSED',$5,$6,$7)`, ids.New("cth"), t.OrganizationID, id, status, snapshot, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "communication_thread.closed.v1", "communication_thread", id, nv, map[string]any{"threadId": id})
	})
	if err != nil {
		return nil, err
	}
	return s.GetThread(ctx, t, id)
}

func (s *Service) SendMessage(ctx context.Context, t tenant.Context, threadID, key string, in MessageInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, true); err != nil {
		return nil, err
	}
	in.Channel = strings.ToUpper(in.Channel)
	if !validChannels[in.Channel] || strings.TrimSpace(in.Text) == "" {
		return nil, apperrors.Validation("Message channel and safe text are required.", nil)
	}
	if strings.TrimSpace(key) == "" {
		return nil, apperrors.Validation("Message idempotency key is required.", nil)
	}
	if existing, err := jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_communication_messages m WHERE organization_id=$1 AND idempotency_key=$2`, t.OrganizationID, key)); err == nil {
		return existing, nil
	}
	if in.SourceTimestamp.IsZero() {
		in.SourceTimestamp = s.o.Now().UTC()
	}
	if err := s.validateMedia(ctx, t, in.MediaFileIDs); err != nil {
		return nil, err
	}
	messageID := ids.New("cmsg")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, visibility string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT th.status,th.visibility,th.version FROM fleet_partner_communication_threads th JOIN fleet_partner_communication_participants p ON p.thread_id=th.id AND p.organization_id=th.organization_id WHERE th.organization_id=$1 AND th.id=$2 AND p.participant_type='USER' AND p.participant_reference=$3 FOR UPDATE OF th`, t.OrganizationID, threadID, t.ActorID).Scan(&status, &visibility, &version); err != nil {
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
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_communication_messages(id,organization_id,thread_id,sender_type,sender_reference,channel,text,media_file_ids,delivery_status,reply_to_message_id,idempotency_key,source_timestamp) VALUES($1,$2,$3,'USER',$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11)`, messageID, t.OrganizationID, threadID, t.ActorID, in.Channel, in.Text, media, deliveryStatus, in.ReplyToMessageID, key, in.SourceTimestamp); err != nil {
			return err
		}
		if in.Channel != "INTERNAL_NOTE" {
			rows, err := tx.Query(ctx, `SELECT participant_type,participant_reference FROM fleet_partner_communication_participants WHERE organization_id=$1 AND thread_id=$2 AND participant_type IN('CUSTOMER','PROVIDER') ORDER BY participant_type,participant_reference`, t.OrganizationID, threadID)
			if err != nil {
				return err
			}
			type recipient struct{ typ, ref string }
			var recipients []recipient
			for rows.Next() {
				var item recipient
				if err := rows.Scan(&item.typ, &item.ref); err != nil {
					rows.Close()
					return err
				}
				recipients = append(recipients, item)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			created := 0
			for _, item := range recipients {
				typ, ref := item.typ, item.ref
				intentID := ids.New("nti")
				safe := map[string]any{"threadId": threadID, "messageId": messageID}
				if visibility != "RESTRICTED" {
					safe["text"] = in.Text
				}
				payload, _ := json.Marshal(safe)
				channels, _ := json.Marshal([]string{in.Channel})
				intentKey := key + ":" + typ + ":" + ref
				if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_notification_intents(id,organization_id,source_type,source_reference,recipient_type,recipient_reference,channels,template_code,safe_payload,idempotency_key) VALUES($1,$2,'COMMUNICATION_MESSAGE',$3,$4,$5,$6,'fleet.communication.message',$7,$8)`, intentID, t.OrganizationID, messageID, typ, ref, channels, payload, intentKey); err != nil {
					return err
				}
				if _, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: "SECTION23_NOTIFICATION_SUBMIT", DedupeKey: intentID, Payload: map[string]any{"organizationId": t.OrganizationID, "intentId": intentID}, MaxAttempts: 8}); err != nil {
					return err
				}
				created++
			}
			if created == 0 {
				return apperrors.Validation("External messages require an authorized customer or provider participant.", nil)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE fleet_partner_communication_threads SET status='PENDING_RESPONSE',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, threadID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "communication_message.created.v1", "communication_message", messageID, 1, map[string]any{"messageId": messageID, "threadId": threadID, "channel": in.Channel, "deliveryStatus": deliveryStatus})
	})
	if err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_communication_messages m WHERE organization_id=$1 AND id=$2`, t.OrganizationID, messageID))
}
func (s *Service) ListMessages(ctx context.Context, t tenant.Context, threadID string) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Communications, false); err != nil {
		return nil, err
	}
	ok, err := s.participant(ctx, t, threadID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(m) FROM fleet_partner_communication_messages m WHERE organization_id=$1 AND thread_id=$2 ORDER BY created_at`, t.OrganizationID, threadID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

func validateQuietHours(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var v struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return apperrors.Validation("Quiet hours must be valid JSON.", nil)
	}
	for _, x := range []string{v.Start, v.End} {
		if x == "" {
			continue
		}
		if _, err := time.Parse("15:04", x); err != nil {
			return apperrors.Validation("Quiet hours must use HH:MM.", nil)
		}
	}
	return nil
}
func (s *Service) SaveNotificationRule(ctx context.Context, t tenant.Context, in NotificationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.NotificationRules, true); err != nil {
		return nil, err
	}
	if in.EventType == "" || in.RecipientScope == "" || in.Timezone == "" {
		return nil, apperrors.Validation("Notification event, recipient scope and timezone are required.", nil)
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, apperrors.Validation("Timezone must be a valid IANA name.", nil)
	}
	if err := validateQuietHours(in.QuietHours); err != nil {
		return nil, err
	}
	channels := rawOr(in.Channels, "[]")
	quiet := rawOr(in.QuietHours, "{}")
	escalation := rawOr(in.EscalationPolicy, "{}")
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	id := ids.New("ntr")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var existingID string
		var version int64
		err := tx.QueryRow(ctx, `SELECT id,version FROM fleet_partner_notification_rules WHERE organization_id=$1 AND event_type=$2 AND recipient_scope=$3 FOR UPDATE`, t.OrganizationID, in.EventType, in.RecipientScope).Scan(&existingID, &version)
		if err == nil {
			if in.ExpectedVersion <= 0 || version != in.ExpectedVersion {
				return apperrors.Conflict("Notification rule version changed.", nil)
			}
			id = existingID
			if _, err := tx.Exec(ctx, `UPDATE fleet_partner_notification_rules SET channels=$3,quiet_hours=$4,timezone=$5,severity_threshold=$6,escalation_policy=$7,enabled=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id, channels, quiet, in.Timezone, strings.ToUpper(in.SeverityThreshold), escalation, enabled); err != nil {
				return err
			}
			version++
			return s.record(ctx, tx, t, "notification_rule.updated.v1", "notification_rule", id, version, map[string]any{"notificationRuleId": id, "eventType": in.EventType, "enabled": enabled})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_notification_rules(id,organization_id,event_type,recipient_scope,channels,quiet_hours,timezone,severity_threshold,escalation_policy,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, t.OrganizationID, in.EventType, in.RecipientScope, channels, quiet, in.Timezone, strings.ToUpper(in.SeverityThreshold), escalation, enabled); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "notification_rule.created.v1", "notification_rule", id, 1, map[string]any{"notificationRuleId": id, "eventType": in.EventType, "enabled": enabled})
	})
	if err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(r) FROM fleet_partner_notification_rules r WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}
func (s *Service) ListNotificationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.NotificationRules, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(r) FROM fleet_partner_notification_rules r WHERE organization_id=$1 ORDER BY event_type,recipient_scope`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
