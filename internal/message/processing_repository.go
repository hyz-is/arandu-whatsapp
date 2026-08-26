package message

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
)

var errMessageProcessingSnapshotNotFound = errors.New("message processing snapshot not found")

type messageProcessingSnapshot struct {
	ProcessID          string
	MessageJobID       string
	CleanupJobID       string
	InstanceID         int64
	InstanceName       string
	RemoteJID          string
	MessageID          string
	MessageType        string
	MessagePayload     []byte
	Content            json.RawMessage
	Presence           *string
	Delay              time.Duration
	ExternalAttributes map[string]any
	WebhookInstance    webhooksvc.WebhookInstance
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type messageProcessingRepository interface {
	Create(context.Context, security.Grant, messageProcessingSnapshot) error
	Find(context.Context, security.Grant, string) (messageProcessingSnapshot, error)
	FindJobIDs(context.Context, security.Grant, string) (string, string, error)
	Delete(context.Context, security.Grant, string) error
}

type sqlMessageProcessingRepository struct{ db *data.DB }

func newSQLMessageProcessingRepository(db *data.DB) *sqlMessageProcessingRepository {
	return &sqlMessageProcessingRepository{db: db}
}

func (r *sqlMessageProcessingRepository) Create(ctx context.Context, grant security.Grant, item messageProcessingSnapshot) error {
	tenant, err := messageProcessingTenant(grant)
	if err != nil {
		return err
	}
	externalAttributes, err := json.Marshal(item.ExternalAttributes)
	if err != nil {
		return fmt.Errorf("encode message processing external attributes: %w", err)
	}
	webhookInstance, err := json.Marshal(item.WebhookInstance)
	if err != nil {
		return fmt.Errorf("encode message processing webhook instance: %w", err)
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `INSERT INTO whatsapp_message_jobs (
		process_id, tenant_id, message_job_id, cleanup_job_id, instance_id, instance_name, remote_jid, message_id,
		message_type, message_payload, content, presence, delay_ms,
		external_attributes, webhook_instance, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ProcessID,
		tenant, item.MessageJobID, item.CleanupJobID, item.InstanceID, item.InstanceName, item.RemoteJID, item.MessageID,
		item.MessageType, base64.StdEncoding.EncodeToString(item.MessagePayload),
		string(item.Content), item.Presence, item.Delay.Milliseconds(),
		string(externalAttributes), string(webhookInstance), now, now)
	if err != nil {
		return fmt.Errorf("create message processing snapshot: %w", err)
	}
	return nil
}

func (r *sqlMessageProcessingRepository) Find(ctx context.Context, grant security.Grant, processID string) (messageProcessingSnapshot, error) {
	tenant, err := messageProcessingTenant(grant)
	if err != nil {
		return messageProcessingSnapshot{}, err
	}
	var item messageProcessingSnapshot
	var payload string
	var content string
	var presence sql.NullString
	var delayMilliseconds int64
	var externalAttributes string
	var webhookInstance string
	err = r.db.QueryRowContext(ctx, `SELECT process_id, message_job_id, cleanup_job_id, instance_id, instance_name,
		remote_jid, message_id, message_type, message_payload, content, presence,
		delay_ms, external_attributes, webhook_instance, created_at, updated_at
		FROM whatsapp_message_jobs WHERE tenant_id = ? AND process_id = ?`, tenant, processID).
		Scan(&item.ProcessID, &item.MessageJobID, &item.CleanupJobID, &item.InstanceID, &item.InstanceName, &item.RemoteJID,
			&item.MessageID, &item.MessageType, &payload, &content, &presence,
			&delayMilliseconds, &externalAttributes, &webhookInstance,
			&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return messageProcessingSnapshot{}, errMessageProcessingSnapshotNotFound
	}
	if err != nil {
		return messageProcessingSnapshot{}, fmt.Errorf("find message processing snapshot: %w", err)
	}
	item.MessagePayload, err = base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return messageProcessingSnapshot{}, fmt.Errorf("%w: decode protobuf payload: %v", errInvalidMessageProcessingSnapshot, err)
	}
	item.Content = json.RawMessage(content)
	if !json.Valid(item.Content) {
		return messageProcessingSnapshot{}, fmt.Errorf("%w: content is invalid JSON", errInvalidMessageProcessingSnapshot)
	}
	if err := json.Unmarshal([]byte(externalAttributes), &item.ExternalAttributes); err != nil {
		return messageProcessingSnapshot{}, fmt.Errorf("%w: decode external attributes: %v", errInvalidMessageProcessingSnapshot, err)
	}
	if err := json.Unmarshal([]byte(webhookInstance), &item.WebhookInstance); err != nil {
		return messageProcessingSnapshot{}, fmt.Errorf("%w: decode webhook instance: %v", errInvalidMessageProcessingSnapshot, err)
	}
	item.Presence = nullableString(presence)
	item.Delay = time.Duration(delayMilliseconds) * time.Millisecond
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func (r *sqlMessageProcessingRepository) FindJobIDs(ctx context.Context, grant security.Grant, processID string) (string, string, error) {
	tenant, err := messageProcessingTenant(grant)
	if err != nil {
		return "", "", err
	}
	var messageJobID, cleanupJobID string
	err = r.db.QueryRowContext(ctx, `SELECT message_job_id, cleanup_job_id FROM whatsapp_message_jobs
		WHERE tenant_id = ? AND process_id = ?`, tenant, processID).Scan(&messageJobID, &cleanupJobID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", errMessageProcessingSnapshotNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("find message processing job ids: %w", err)
	}
	return messageJobID, cleanupJobID, nil
}

func (r *sqlMessageProcessingRepository) Delete(ctx context.Context, grant security.Grant, processID string) error {
	tenant, err := messageProcessingTenant(grant)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`DELETE FROM whatsapp_message_jobs WHERE tenant_id = ? AND process_id = ?`,
		tenant, processID)
	if err != nil {
		return fmt.Errorf("delete message processing snapshot: %w", err)
	}
	return nil
}

func messageProcessingTenant(grant security.Grant) (string, error) {
	if err := grant.Check(authz.ActionMessageSend); err != nil {
		return "", err
	}
	tenant := data.Tenant(grant)
	if tenant == "" {
		return "", fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	return tenant, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
