package repository

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

type SQLWebhookRepository struct{ base *Base }

func NewWebhookRepository(base *Base) *SQLWebhookRepository {
	return &SQLWebhookRepository{base: base}
}

var _ WebhookRepository = (*SQLWebhookRepository)(nil)

func (r *SQLWebhookRepository) Create(ctx context.Context, grant security.Grant, input types.CreateWebhookInput) (types.Webhook, error) {
	if err := grant.Check(authz.ActionWebhookSet); err != nil {
		return types.Webhook{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Webhook{}, err
	}
	if err := validateWebhookEventsJSON(input.Events); err != nil {
		return types.Webhook{}, err
	}
	exists, err := r.base.instanceExists(ctx, tenant, input.InstanceID)
	if err != nil {
		return types.Webhook{}, err
	}
	if !exists {
		return types.Webhook{}, ErrInstanceNotFound
	}
	id, err := newID()
	if err != nil {
		return types.Webhook{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC()
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_webhooks (
		id, tenant_id, instance_id, url, enabled, events, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, tenant, input.InstanceID, input.URL,
		enabled, jsonText(input.Events, "{}"), now, now)
	if err != nil {
		return types.Webhook{}, r.createError(ctx, tenant, input.InstanceID, err)
	}
	return r.findByID(ctx, tenant, id)
}

func (r *SQLWebhookRepository) createError(ctx context.Context, tenant string, instanceID int64, cause error) error {
	if looksUnique(cause) {
		duplicate, err := r.base.rowExists(ctx,
			`SELECT 1 FROM whatsapp_webhooks WHERE tenant_id = ? AND instance_id = ?`,
			tenant, instanceID)
		if err == nil && duplicate {
			return ErrWebhookAlreadyExists
		}
	}
	return fmt.Errorf("create whatsapp webhook: %w", cause)
}

func (r *SQLWebhookRepository) FindByInstanceName(ctx context.Context, grant security.Grant, instanceName string) (types.Webhook, error) {
	if err := authz.CheckWebhookLookup(grant); err != nil {
		return types.Webhook{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Webhook{}, err
	}
	row := r.base.db.QueryRowContext(ctx, `SELECT w.id, w.url, w.enabled, w.events,
		w.created_at, w.updated_at, w.instance_id
		FROM whatsapp_webhooks w
		JOIN whatsapp_instances i ON i.tenant_id = w.tenant_id AND i.id = w.instance_id
		WHERE w.tenant_id = ? AND i.name = ?`, tenant, instanceName)
	return scanWebhook(row)
}

func (r *SQLWebhookRepository) ListEnabledWithInstance(ctx context.Context, grant security.Grant) ([]types.WebhookWithInstance, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return nil, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return nil, err
	}
	rows, err := r.base.db.QueryContext(ctx, `SELECT w.id, w.url, w.enabled, w.events,
		w.created_at, w.updated_at, w.instance_id, i.name
		FROM whatsapp_webhooks w
		JOIN whatsapp_instances i ON i.tenant_id = w.tenant_id AND i.id = w.instance_id
		WHERE w.tenant_id = ? AND w.enabled = ? ORDER BY w.id`, tenant, true)
	if err != nil {
		return nil, fmt.Errorf("list enabled whatsapp webhooks: %w", err)
	}
	defer rows.Close()
	items := make([]types.WebhookWithInstance, 0)
	for rows.Next() {
		var item types.WebhookWithInstance
		if err := rows.Scan(&item.Webhook.ID, &item.Webhook.URL, &item.Webhook.Enabled,
			&item.Webhook.Events, &item.Webhook.CreatedAt, &item.Webhook.UpdatedAt,
			&item.Webhook.InstanceID, &item.InstanceName); err != nil {
			return nil, fmt.Errorf("scan enabled whatsapp webhook: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLWebhookRepository) Update(ctx context.Context, grant security.Grant, webhookID int64, input types.UpdateWebhookInput) (types.Webhook, error) {
	if err := grant.Check(authz.ActionWebhookSet); err != nil {
		return types.Webhook{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Webhook{}, err
	}
	query := `UPDATE whatsapp_webhooks SET updated_at = ?`
	args := []any{time.Now().UTC()}
	if input.URL != nil {
		query += `, url = ?`
		args = append(args, *input.URL)
	}
	if input.Enabled != nil {
		query += `, enabled = ?`
		args = append(args, *input.Enabled)
	}
	query += ` WHERE tenant_id = ? AND id = ?`
	args = append(args, tenant, webhookID)
	result, err := r.base.db.ExecContext(ctx, query, args...)
	if err != nil {
		return types.Webhook{}, fmt.Errorf("update whatsapp webhook: %w", err)
	}
	if affected(result) == 0 {
		return types.Webhook{}, ErrWebhookNotFound
	}
	return r.findByID(ctx, tenant, webhookID)
}

func (r *SQLWebhookRepository) UpsertEvents(ctx context.Context, grant security.Grant, webhookID int64, events map[string]bool) (types.Webhook, error) {
	if err := grant.Check(authz.ActionWebhookSet); err != nil {
		return types.Webhook{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Webhook{}, err
	}
	for event := range events {
		if !types.IsWebhookEventField(event) {
			return types.Webhook{}, fmt.Errorf("%w: %s", ErrInvalidWebhookEvent, event)
		}
	}
	err = data.Transaction(ctx, r.base.db, func(txCtx context.Context) error {
		current, findErr := r.findByID(txCtx, tenant, webhookID)
		if findErr != nil {
			return findErr
		}
		merged := map[string]bool{}
		if len(current.Events) > 0 && string(current.Events) != "null" {
			if unmarshalErr := json.Unmarshal(current.Events, &merged); unmarshalErr != nil {
				return fmt.Errorf("decode whatsapp webhook events: %w", unmarshalErr)
			}
		}
		if len(events) == 0 {
			merged = map[string]bool{}
		} else {
			for name, enabled := range events {
				merged[name] = enabled
			}
		}
		payload, marshalErr := json.Marshal(merged)
		if marshalErr != nil {
			return marshalErr
		}
		result, execErr := r.base.db.ExecContext(txCtx,
			`UPDATE whatsapp_webhooks SET events = ?, updated_at = ? WHERE tenant_id = ? AND id = ?`,
			string(payload), time.Now().UTC(), tenant, webhookID)
		if execErr != nil {
			return execErr
		}
		if affected(result) == 0 {
			return ErrWebhookNotFound
		}
		return nil
	})
	if err != nil {
		return types.Webhook{}, err
	}
	return r.findByID(ctx, tenant, webhookID)
}

func (r *SQLWebhookRepository) findByID(ctx context.Context, tenant string, id int64) (types.Webhook, error) {
	row := r.base.db.QueryRowContext(ctx, `SELECT id, url, enabled, events, created_at, updated_at, instance_id
		FROM whatsapp_webhooks WHERE tenant_id = ? AND id = ?`, tenant, id)
	return scanWebhook(row)
}

func scanWebhook(row rowScanner) (types.Webhook, error) {
	var item types.Webhook
	var events string
	err := row.Scan(&item.ID, &item.URL, &item.Enabled, &events,
		&item.CreatedAt, &item.UpdatedAt, &item.InstanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Webhook{}, ErrWebhookNotFound
	}
	if err != nil {
		return types.Webhook{}, fmt.Errorf("scan whatsapp webhook: %w", err)
	}
	item.Events = json.RawMessage(events)
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func validateWebhookEventsJSON(value json.RawMessage) error {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	if !json.Valid(value) {
		return ErrInvalidJSON
	}
	var events map[string]bool
	if err := json.Unmarshal(value, &events); err != nil {
		return ErrInvalidJSON
	}
	return types.ValidateWebhookEventFields(events)
}
