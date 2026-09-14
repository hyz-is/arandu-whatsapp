package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/query"
	hesapewebhook "github.com/arandu-io/hesape/webhook"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

var errWebhookConfigurationNotFound = errors.New("webhook configuration not found")

type configurationRepository interface {
	FindConfiguration(context.Context, security.Grant, int64) (types.Webhook, error)
}

// sqlDeliveryStore adapts the generic state machine to the existing table.
type sqlDeliveryStore struct{ db *data.DB }

func newSQLDeliveryStore(db *data.DB) *sqlDeliveryStore { return &sqlDeliveryStore{db: db} }

func (s *sqlDeliveryStore) FindConfiguration(ctx context.Context, grant security.Grant, instanceID int64) (types.Webhook, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return types.Webhook{}, err
	}
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return types.Webhook{}, err
	}
	var item types.Webhook
	var events string
	err = s.db.QueryRowContext(ctx, `SELECT id, url, enabled, events, created_at, updated_at, instance_id
		FROM whatsapp_webhooks WHERE tenant_id = ? AND instance_id = ?`, tenant, instanceID).
		Scan(&item.ID, &item.URL, &item.Enabled, &events, &item.CreatedAt, &item.UpdatedAt, &item.InstanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Webhook{}, errWebhookConfigurationNotFound
	}
	if err != nil {
		return types.Webhook{}, fmt.Errorf("find webhook configuration for delivery: %w", err)
	}
	item.Events = json.RawMessage(events)
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	return item, nil
}

func (s *sqlDeliveryStore) Transaction(ctx context.Context, fn func(context.Context) error) error {
	return data.Transaction(ctx, s.db, fn)
}

func (s *sqlDeliveryStore) Create(ctx context.Context, grant security.Grant, item hesapewebhook.Delivery) (bool, error) {
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return false, err
	}
	target, instanceID, err := parseDeliveryEndpointID(item.EndpointID)
	if err != nil {
		return false, err
	}
	headers := make(map[string]string, len(item.Headers)+1)
	for name, value := range item.Headers {
		headers[name] = value
	}
	headers["X-Arandu-Delivery-ID"] = item.ID
	encodedHeaders, err := json.Marshal(headers)
	if err != nil {
		return false, fmt.Errorf("encode webhook delivery headers: %w", err)
	}
	created, err := query.NewBuilder(s.db, s.db.GetQueryGrammar(), s.db.GetPostProcessor()).From("whatsapp_webhook_deliveries").InsertOrIgnore(ctx, grant, map[string]any{
		"id": item.ID, "tenant_id": tenant, "instance_id": instanceID, "event": item.EventName,
		"target": target, "url": item.URL, "body": string(item.Body), "headers": string(encodedHeaders),
		"status": string(hesapewebhook.StatusPending), "attempts": 0, "response_body": nil,
		"event_id": item.EventID, "endpoint_id": item.EndpointID, "secret_ref": item.SecretRef,
		"permanent": false, "claim_version": 0, "created_at": item.CreatedAt.UTC(), "updated_at": item.UpdatedAt.UTC(),
	})
	if err != nil {
		return false, fmt.Errorf("create webhook delivery: %w", err)
	}
	return created == 1, nil
}

func (s *sqlDeliveryStore) Find(ctx context.Context, grant security.Grant, id string) (hesapewebhook.Delivery, error) {
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return hesapewebhook.Delivery{}, err
	}
	var item hesapewebhook.Delivery
	var body, headers, eventName, target, status string
	var instanceID int64
	var eventID, endpointID, secretRef sql.NullString
	var responseStatus sql.NullInt64
	var lastError sql.NullString
	var leaseUntil, deliveredAt sql.NullTime
	err = s.db.QueryRowContext(ctx, `SELECT id, instance_id, event, target, url, body, headers,
		status, attempts, response_status, last_error, created_at, updated_at, delivered_at,
		event_id, endpoint_id, secret_ref, permanent, lease_until
		FROM whatsapp_webhook_deliveries WHERE tenant_id = ? AND id = ?`, tenant, id).Scan(
		&item.ID, &instanceID, &eventName, &target, &item.URL, &body, &headers, &status, &item.Attempts,
		&responseStatus, &lastError, &item.CreatedAt, &item.UpdatedAt, &deliveredAt,
		&eventID, &endpointID, &secretRef, &item.Permanent, &leaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return hesapewebhook.Delivery{}, hesapewebhook.ErrDeliveryNotFound
	}
	if err != nil {
		return hesapewebhook.Delivery{}, fmt.Errorf("find webhook delivery: %w", err)
	}
	if err := json.Unmarshal([]byte(headers), &item.Headers); err != nil {
		return hesapewebhook.Delivery{}, fmt.Errorf("decode webhook delivery headers: %w", err)
	}
	item.TenantID, item.EventID, item.EventName = tenant, eventID.String, eventName
	if item.EventID == "" {
		item.EventID = item.ID
	}
	item.EndpointID = endpointID.String
	if item.EndpointID == "" {
		item.EndpointID = deliveryEndpointID(target, instanceID)
	}
	item.SecretRef, item.Body, item.Status = secretRef.String, []byte(body), hesapewebhook.Status(status)
	if responseStatus.Valid {
		item.ResponseStatus = int(responseStatus.Int64)
	}
	if lastError.Valid {
		item.LastError = lastError.String
	}
	if leaseUntil.Valid {
		item.LeaseUntil = leaseUntil.Time.UTC()
	}
	if deliveredAt.Valid {
		item.DeliveredAt = deliveredAt.Time.UTC()
	}
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	return item, nil
}

func (s *sqlDeliveryStore) Claim(ctx context.Context, grant security.Grant, id string, lease time.Duration) (hesapewebhook.Claim, bool, error) {
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return hesapewebhook.Claim{}, false, err
	}
	token, err := data.NewID()
	if err != nil {
		return hesapewebhook.Claim{}, false, err
	}
	now := time.Now().UTC()
	leaseEnd := now.Add(lease)
	result, err := s.db.ExecContext(ctx, `UPDATE whatsapp_webhook_deliveries SET status = ?, claim_token = ?,
		claim_version = claim_version + 1, lease_until = ?, attempts = attempts + 1, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND status <> ? AND permanent = ?
		AND (lease_until IS NULL OR lease_until <= ?)`, string(hesapewebhook.StatusProcessing), token, leaseEnd,
		now, tenant, id, string(hesapewebhook.StatusDelivered), false, now)
	if err != nil {
		return hesapewebhook.Claim{}, false, fmt.Errorf("claim webhook delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return hesapewebhook.Claim{}, false, err
	}
	if rows == 0 {
		item, findErr := s.Find(ctx, grant, id)
		if findErr != nil {
			return hesapewebhook.Claim{}, false, findErr
		}
		if item.Status == hesapewebhook.StatusDelivered || item.Permanent {
			return hesapewebhook.Claim{}, false, hesapewebhook.ErrDeliveryNotFound
		}
		return hesapewebhook.Claim{}, false, nil
	}
	item, err := s.Find(ctx, grant, id)
	if err != nil {
		return hesapewebhook.Claim{}, false, err
	}
	var version int64
	err = s.db.QueryRowContext(ctx, `SELECT claim_version FROM whatsapp_webhook_deliveries WHERE tenant_id = ? AND id = ? AND claim_token = ?`, tenant, id, token).Scan(&version)
	if err != nil {
		return hesapewebhook.Claim{}, false, fmt.Errorf("read webhook delivery claim fence: %w", err)
	}
	return hesapewebhook.Claim{Delivery: item, Token: token, Version: version, LeaseEnd: leaseEnd}, true, nil
}

func (s *sqlDeliveryStore) Complete(ctx context.Context, grant security.Grant, claim hesapewebhook.Claim, result hesapewebhook.Result) error {
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return err
	}
	finished := result.FinishedAt.UTC()
	if finished.IsZero() {
		finished = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `UPDATE whatsapp_webhook_deliveries SET status = ?, response_status = ?,
		response_body = NULL, last_error = NULL, claim_token = NULL, lease_until = NULL,
		url = '', body = '', headers = '{}', updated_at = ?, delivered_at = ?
		WHERE tenant_id = ? AND id = ? AND claim_token = ? AND claim_version = ?`, string(hesapewebhook.StatusDelivered),
		result.StatusCode, finished, finished, tenant, claim.Delivery.ID, claim.Token, claim.Version)
	if err != nil {
		return fmt.Errorf("complete webhook delivery: %w", err)
	}
	return s.requireCurrentClaim(ctx, grant, claim.Delivery.ID, res)
}

func (s *sqlDeliveryStore) Fail(ctx context.Context, grant security.Grant, claim hesapewebhook.Claim, failure hesapewebhook.Failure) error {
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE whatsapp_webhook_deliveries SET status = ?, response_status = ?,
		response_body = NULL, last_error = ?, permanent = ?, claim_token = NULL, lease_until = NULL, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND claim_token = ? AND claim_version = ?`, string(hesapewebhook.StatusFailed),
		nullableStatus(failure.StatusCode), failure.Code, failure.Permanent, time.Now().UTC(), tenant,
		claim.Delivery.ID, claim.Token, claim.Version)
	if err != nil {
		return fmt.Errorf("fail webhook delivery: %w", err)
	}
	return s.requireCurrentClaim(ctx, grant, claim.Delivery.ID, res)
}

func (s *sqlDeliveryStore) Prune(ctx context.Context, grant security.Grant, cutoff time.Time) (int64, error) {
	tenant, err := deliveryTenant(grant)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM whatsapp_webhook_deliveries WHERE tenant_id = ? AND created_at < ?`, tenant, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("prune expired webhook deliveries: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned webhook deliveries: %w", err)
	}
	return deleted, nil
}

func deliveryTenant(grant security.Grant) (string, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return "", err
	}
	tenant := data.Tenant(grant)
	if tenant == "" {
		return "", fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	return tenant, nil
}

func parseDeliveryEndpointID(endpointID string) (string, int64, error) {
	target, instance, found := strings.Cut(endpointID, ":")
	if !found || target != endpointTargetInstance && target != endpointTargetGlobal {
		return "", 0, fmt.Errorf("invalid WhatsApp webhook endpoint id %q", endpointID)
	}
	instanceID, err := strconv.ParseInt(instance, 10, 64)
	if err != nil || instanceID <= 0 {
		return "", 0, fmt.Errorf("invalid WhatsApp webhook endpoint id %q", endpointID)
	}
	return target, instanceID, nil
}

func (s *sqlDeliveryStore) requireCurrentClaim(ctx context.Context, grant security.Grant, id string, result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 0 {
		return nil
	}
	if _, err := s.Find(ctx, grant, id); errors.Is(err, hesapewebhook.ErrDeliveryNotFound) {
		return nil
	}
	return hesapewebhook.ErrStaleClaim
}

func nullableStatus(status int) any {
	if status == 0 {
		return nil
	}
	return status
}

var _ hesapewebhook.Store = (*sqlDeliveryStore)(nil)
