package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

type SQLMessageUpdateRepository struct{ base *Base }

func NewMessageUpdateRepository(base *Base) *SQLMessageUpdateRepository {
	return &SQLMessageUpdateRepository{base: base}
}

var _ MessageUpdateRepository = (*SQLMessageUpdateRepository)(nil)

func (r *SQLMessageUpdateRepository) Create(ctx context.Context, grant security.Grant, input types.CreateMessageUpdateInput) (types.MessageUpdate, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return types.MessageUpdate{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.MessageUpdate{}, err
	}
	return r.create(ctx, tenant, input, false)
}

func (r *SQLMessageUpdateRepository) CreateOrIgnore(ctx context.Context, grant security.Grant, input types.CreateMessageUpdateInput) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	_, err = r.create(ctx, tenant, input, true)
	return err
}

func (r *SQLMessageUpdateRepository) create(ctx context.Context, tenant string, input types.CreateMessageUpdateInput, ignore bool) (types.MessageUpdate, error) {
	var instanceID int64
	err := r.base.db.QueryRowContext(ctx, `SELECT instance_id FROM whatsapp_messages
		WHERE tenant_id = ? AND id = ?`, tenant, input.MessageID).Scan(&instanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.MessageUpdate{}, ErrMessageNotFound
	}
	if err != nil {
		return types.MessageUpdate{}, err
	}
	id, err := newID()
	if err != nil {
		return types.MessageUpdate{}, err
	}
	query := `INSERT INTO whatsapp_message_updates (
		id, tenant_id, instance_id, message_id, date_time, status
	) VALUES (?, ?, ?, ?, ?, ?)`
	if ignore {
		query += ` ON CONFLICT (tenant_id, instance_id, message_id, status, date_time) DO NOTHING`
	}
	result, err := r.base.db.ExecContext(ctx, query,
		id, tenant, instanceID, input.MessageID, input.DateTime.UTC(), input.Status)
	if err != nil {
		return types.MessageUpdate{}, fmt.Errorf("create whatsapp message update: %w", err)
	}
	if ignore && affected(result) == 0 {
		return types.MessageUpdate{}, nil
	}
	return types.MessageUpdate{ID: id, DateTime: input.DateTime.UTC(), Status: input.Status, MessageID: input.MessageID}, nil
}

func (r *SQLMessageUpdateRepository) ListByMessageID(ctx context.Context, grant security.Grant, messageID int64) ([]types.MessageUpdate, error) {
	if err := grant.Check(authz.ActionMessageList); err != nil {
		return nil, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return nil, err
	}
	rows, err := r.base.db.QueryContext(ctx, `SELECT id, date_time, status, message_id
		FROM whatsapp_message_updates WHERE tenant_id = ? AND message_id = ?
		ORDER BY date_time, id`, tenant, messageID)
	if err != nil {
		return nil, fmt.Errorf("list whatsapp message updates: %w", err)
	}
	defer rows.Close()
	items := make([]types.MessageUpdate, 0)
	for rows.Next() {
		var item types.MessageUpdate
		if err := rows.Scan(&item.ID, &item.DateTime, &item.Status, &item.MessageID); err != nil {
			return nil, err
		}
		item.DateTime = item.DateTime.UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}
