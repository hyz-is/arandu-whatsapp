package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

var (
	errWebhookConfigurationNotFound = errors.New("webhook configuration not found")
	errWebhookDeliveryNotFound      = errors.New("webhook delivery not found")
)

type deliveryStatus string

const (
	deliveryStatusPending    deliveryStatus = "pending"
	deliveryStatusProcessing deliveryStatus = "processing"
	deliveryStatusDelivered  deliveryStatus = "delivered"
	deliveryStatusFailed     deliveryStatus = "failed"
)

type deliveryTarget string

const (
	deliveryTargetInstance deliveryTarget = "instance"
	deliveryTargetGlobal   deliveryTarget = "global"
)

type createDeliveryInput struct {
	ID         string
	InstanceID int64
	Event      types.WebhookEvent
	Target     deliveryTarget
	URL        string
	Body       []byte
	Headers    map[string]string
}

type delivery struct {
	ID             string
	InstanceID     int64
	Event          types.WebhookEvent
	Target         deliveryTarget
	URL            string
	Body           []byte
	Headers        map[string]string
	Status         deliveryStatus
	Attempts       int
	ResponseStatus *int
	ResponseBody   *string
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeliveredAt    *time.Time
}

type deliveryRepository interface {
	FindConfiguration(context.Context, security.Grant, int64) (types.Webhook, error)
	CreateDelivery(context.Context, security.Grant, createDeliveryInput) error
	FindDelivery(context.Context, security.Grant, string) (delivery, error)
	MarkAttempt(context.Context, security.Grant, string, int) error
	MarkDelivered(context.Context, security.Grant, string, int, int) error
	MarkFailed(context.Context, security.Grant, string, int, int, string) error
	PruneBefore(context.Context, security.Grant, time.Time) (int64, error)
}

type sqlDeliveryRepository struct{ db *data.DB }

func newSQLDeliveryRepository(db *data.DB) *sqlDeliveryRepository {
	return &sqlDeliveryRepository{db: db}
}

func (r *sqlDeliveryRepository) FindConfiguration(ctx context.Context, grant security.Grant, instanceID int64) (types.Webhook, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return types.Webhook{}, err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return types.Webhook{}, err
	}
	var item types.Webhook
	var events string
	err = r.db.QueryRowContext(ctx, `SELECT id, url, enabled, events, created_at, updated_at, instance_id
		FROM whatsapp_webhooks WHERE tenant_id = ? AND instance_id = ?`, tenant, instanceID).
		Scan(&item.ID, &item.URL, &item.Enabled, &events, &item.CreatedAt, &item.UpdatedAt, &item.InstanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Webhook{}, errWebhookConfigurationNotFound
	}
	if err != nil {
		return types.Webhook{}, fmt.Errorf("find webhook configuration for delivery: %w", err)
	}
	item.Events = json.RawMessage(events)
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func (r *sqlDeliveryRepository) CreateDelivery(ctx context.Context, grant security.Grant, input createDeliveryInput) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return err
	}
	snapshotHeaders := make(map[string]string, len(input.Headers)+1)
	for name, value := range input.Headers {
		snapshotHeaders[name] = value
	}
	snapshotHeaders["X-Arandu-Delivery-ID"] = input.ID
	headers, err := json.Marshal(snapshotHeaders)
	if err != nil {
		return fmt.Errorf("encode webhook delivery headers: %w", err)
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `INSERT INTO whatsapp_webhook_deliveries (
		id, tenant_id, instance_id, event, target, url, body, headers, status,
		attempts, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, input.ID, tenant, input.InstanceID,
		string(input.Event), string(input.Target), input.URL, string(input.Body), string(headers),
		string(deliveryStatusPending), 0, now, now)
	if err != nil {
		return fmt.Errorf("create webhook delivery: %w", err)
	}
	return nil
}

func (r *sqlDeliveryRepository) FindDelivery(ctx context.Context, grant security.Grant, id string) (delivery, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return delivery{}, err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return delivery{}, err
	}
	var item delivery
	var body string
	var headers string
	var event string
	var target string
	var status string
	var responseStatus sql.NullInt64
	var responseBody sql.NullString
	var lastError sql.NullString
	var deliveredAt sql.NullTime
	err = r.db.QueryRowContext(ctx, `SELECT id, instance_id, event, target, url, body, headers,
		status, attempts, response_status, response_body, last_error, created_at, updated_at, delivered_at
		FROM whatsapp_webhook_deliveries WHERE tenant_id = ? AND id = ?`, tenant, id).
		Scan(&item.ID, &item.InstanceID, &event, &target, &item.URL, &body, &headers,
			&status, &item.Attempts, &responseStatus, &responseBody, &lastError,
			&item.CreatedAt, &item.UpdatedAt, &deliveredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery{}, errWebhookDeliveryNotFound
	}
	if err != nil {
		return delivery{}, fmt.Errorf("find webhook delivery: %w", err)
	}
	if err := json.Unmarshal([]byte(headers), &item.Headers); err != nil {
		return delivery{}, fmt.Errorf("decode webhook delivery headers: %w", err)
	}
	item.Event = types.WebhookEvent(event)
	item.Target = deliveryTarget(target)
	item.Status = deliveryStatus(status)
	item.Body = []byte(body)
	item.ResponseStatus = nullIntPointer(responseStatus)
	item.ResponseBody = nullStringPointer(responseBody)
	item.LastError = nullStringPointer(lastError)
	item.DeliveredAt = nullTimePointer(deliveredAt)
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func (r *sqlDeliveryRepository) MarkAttempt(ctx context.Context, grant security.Grant, id string, attempts int) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE whatsapp_webhook_deliveries
		SET status = ?, attempts = CASE WHEN attempts > ? THEN attempts ELSE ? END, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND status <> ?`, string(deliveryStatusProcessing),
		attempts, attempts, time.Now().UTC(), tenant, id, string(deliveryStatusDelivered))
	if err != nil {
		return fmt.Errorf("mark webhook delivery attempt: %w", err)
	}
	return requireDeliveryRow(result, "mark webhook delivery attempt")
}

func (r *sqlDeliveryRepository) MarkDelivered(ctx context.Context, grant security.Grant, id string, attempts, responseStatus int) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE whatsapp_webhook_deliveries
		SET status = ?, attempts = CASE WHEN attempts > ? THEN attempts ELSE ? END,
			response_status = ?, response_body = NULL, last_error = NULL,
			url = '', body = '', headers = '{}', updated_at = ?, delivered_at = ?
		WHERE tenant_id = ? AND id = ?`, string(deliveryStatusDelivered), attempts,
		attempts, responseStatus, now, now, tenant, id)
	if err != nil {
		return fmt.Errorf("mark webhook delivery delivered: %w", err)
	}
	return requireDeliveryRow(result, "mark webhook delivery delivered")
}

func (r *sqlDeliveryRepository) PruneBefore(ctx context.Context, grant security.Grant, cutoff time.Time) (int64, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return 0, err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM whatsapp_webhook_deliveries
		WHERE tenant_id = ? AND created_at < ?`, tenant, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("prune expired webhook deliveries: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned webhook deliveries: %w", err)
	}
	return deleted, nil
}

func (r *sqlDeliveryRepository) MarkFailed(ctx context.Context, grant security.Grant, id string, attempts, responseStatus int, lastError string) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE whatsapp_webhook_deliveries
		SET status = ?, attempts = CASE WHEN attempts > ? THEN attempts ELSE ? END,
			response_status = ?, response_body = NULL, last_error = ?,
			updated_at = ?
		WHERE tenant_id = ? AND id = ? AND status <> ?`, string(deliveryStatusFailed), attempts,
		attempts, nullableStatus(responseStatus), lastError, time.Now().UTC(), tenant, id,
		string(deliveryStatusDelivered))
	if err != nil {
		return fmt.Errorf("mark webhook delivery failed: %w", err)
	}
	return requireDeliveryRow(result, "mark webhook delivery failed")
}

func deliveryTenant(grant security.Grant) (string, error) {
	tenant := data.Tenant(grant)
	if tenant == "" {
		return "", fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	return tenant, nil
}

func requireDeliveryRow(result sql.Result, operation string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if rows == 0 {
		return errWebhookDeliveryNotFound
	}
	return nil
}

func nullableStatus(status int) any {
	if status == 0 {
		return nil
	}
	return status
}

func nullIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	converted := value.Time.UTC()
	return &converted
}
