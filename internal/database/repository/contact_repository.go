package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

type SQLContactRepository struct{ base *Base }

func NewContactRepository(base *Base) *SQLContactRepository {
	return &SQLContactRepository{base: base}
}

var _ ContactRepository = (*SQLContactRepository)(nil)

func (r *SQLContactRepository) Create(ctx context.Context, grant security.Grant, input types.CreateContactInput) (types.Contact, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return types.Contact{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Contact{}, err
	}
	exists, err := r.base.instanceExists(ctx, tenant, input.InstanceID)
	if err != nil {
		return types.Contact{}, err
	}
	if !exists {
		return types.Contact{}, ErrInstanceNotFound
	}
	id, err := newID()
	if err != nil {
		return types.Contact{}, err
	}
	now := time.Now().UTC()
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_contacts (
		id, tenant_id, instance_id, remote_jid, push_name, profile_pic_url, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tenant, input.InstanceID, strings.TrimSpace(input.RemoteJid), input.PushName,
		input.ProfilePicUrl, now, now)
	if err != nil {
		return types.Contact{}, fmt.Errorf("create whatsapp contact: %w", err)
	}
	return r.find(ctx, tenant, input.InstanceID, id)
}

func (r *SQLContactRepository) Upsert(ctx context.Context, grant security.Grant, input types.CreateContactInput) (types.Contact, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return types.Contact{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.Contact{}, err
	}
	exists, err := r.base.instanceExists(ctx, tenant, input.InstanceID)
	if err != nil {
		return types.Contact{}, err
	}
	if !exists {
		return types.Contact{}, ErrInstanceNotFound
	}
	id, err := newID()
	if err != nil {
		return types.Contact{}, err
	}
	now := time.Now().UTC()
	remote := strings.TrimSpace(input.RemoteJid)
	if remote == "" {
		return types.Contact{}, ErrInvalidInput
	}
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_contacts (
		id, tenant_id, instance_id, remote_jid, push_name, profile_pic_url, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (tenant_id, instance_id, remote_jid) DO UPDATE SET
		push_name = CASE WHEN excluded.push_name IS NULL OR excluded.push_name = ''
			THEN whatsapp_contacts.push_name ELSE excluded.push_name END,
		profile_pic_url = CASE WHEN excluded.profile_pic_url IS NULL OR excluded.profile_pic_url = ''
			THEN whatsapp_contacts.profile_pic_url ELSE excluded.profile_pic_url END,
		updated_at = excluded.updated_at`,
		id, tenant, input.InstanceID, remote, input.PushName, input.ProfilePicUrl, now, now)
	if err != nil {
		return types.Contact{}, fmt.Errorf("upsert whatsapp contact: %w", err)
	}
	row := r.base.db.QueryRowContext(ctx, `SELECT id, remote_jid, push_name, profile_pic_url,
		created_at, updated_at, instance_id FROM whatsapp_contacts
		WHERE tenant_id = ? AND instance_id = ? AND remote_jid = ?`, tenant, input.InstanceID, remote)
	return scanContact(row)
}

func (r *SQLContactRepository) List(ctx context.Context, grant security.Grant, instanceID int64, filters types.ContactFilters) ([]types.Contact, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return nil, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return nil, err
	}
	query := `SELECT id, remote_jid, push_name, profile_pic_url, created_at, updated_at, instance_id
		FROM whatsapp_contacts WHERE tenant_id = ? AND instance_id = ?`
	args := []any{tenant, instanceID}
	if filters.ID != nil {
		query += ` AND id = ?`
		args = append(args, *filters.ID)
	} else {
		if filters.RemoteJid != nil {
			query += ` AND remote_jid = ?`
			args = append(args, *filters.RemoteJid)
		}
		if filters.PushName != nil {
			query += ` AND LOWER(push_name) LIKE ?`
			args = append(args, "%"+strings.ToLower(*filters.PushName)+"%")
		}
	}
	query += ` ORDER BY created_at, id`
	rows, err := r.base.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list whatsapp contacts: %w", err)
	}
	defer rows.Close()
	items := make([]types.Contact, 0)
	for rows.Next() {
		item, scanErr := scanContact(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLContactRepository) find(ctx context.Context, tenant string, instanceID, id int64) (types.Contact, error) {
	row := r.base.db.QueryRowContext(ctx, `SELECT id, remote_jid, push_name, profile_pic_url,
		created_at, updated_at, instance_id FROM whatsapp_contacts
		WHERE tenant_id = ? AND instance_id = ? AND id = ?`, tenant, instanceID, id)
	return scanContact(row)
}

func scanContact(row rowScanner) (types.Contact, error) {
	var item types.Contact
	var pushName, profilePicURL sql.NullString
	err := row.Scan(&item.ID, &item.RemoteJid, &pushName, &profilePicURL,
		&item.CreatedAt, &item.UpdatedAt, &item.InstanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Contact{}, ErrInstanceNotFound
	}
	if err != nil {
		return types.Contact{}, fmt.Errorf("scan whatsapp contact: %w", err)
	}
	item.PushName = stringPointer(pushName)
	item.ProfilePicUrl = stringPointer(profilePicURL)
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}
