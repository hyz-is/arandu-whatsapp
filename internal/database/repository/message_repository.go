package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const messageColumns = `m.id, m.key_id, m.key_remote_jid, m.key_lid, m.key_from_me,
	m.key_participant, m.key_participant_lid, m.push_name, m.message_type, m.content,
	m.message_timestamp, m.device, m.is_group, m.instance_id, m.metadata`

type SQLMessageRepository struct{ base *Base }

func NewMessageRepository(base *Base) *SQLMessageRepository {
	return &SQLMessageRepository{base: base}
}

var _ MessageRepository = (*SQLMessageRepository)(nil)

func (r *SQLMessageRepository) Create(ctx context.Context, grant security.Grant, input types.CreateMessageInput) (types.Message, error) {
	if err := authz.CheckMessageCreate(grant); err != nil {
		return types.Message{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Message{}, err
	}
	if err := r.validateInput(ctx, tenant, input); err != nil {
		return types.Message{}, err
	}
	id, err := newID()
	if err != nil {
		return types.Message{}, err
	}
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_messages (
		id, tenant_id, instance_id, key_id, key_remote_jid, key_lid, key_from_me,
		key_participant, key_participant_lid, push_name, message_type, content,
		message_timestamp, device, is_group, metadata, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tenant, input.InstanceID, input.KeyID, input.KeyRemoteJid, input.KeyLid,
		input.KeyFromMe, input.KeyParticipant, input.KeyParticipantLid, input.PushName,
		input.MessageType, jsonText(input.Content, "null"), input.MessageTimestamp,
		input.Device, input.IsGroup, jsonText(input.Metadata, "null"), time.Now().UTC())
	if err != nil {
		return types.Message{}, fmt.Errorf("create whatsapp message: %w", err)
	}
	return r.find(ctx, tenant, input.InstanceID, "m.id = ?", id)
}

func (r *SQLMessageRepository) CreateOrIgnore(ctx context.Context, grant security.Grant, input types.CreateMessageInput) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	if err := r.validateInput(ctx, tenant, input); err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return err
	}
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_messages (
		id, tenant_id, instance_id, key_id, key_remote_jid, key_lid, key_from_me,
		key_participant, key_participant_lid, push_name, message_type, content,
		message_timestamp, device, is_group, metadata, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (tenant_id, instance_id, key_id) DO NOTHING`,
		id, tenant, input.InstanceID, input.KeyID, input.KeyRemoteJid, input.KeyLid,
		input.KeyFromMe, input.KeyParticipant, input.KeyParticipantLid, input.PushName,
		input.MessageType, jsonText(input.Content, "null"), input.MessageTimestamp,
		input.Device, input.IsGroup, jsonText(input.Metadata, "null"), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("create or ignore whatsapp message: %w", err)
	}
	return nil
}

func (r *SQLMessageRepository) FindByIDForInstance(ctx context.Context, grant security.Grant, instanceID, id int64) (types.Message, error) {
	if err := authz.CheckMessageByID(grant); err != nil {
		return types.Message{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Message{}, err
	}
	return r.find(ctx, tenant, instanceID, "m.id = ?", id)
}

func (r *SQLMessageRepository) FindByKeyIDForInstance(ctx context.Context, grant security.Grant, instanceID int64, keyID string) (types.Message, error) {
	if err := authz.CheckMessageByKey(grant); err != nil {
		return types.Message{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Message{}, err
	}
	return r.find(ctx, tenant, instanceID, "m.key_id = ?", keyID)
}

func (r *SQLMessageRepository) FindByIDsForInstance(ctx context.Context, grant security.Grant, instanceID int64, ids []int64) ([]types.Message, error) {
	if err := grant.Check(authz.ActionMessageRead); err != nil {
		return nil, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return nil, err
	}
	return r.findByIDs(ctx, tenant, instanceID, ids)
}

func (r *SQLMessageRepository) findByIDs(ctx context.Context, tenant string, instanceID int64, ids []int64) ([]types.Message, error) {
	if len(ids) == 0 {
		return []types.Message{}, nil
	}
	args := []any{tenant, instanceID}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.base.db.QueryContext(ctx, `SELECT `+messageColumns+`
		FROM whatsapp_messages m WHERE m.tenant_id = ? AND m.instance_id = ?
		AND m.id IN (`+placeholders(len(ids))+`) ORDER BY m.created_at, m.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("find whatsapp messages: %w", err)
	}
	defer rows.Close()
	items := make([]types.Message, 0, len(ids))
	for rows.Next() {
		item, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLMessageRepository) FindOutgoingByIDForInstance(ctx context.Context, grant security.Grant, instanceID, id int64) (types.Message, error) {
	if err := grant.Check(authz.ActionMessageEdit); err != nil {
		return types.Message{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Message{}, err
	}
	return r.find(ctx, tenant, instanceID, "m.id = ? AND m.key_from_me = ?", id, true)
}

func (r *SQLMessageRepository) FindOutgoingByKeyIDForInstance(ctx context.Context, grant security.Grant, instanceID int64, keyID string) (types.Message, error) {
	if err := grant.Check(authz.ActionMessageEdit); err != nil {
		return types.Message{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Message{}, err
	}
	return r.find(ctx, tenant, instanceID, "m.key_id = ? AND m.key_from_me = ?", keyID, true)
}

func (r *SQLMessageRepository) MarkReadForInstance(ctx context.Context, grant security.Grant, instanceID int64, ids []int64) error {
	if err := grant.Check(authz.ActionMessageRead); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return data.Transaction(ctx, r.base.db, func(txCtx context.Context) error {
		items, findErr := r.findByIDs(txCtx, tenant, instanceID, ids)
		if findErr != nil {
			return findErr
		}
		if len(items) != len(ids) {
			return ErrMessageNotFound
		}
		now := time.Now().UTC()
		for _, item := range items {
			id, idErr := newID()
			if idErr != nil {
				return idErr
			}
			_, execErr := r.base.db.ExecContext(txCtx, `INSERT INTO whatsapp_message_updates (
				id, tenant_id, instance_id, message_id, date_time, status
			) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id, instance_id, message_id, status, date_time) DO NOTHING`,
				id, tenant, instanceID, item.ID, now, "read")
			if execErr != nil {
				return execErr
			}
		}
		return nil
	})
}

func (r *SQLMessageRepository) UpdateContentForInstance(ctx context.Context, grant security.Grant, instanceID, id int64, content json.RawMessage) (types.Message, error) {
	if err := authz.CheckMessageContentUpdate(grant); err != nil {
		return types.Message{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Message{}, err
	}
	if !json.Valid(content) {
		return types.Message{}, ErrInvalidJSON
	}
	result, err := r.base.db.ExecContext(ctx, `UPDATE whatsapp_messages SET content = ?
		WHERE tenant_id = ? AND instance_id = ? AND id = ?`, string(content), tenant, instanceID, id)
	if err != nil {
		return types.Message{}, fmt.Errorf("update whatsapp message: %w", err)
	}
	if affected(result) == 0 {
		return types.Message{}, ErrMessageNotFound
	}
	return r.find(ctx, tenant, instanceID, "m.id = ?", id)
}

func (r *SQLMessageRepository) Count(ctx context.Context, grant security.Grant, instanceID int64, filters types.MessageFilters) (int64, error) {
	if err := grant.Check(authz.ActionMessageList); err != nil {
		return 0, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return 0, err
	}
	return r.count(ctx, tenant, instanceID, filters)
}

func (r *SQLMessageRepository) count(ctx context.Context, tenant string, instanceID int64, filters types.MessageFilters) (int64, error) {
	where, args := messageFilterSQL(tenant, instanceID, filters)
	var count int64
	err := r.base.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM whatsapp_messages m WHERE `+where, args...).Scan(&count)
	return count, err
}

func (r *SQLMessageRepository) List(ctx context.Context, grant security.Grant, instanceID int64, input types.ListMessagesInput) (types.MessageListResult, error) {
	if err := grant.Check(authz.ActionMessageList); err != nil {
		return types.MessageListResult{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.MessageListResult{}, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	where, args := messageFilterSQL(tenant, instanceID, input.Filters)
	order := "ASC"
	if input.Direction == types.CursorDirectionPrevious {
		order = "DESC"
	}
	if input.Cursor != nil {
		operator := ">"
		if order == "DESC" {
			operator = "<"
		}
		where += ` AND (m.created_at ` + operator + ` (SELECT created_at FROM whatsapp_messages WHERE tenant_id = ? AND instance_id = ? AND id = ?)
			OR (m.created_at = (SELECT created_at FROM whatsapp_messages WHERE tenant_id = ? AND instance_id = ? AND id = ?) AND m.id ` + operator + ` ?))`
		args = append(args, tenant, instanceID, *input.Cursor, tenant, instanceID, *input.Cursor, *input.Cursor)
	}
	query := `SELECT ` + messageColumns + ` FROM whatsapp_messages m WHERE ` + where +
		` ORDER BY m.created_at ` + order + `, m.id ` + order + ` LIMIT ?`
	args = append(args, limit)
	items, err := r.listRows(ctx, tenant, query, args...)
	if err != nil {
		return types.MessageListResult{}, err
	}
	if order == "DESC" {
		for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
			items[left], items[right] = items[right], items[left]
		}
	}
	total, err := r.count(ctx, tenant, instanceID, input.Filters)
	if err != nil {
		return types.MessageListResult{}, err
	}
	return messageListResult(items, total, int64(limit), 1), nil
}

func (r *SQLMessageRepository) ListPage(ctx context.Context, grant security.Grant, instanceID int64, input types.ListMessagesPageInput) (types.MessageListResult, error) {
	if err := grant.Check(authz.ActionMessageList); err != nil {
		return types.MessageListResult{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.MessageListResult{}, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	page := input.Page
	if page <= 0 {
		page = 1
	}
	where, args := messageFilterSQL(tenant, instanceID, input.Filters)
	query := `SELECT ` + messageColumns + ` FROM whatsapp_messages m WHERE ` + where +
		` ORDER BY m.created_at DESC, m.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, (page-1)*limit)
	items, err := r.listRows(ctx, tenant, query, args...)
	if err != nil {
		return types.MessageListResult{}, err
	}
	total, err := r.count(ctx, tenant, instanceID, input.Filters)
	if err != nil {
		return types.MessageListResult{}, err
	}
	return messageListResult(items, total, int64(limit), int64(page)), nil
}

func (r *SQLMessageRepository) validateInput(ctx context.Context, tenant string, input types.CreateMessageInput) error {
	exists, err := r.base.instanceExists(ctx, tenant, input.InstanceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInstanceNotFound
	}
	if strings.TrimSpace(input.KeyID) == "" || strings.TrimSpace(input.MessageType) == "" {
		return ErrInvalidInput
	}
	if !input.Device.IsValid() {
		return ErrInvalidEnum
	}
	if len(input.Content) > 0 && !json.Valid(input.Content) {
		return ErrInvalidJSON
	}
	if len(input.Metadata) > 0 && !json.Valid(input.Metadata) {
		return ErrInvalidJSON
	}
	return nil
}

func (r *SQLMessageRepository) find(ctx context.Context, tenant string, instanceID int64, predicate string, values ...any) (types.Message, error) {
	args := []any{tenant, instanceID}
	args = append(args, values...)
	row := r.base.db.QueryRowContext(ctx, `SELECT `+messageColumns+`
		FROM whatsapp_messages m WHERE m.tenant_id = ? AND m.instance_id = ? AND `+predicate, args...)
	return scanMessage(row)
}

func (r *SQLMessageRepository) listRows(ctx context.Context, tenant, query string, args ...any) ([]types.MessageWithUpdates, error) {
	rows, err := r.base.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list whatsapp messages: %w", err)
	}
	defer rows.Close()
	messages := make([]types.Message, 0)
	for rows.Next() {
		item, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]types.MessageWithUpdates, 0, len(messages))
	for _, message := range messages {
		updates, updateErr := r.updateSummaries(ctx, tenant, message.InstanceID, message.ID)
		if updateErr != nil {
			return nil, updateErr
		}
		items = append(items, types.MessageWithUpdates{Message: message, MessageUpdate: updates})
	}
	return items, nil
}

func (r *SQLMessageRepository) updateSummaries(ctx context.Context, tenant string, instanceID, messageID int64) ([]types.MessageUpdateSummary, error) {
	rows, err := r.base.db.QueryContext(ctx, `SELECT status, date_time FROM whatsapp_message_updates
		WHERE tenant_id = ? AND instance_id = ? AND message_id = ? ORDER BY date_time, id`,
		tenant, instanceID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]types.MessageUpdateSummary, 0)
	for rows.Next() {
		var item types.MessageUpdateSummary
		if err := rows.Scan(&item.Status, &item.DateTime); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func messageFilterSQL(tenant string, instanceID int64, filters types.MessageFilters) (string, []any) {
	parts := []string{"m.tenant_id = ?", "m.instance_id = ?"}
	args := []any{tenant, instanceID}
	appendFilter := func(condition string, value any) {
		parts = append(parts, condition)
		args = append(args, value)
	}
	if filters.ID != nil {
		appendFilter("m.id = ?", *filters.ID)
	}
	if filters.KeyID != nil {
		appendFilter("m.key_id = ?", *filters.KeyID)
	}
	if filters.KeyRemoteJid != nil {
		appendFilter("m.key_remote_jid = ?", *filters.KeyRemoteJid)
	}
	if filters.KeyFromMe != nil {
		appendFilter("m.key_from_me = ?", *filters.KeyFromMe)
	}
	if filters.MessageType != nil {
		appendFilter("m.message_type = ?", *filters.MessageType)
	}
	if filters.Device != nil {
		appendFilter("m.device = ?", *filters.Device)
	}
	if filters.MessageTimestampGTE != nil {
		appendFilter("m.message_timestamp >= ?", *filters.MessageTimestampGTE)
	}
	if filters.MessageTimestampLTE != nil {
		appendFilter("m.message_timestamp <= ?", *filters.MessageTimestampLTE)
	}
	if filters.MessageStatus != nil {
		parts = append(parts, `EXISTS (SELECT 1 FROM whatsapp_message_updates u
			WHERE u.tenant_id = m.tenant_id AND u.instance_id = m.instance_id
			AND u.message_id = m.id AND u.status = ?)`)
		args = append(args, *filters.MessageStatus)
	}
	return strings.Join(parts, " AND "), args
}

func messageListResult(items []types.MessageWithUpdates, total, limit, page int64) types.MessageListResult {
	pages := int64(0)
	if limit > 0 {
		pages = int64(math.Ceil(float64(total) / float64(limit)))
	}
	return types.MessageListResult{Messages: types.MessagePage{
		Total: total, Pages: pages, CurrentPage: page, Records: items,
	}}
}

func scanMessage(row rowScanner) (types.Message, error) {
	var item types.Message
	var remoteJID, lid, participant, participantLID, pushName sql.NullString
	var isGroup sql.NullBool
	var content, metadata string
	var device string
	err := row.Scan(&item.ID, &item.KeyID, &remoteJID, &lid, &item.KeyFromMe,
		&participant, &participantLID, &pushName, &item.MessageType, &content,
		&item.MessageTimestamp, &device, &isGroup, &item.InstanceID, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Message{}, ErrMessageNotFound
	}
	if err != nil {
		return types.Message{}, fmt.Errorf("scan whatsapp message: %w", err)
	}
	item.KeyRemoteJid = stringPointer(remoteJID)
	item.KeyLid = stringPointer(lid)
	item.KeyParticipant = stringPointer(participant)
	item.KeyParticipantLid = stringPointer(participantLID)
	item.PushName = stringPointer(pushName)
	item.IsGroup = boolPointer(isGroup)
	item.Content = json.RawMessage(content)
	item.Metadata = json.RawMessage(metadata)
	item.Device = types.DeviceMessage(device)
	return item, nil
}
