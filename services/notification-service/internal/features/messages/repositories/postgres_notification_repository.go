package messagerepositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	messagemodels "cosmicforge/logistics/services/notification-service/internal/features/messages/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/notifications"
)

type CreateMessageInput struct {
	Request      notifications.Request
	Channels     []string
	TemplateKey  *string
	Locale       string
	Title        string
	Body         string
	Priority     string
	InitialState string
}

type Repository interface {
	CreateMessageWithDeliveries(ctx context.Context, input CreateMessageInput) (messagemodels.Message, []messagemodels.Delivery, bool, error)
	GetMessage(ctx context.Context, id string) (messagemodels.Message, error)
	ListMessages(ctx context.Context, recipientType string, recipientID string, limit int) ([]messagemodels.Message, error)
	GetDelivery(ctx context.Context, id string) (messagemodels.Delivery, error)
	ListDueRetryDeliveries(ctx context.Context, limit int) ([]messagemodels.Delivery, error)
	MarkDelivery(ctx context.Context, id string, status string, nextAttemptAt *time.Time, providerMessageID *string, lastError *string) error
	RecordAttempt(ctx context.Context, deliveryID string, provider string, providerMessageID *string, status string, errorMessage *string) error
	GetTemplate(ctx context.Context, key string, locale string) (messagemodels.Template, bool, error)
	IsChannelEnabled(ctx context.Context, recipientType string, recipientID string, channel string) (bool, error)
	UpsertDevice(ctx context.Context, device messagemodels.Device) (messagemodels.Device, error)
	ListActiveDevices(ctx context.Context, recipientType string, recipientID string) ([]messagemodels.Device, error)
	DeactivateDeviceToken(ctx context.Context, token string) error
}

type PostgresNotificationRepository struct {
	db *pgxpool.Pool
}

func NewPostgresNotificationRepository(db *pgxpool.Pool) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{db: db}
}

func (r *PostgresNotificationRepository) CreateMessageWithDeliveries(ctx context.Context, input CreateMessageInput) (messagemodels.Message, []messagemodels.Delivery, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return messagemodels.Message{}, nil, false, err
	}
	defer tx.Rollback(ctx)

	channelsJSON, err := json.Marshal(input.Channels)
	if err != nil {
		return messagemodels.Message{}, nil, false, err
	}
	dataJSON, err := json.Marshal(emptyMap(input.Request.Data))
	if err != nil {
		return messagemodels.Message{}, nil, false, err
	}
	templateDataJSON, err := json.Marshal(emptyMap(input.Request.TemplateData))
	if err != nil {
		return messagemodels.Message{}, nil, false, err
	}

	messageID := uuid.NewString()
	status := input.InitialState
	if status == "" {
		status = messagemodels.StatusQueued
	}
	priority := input.Priority
	if priority == "" {
		priority = notifications.PriorityNormal
	}
	locale := input.Locale
	if locale == "" {
		locale = "en-NG"
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO notification_messages (
			id,
			idempotency_key,
			source_service,
			event_type,
			recipient_type,
			recipient_id,
			recipient_email,
			recipient_phone,
			channels,
			template_key,
			locale,
			title,
			body,
			data,
			template_data,
			priority,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id::text, idempotency_key, source_service, event_type, recipient_type, recipient_id, recipient_email, recipient_phone, channels, template_key, locale, title, body, data, template_data, priority, status, created_at, updated_at
	`,
		messageID,
		input.Request.IDempotencyKey,
		input.Request.SourceService,
		input.Request.EventName(),
		input.Request.Recipient.Type,
		input.Request.Recipient.ID,
		input.Request.Recipient.Email,
		input.Request.Recipient.Phone,
		channelsJSON,
		input.TemplateKey,
		locale,
		input.Title,
		input.Body,
		dataJSON,
		templateDataJSON,
		priority,
		status,
	)

	message, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := r.getMessageTx(ctx, tx, input.Request.IDempotencyKey, true)
		if err != nil {
			return messagemodels.Message{}, nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return messagemodels.Message{}, nil, false, err
		}
		return existing, nil, true, nil
	}
	if err != nil {
		return messagemodels.Message{}, nil, false, err
	}

	deliveries := make([]messagemodels.Delivery, 0, len(input.Channels))
	for _, channel := range input.Channels {
		deliveryID := uuid.NewString()
		deliveryRow := tx.QueryRow(ctx, `
			INSERT INTO notification_deliveries (id, message_id, channel, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id::text, message_id::text, channel, status, attempts, provider, provider_message_id, last_error, next_attempt_at, created_at, updated_at
		`, deliveryID, message.ID, channel, messagemodels.StatusQueued)
		delivery, err := scanDelivery(deliveryRow)
		if err != nil {
			return messagemodels.Message{}, nil, false, err
		}
		deliveries = append(deliveries, delivery)
	}

	if err := tx.Commit(ctx); err != nil {
		return messagemodels.Message{}, nil, false, err
	}

	return message, deliveries, false, nil
}

func (r *PostgresNotificationRepository) GetMessage(ctx context.Context, id string) (messagemodels.Message, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, idempotency_key, source_service, event_type, recipient_type, recipient_id, recipient_email, recipient_phone, channels, template_key, locale, title, body, data, template_data, priority, status, created_at, updated_at
		FROM notification_messages
		WHERE id = $1
	`, id)

	message, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return messagemodels.Message{}, apperrors.NotFound("Notification message could not be found.", err)
	}
	return message, err
}

func (r *PostgresNotificationRepository) ListMessages(ctx context.Context, recipientType string, recipientID string, limit int) ([]messagemodels.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id::text, idempotency_key, source_service, event_type, recipient_type, recipient_id, recipient_email, recipient_phone, channels, template_key, locale, title, body, data, template_data, priority, status, created_at, updated_at
		FROM notification_messages
		WHERE recipient_type = $1 AND recipient_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, recipientType, recipientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []messagemodels.Message
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *PostgresNotificationRepository) GetDelivery(ctx context.Context, id string) (messagemodels.Delivery, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, message_id::text, channel, status, attempts, provider, provider_message_id, last_error, next_attempt_at, created_at, updated_at
		FROM notification_deliveries
		WHERE id = $1
	`, id)
	delivery, err := scanDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return messagemodels.Delivery{}, apperrors.NotFound("Notification delivery could not be found.", err)
	}
	return delivery, err
}

func (r *PostgresNotificationRepository) ListDueRetryDeliveries(ctx context.Context, limit int) ([]messagemodels.Delivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id::text, message_id::text, channel, status, attempts, provider, provider_message_id, last_error, next_attempt_at, created_at, updated_at
		FROM notification_deliveries
		WHERE status = $1 AND next_attempt_at <= now()
		ORDER BY next_attempt_at ASC
		LIMIT $2
	`, messagemodels.StatusRetrying, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []messagemodels.Delivery
	for rows.Next() {
		delivery, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (r *PostgresNotificationRepository) MarkDelivery(ctx context.Context, id string, status string, nextAttemptAt *time.Time, providerMessageID *string, lastError *string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = $2,
		    attempts = attempts + 1,
		    next_attempt_at = $3,
		    provider_message_id = COALESCE($4, provider_message_id),
		    last_error = $5,
		    updated_at = now()
		WHERE id = $1
	`, id, status, nextAttemptAt, providerMessageID, lastError)
	return err
}

func (r *PostgresNotificationRepository) RecordAttempt(ctx context.Context, deliveryID string, provider string, providerMessageID *string, status string, errorMessage *string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_delivery_attempts (
			delivery_id,
			provider,
			provider_message_id,
			status,
			error_message
		)
		VALUES ($1, $2, $3, $4, $5)
	`, deliveryID, provider, providerMessageID, status, errorMessage)
	return err
}

func (r *PostgresNotificationRepository) GetTemplate(ctx context.Context, key string, locale string) (messagemodels.Template, bool, error) {
	if locale == "" {
		locale = "en-NG"
	}
	row := r.db.QueryRow(ctx, `
		SELECT key, locale, title, body, default_channels, active, created_at, updated_at
		FROM notification_templates
		WHERE key = $1 AND active = true AND locale IN ($2, 'default')
		ORDER BY CASE WHEN locale = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`, key, locale)

	template, err := scanTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return messagemodels.Template{}, false, nil
	}
	return template, true, err
}

func (r *PostgresNotificationRepository) IsChannelEnabled(ctx context.Context, recipientType string, recipientID string, channel string) (bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT enabled
		FROM notification_preferences
		WHERE recipient_type = $1 AND recipient_id = $2 AND channel = $3
	`, recipientType, recipientID, channel)

	var enabled bool
	err := row.Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return enabled, err
}

func (r *PostgresNotificationRepository) UpsertDevice(ctx context.Context, device messagemodels.Device) (messagemodels.Device, error) {
	if device.ID == "" {
		device.ID = uuid.NewString()
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO notification_devices (id, recipient_type, recipient_id, token, platform, app, active)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		ON CONFLICT (token) DO UPDATE
		SET recipient_type = EXCLUDED.recipient_type,
		    recipient_id = EXCLUDED.recipient_id,
		    platform = EXCLUDED.platform,
		    app = EXCLUDED.app,
		    active = true,
		    updated_at = now()
		RETURNING id::text, recipient_type, recipient_id, token, platform, app, active, created_at, updated_at
	`, device.ID, device.RecipientType, device.RecipientID, device.Token, device.Platform, device.App)
	return scanDevice(row)
}

func (r *PostgresNotificationRepository) ListActiveDevices(ctx context.Context, recipientType string, recipientID string) ([]messagemodels.Device, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, recipient_type, recipient_id, token, platform, app, active, created_at, updated_at
		FROM notification_devices
		WHERE recipient_type = $1 AND recipient_id = $2 AND active = true
		ORDER BY updated_at DESC
	`, recipientType, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []messagemodels.Device
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (r *PostgresNotificationRepository) DeactivateDeviceToken(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE notification_devices
		SET active = false, updated_at = now()
		WHERE token = $1
	`, token)
	return err
}

func (r *PostgresNotificationRepository) getMessageTx(ctx context.Context, tx pgx.Tx, key string, byIdempotencyKey bool) (messagemodels.Message, error) {
	column := "id"
	if byIdempotencyKey {
		column = "idempotency_key"
	}
	row := tx.QueryRow(ctx, `
		SELECT id::text, idempotency_key, source_service, event_type, recipient_type, recipient_id, recipient_email, recipient_phone, channels, template_key, locale, title, body, data, template_data, priority, status, created_at, updated_at
		FROM notification_messages
		WHERE `+column+` = $1
	`, key)
	return scanMessage(row)
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanMessage(row scanner) (messagemodels.Message, error) {
	var message messagemodels.Message
	var channelsJSON []byte
	var dataJSON []byte
	var templateDataJSON []byte

	err := row.Scan(
		&message.ID,
		&message.IdempotencyKey,
		&message.SourceService,
		&message.EventType,
		&message.RecipientType,
		&message.RecipientID,
		&message.RecipientEmail,
		&message.RecipientPhone,
		&channelsJSON,
		&message.TemplateKey,
		&message.Locale,
		&message.Title,
		&message.Body,
		&dataJSON,
		&templateDataJSON,
		&message.Priority,
		&message.Status,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if err != nil {
		return messagemodels.Message{}, err
	}
	if err := json.Unmarshal(channelsJSON, &message.Channels); err != nil {
		return messagemodels.Message{}, err
	}
	if len(dataJSON) > 0 {
		_ = json.Unmarshal(dataJSON, &message.Data)
	}
	if len(templateDataJSON) > 0 {
		_ = json.Unmarshal(templateDataJSON, &message.TemplateData)
	}
	if message.Data == nil {
		message.Data = map[string]interface{}{}
	}
	if message.TemplateData == nil {
		message.TemplateData = map[string]interface{}{}
	}
	return message, nil
}

func scanDelivery(row scanner) (messagemodels.Delivery, error) {
	var delivery messagemodels.Delivery
	err := row.Scan(
		&delivery.ID,
		&delivery.MessageID,
		&delivery.Channel,
		&delivery.Status,
		&delivery.Attempts,
		&delivery.Provider,
		&delivery.ProviderMessageID,
		&delivery.LastError,
		&delivery.NextAttemptAt,
		&delivery.CreatedAt,
		&delivery.UpdatedAt,
	)
	return delivery, err
}

func scanTemplate(row scanner) (messagemodels.Template, error) {
	var template messagemodels.Template
	var channelsJSON []byte
	err := row.Scan(
		&template.Key,
		&template.Locale,
		&template.Title,
		&template.Body,
		&channelsJSON,
		&template.Active,
		&template.CreatedAt,
		&template.UpdatedAt,
	)
	if err != nil {
		return messagemodels.Template{}, err
	}
	if err := json.Unmarshal(channelsJSON, &template.DefaultChannels); err != nil {
		return messagemodels.Template{}, err
	}
	return template, nil
}

func scanDevice(row scanner) (messagemodels.Device, error) {
	var device messagemodels.Device
	err := row.Scan(
		&device.ID,
		&device.RecipientType,
		&device.RecipientID,
		&device.Token,
		&device.Platform,
		&device.App,
		&device.Active,
		&device.CreatedAt,
		&device.UpdatedAt,
	)
	return device, err
}

func emptyMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}
