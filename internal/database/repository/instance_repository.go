package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/str"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const instanceSelectColumns = `
i.id, i.tenant_id, i.name, i.description, i.status, i.owner_jid,
	i.profile_pic_url, i.external_attributes, i.created_at, i.updated_at,
	COALESCE(c.connection_status, 'offline'), c.whatsapp_device_jid,
	c.whatsapp_owner_jid, c.whatsapp_phone_number, c.profile_pic_id,
	c.last_connected_at, c.last_disconnected_at, c.last_connection_attempt_at,
	c.last_connection_error, c.last_connection_event,
	COALESCE(c.connection_attempts, 0)`

// SQLInstanceRepository adapts the original WhatsApp domain to Arandu's
// instrumented database. Every exported persistence operation receives and
// checks its Grant before deriving tenant information.
type SQLInstanceRepository struct {
	base      *Base
	lockOwner string
}

func NewInstanceRepository(base *Base) *SQLInstanceRepository {
	return &SQLInstanceRepository{base: base, lockOwner: str.UUID()}
}

var _ InstanceRepository = (*SQLInstanceRepository)(nil)

func (r *SQLInstanceRepository) Create(ctx context.Context, grant security.Grant, input types.CreateInstanceInput) (types.InstanceRecord, error) {
	if err := grant.Check(authz.ActionInstanceCreate); err != nil {
		return types.InstanceRecord{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.InstanceRecord{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return types.InstanceRecord{}, ErrInvalidInput
	}
	status := types.InstanceStatusOnline
	if input.Status != nil {
		status = *input.Status
	}
	if !status.IsValid() {
		return types.InstanceRecord{}, ErrInvalidEnum
	}
	if len(input.ExternalAttributes) > 0 && !json.Valid(input.ExternalAttributes) {
		return types.InstanceRecord{}, ErrInvalidJSON
	}
	id, err := newID()
	if err != nil {
		return types.InstanceRecord{}, err
	}
	now := time.Now().UTC()
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_instances (
		id, tenant_id, name, description, status, owner_jid, profile_pic_url,
		external_attributes, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tenant, name, input.Description, status, input.OwnerJid,
		input.ProfilePicUrl, jsonText(input.ExternalAttributes, "{}"), now, now)
	if err != nil {
		return types.InstanceRecord{}, r.createError(ctx, tenant, name, err)
	}
	return r.findByName(ctx, tenant, name)
}

func (r *SQLInstanceRepository) createError(ctx context.Context, tenant, name string, cause error) error {
	if looksUnique(cause) {
		duplicate, err := r.base.rowExists(ctx,
			`SELECT 1 FROM whatsapp_instances WHERE tenant_id = ? AND name = ?`, tenant, name)
		if err == nil && duplicate {
			return fmt.Errorf("%w: %s", ErrInstanceNameAlreadyExists, name)
		}
	}
	return fmt.Errorf("create whatsapp instance: %w", cause)
}

func (r *SQLInstanceRepository) FindByName(ctx context.Context, grant security.Grant, name string) (types.InstanceRecord, error) {
	if err := authz.CheckInstanceLookup(grant); err != nil {
		return types.InstanceRecord{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.InstanceRecord{}, err
	}
	return r.findByName(ctx, tenant, name)
}

func (r *SQLInstanceRepository) findByName(ctx context.Context, tenant, name string) (types.InstanceRecord, error) {
	row := r.base.db.QueryRowContext(ctx, `SELECT `+instanceSelectColumns+`
		FROM whatsapp_instances i
		LEFT JOIN whatsapp_instance_connections c
		  ON c.tenant_id = i.tenant_id AND c.instance_id = i.id
		WHERE i.tenant_id = ? AND i.name = ?`, tenant, strings.TrimSpace(name))
	instance, err := scanInstance(row)
	if err != nil {
		return types.InstanceRecord{}, err
	}
	return types.InstanceRecord{Instance: instance}, nil
}

func (r *SQLInstanceRepository) FindByID(ctx context.Context, grant security.Grant, id int64) (types.InstanceRecord, error) {
	if err := grant.Check(authz.ActionInstanceView); err != nil {
		return types.InstanceRecord{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.InstanceRecord{}, err
	}
	return r.findByID(ctx, tenant, id)
}

// instancePageCursor stays private because the package receives no application
// signer through New(cfg, db, sessions). Constructing pagination.CursorSigner
// here would require inventing a second secret. The repository instead treats
// this value only as an opaque boundary and verifies its exact row against the
// Grant tenant and active name filter before using it in the keyset query.
type instancePageCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        int64     `json:"id"`
}

func (r *SQLInstanceRepository) ListPage(ctx context.Context, grant security.Grant, query data.Query, name *string) (database.Page[types.InstanceRecord], error) {
	if err := grant.Check(authz.ActionInstanceList); err != nil {
		return database.Page[types.InstanceRecord]{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return database.Page[types.InstanceRecord]{}, err
	}
	if query.Sort != "" {
		return database.Page[types.InstanceRecord]{}, ErrInvalidInput
	}
	limit := query.Limit
	if limit == 0 {
		limit = DefaultInstancePageLimit
	}
	if limit < 1 || limit > MaxInstancePageLimit {
		return database.Page[types.InstanceRecord]{}, ErrInvalidInput
	}
	var normalizedName *string
	if name != nil {
		value := strings.TrimSpace(*name)
		if value == "" || len(value) > 255 {
			return database.Page[types.InstanceRecord]{}, ErrInvalidInput
		}
		normalizedName = &value
	}

	cursor, err := decodeInstancePageCursor(query.Cursor)
	if err != nil {
		return database.Page[types.InstanceRecord]{}, err
	}
	if cursor != nil {
		valid, validateErr := r.instancePageCursorExists(ctx, tenant, normalizedName, *cursor)
		if validateErr != nil {
			return database.Page[types.InstanceRecord]{}, validateErr
		}
		if !valid {
			return database.Page[types.InstanceRecord]{}, ErrInvalidCursor
		}
	}

	statement := `SELECT ` + instanceSelectColumns + `
		FROM whatsapp_instances i
		LEFT JOIN whatsapp_instance_connections c
		  ON c.tenant_id = i.tenant_id AND c.instance_id = i.id
		WHERE i.tenant_id = ?`
	args := []any{tenant}
	if normalizedName != nil {
		statement += ` AND LOWER(i.name) LIKE ?`
		args = append(args, "%"+strings.ToLower(*normalizedName)+"%")
	}
	if cursor != nil {
		statement += ` AND (i.created_at > ? OR (i.created_at = ? AND i.id > ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	statement += ` ORDER BY i.created_at, i.id LIMIT ?`
	args = append(args, limit+1)

	rows, err := r.base.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return database.Page[types.InstanceRecord]{}, fmt.Errorf("list whatsapp instances: %w", err)
	}
	defer rows.Close()
	items := make([]types.InstanceRecord, 0, limit+1)
	for rows.Next() {
		instance, scanErr := scanInstance(rows)
		if scanErr != nil {
			return database.Page[types.InstanceRecord]{}, scanErr
		}
		items = append(items, types.InstanceRecord{Instance: instance})
	}
	if err := rows.Err(); err != nil {
		return database.Page[types.InstanceRecord]{}, err
	}

	page := database.Page[types.InstanceRecord]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.Next, err = encodeInstancePageCursor(page.Items[len(page.Items)-1].Instance)
		if err != nil {
			return database.Page[types.InstanceRecord]{}, err
		}
	}
	return page, nil
}

func (r *SQLInstanceRepository) instancePageCursorExists(ctx context.Context, tenant string, name *string, cursor instancePageCursor) (bool, error) {
	statement := `SELECT 1 FROM whatsapp_instances i
		WHERE i.tenant_id = ? AND i.created_at = ? AND i.id = ?`
	args := []any{tenant, cursor.CreatedAt, cursor.ID}
	if name != nil {
		statement += ` AND LOWER(i.name) LIKE ?`
		args = append(args, "%"+strings.ToLower(*name)+"%")
	}
	var found int
	err := r.base.db.QueryRowContext(ctx, statement, args...).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("validate whatsapp instance cursor: %w", err)
	}
	return found == 1, nil
}

func encodeInstancePageCursor(instance types.Instance) (string, error) {
	payload, err := json.Marshal(instancePageCursor{CreatedAt: instance.CreatedAt.UTC(), ID: instance.ID})
	if err != nil {
		return "", fmt.Errorf("encode whatsapp instance cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeInstancePageCursor(raw string) (*instancePageCursor, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 512 {
		return nil, ErrInvalidCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var cursor instancePageCursor
	if err := decoder.Decode(&cursor); err != nil {
		return nil, ErrInvalidCursor
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidCursor
	}
	if cursor.ID < 1 || cursor.CreatedAt.IsZero() {
		return nil, ErrInvalidCursor
	}
	cursor.CreatedAt = cursor.CreatedAt.UTC()
	return &cursor, nil
}

func (r *SQLInstanceRepository) FetchDetailsByName(ctx context.Context, grant security.Grant, name string) (types.InstanceDetails, error) {
	if err := grant.Check(authz.ActionInstanceView); err != nil {
		return types.InstanceDetails{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.InstanceDetails{}, err
	}
	item, err := r.findByName(ctx, tenant, name)
	if err != nil {
		return types.InstanceDetails{}, err
	}
	detail := types.InstanceDetails{Instance: item.Instance}
	webhook, err := r.findWebhook(ctx, tenant, item.Instance.ID)
	if err == nil {
		detail.Webhook = &webhook
	} else if !errors.Is(err, ErrWebhookNotFound) {
		return types.InstanceDetails{}, err
	}
	return detail, nil
}

func (r *SQLInstanceRepository) FindAutoConnectInstances(ctx context.Context, grant security.Grant) ([]types.Instance, error) {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return nil, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return nil, err
	}
	rows, err := r.base.db.QueryContext(ctx, `SELECT `+instanceSelectColumns+`
		FROM whatsapp_instances i
		JOIN whatsapp_instance_connections c
		  ON c.tenant_id = i.tenant_id AND c.instance_id = i.id
		WHERE i.tenant_id = ? AND i.status = ? AND c.whatsapp_device_jid IS NOT NULL
		ORDER BY i.created_at, i.id`, tenant, types.InstanceStatusOnline)
	if err != nil {
		return nil, fmt.Errorf("list reconnectable whatsapp instances: %w", err)
	}
	defer rows.Close()
	items := make([]types.Instance, 0)
	for rows.Next() {
		item, scanErr := scanInstance(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLInstanceRepository) Update(ctx context.Context, grant security.Grant, instanceID int64, input types.UpdateInstanceInput) (types.InstanceRecord, error) {
	if err := grant.Check(authz.ActionInstanceUpdate); err != nil {
		return types.InstanceRecord{}, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return types.InstanceRecord{}, err
	}
	sets := make([]string, 0, 5)
	args := make([]any, 0, 7)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return types.InstanceRecord{}, ErrInvalidInput
		}
		sets, args = append(sets, "name = ?"), append(args, name)
	}
	if input.Description.Set {
		sets, args = append(sets, "description = ?"), append(args, input.Description.Value)
	}
	if input.ProfilePicUrl.Set {
		sets, args = append(sets, "profile_pic_url = ?"), append(args, input.ProfilePicUrl.Value)
	}
	if input.ExternalAttributes.Set {
		value := json.RawMessage("null")
		if input.ExternalAttributes.Value != nil {
			value = *input.ExternalAttributes.Value
		}
		if !json.Valid(value) {
			return types.InstanceRecord{}, ErrInvalidJSON
		}
		sets, args = append(sets, "external_attributes = ?"), append(args, string(value))
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = ?")
		args = append(args, time.Now().UTC(), tenant, instanceID)
		result, execErr := r.base.db.ExecContext(ctx,
			`UPDATE whatsapp_instances SET `+strings.Join(sets, ", ")+` WHERE tenant_id = ? AND id = ?`, args...)
		if execErr != nil {
			if looksUnique(execErr) {
				name := ""
				if input.Name != nil {
					name = strings.TrimSpace(*input.Name)
				}
				duplicate, checkErr := r.base.rowExists(ctx,
					`SELECT 1 FROM whatsapp_instances WHERE tenant_id = ? AND name = ? AND id <> ?`,
					tenant, name, instanceID)
				if checkErr == nil && duplicate {
					return types.InstanceRecord{}, ErrInstanceNameAlreadyExists
				}
			}
			return types.InstanceRecord{}, fmt.Errorf("update whatsapp instance: %w", execErr)
		}
		if affected(result) == 0 {
			return types.InstanceRecord{}, ErrInstanceNotFound
		}
	}
	return r.findByID(ctx, tenant, instanceID)
}

func (r *SQLInstanceRepository) UpdateStatus(ctx context.Context, grant security.Grant, instanceID int64, status types.InstanceStatus) error {
	if err := authz.CheckInstanceStatus(grant); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	if !status.IsValid() {
		return ErrInvalidEnum
	}
	result, err := r.base.db.ExecContext(ctx,
		`UPDATE whatsapp_instances SET status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?`,
		status, time.Now().UTC(), tenant, instanceID)
	if err != nil {
		return fmt.Errorf("update whatsapp instance status: %w", err)
	}
	if affected(result) == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

func (r *SQLInstanceRepository) UpdateConnectionState(ctx context.Context, grant security.Grant, input types.UpdateConnectionStateInput) error {
	if err := authz.CheckConnectionState(grant); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	return data.Transaction(ctx, r.base.db, func(txCtx context.Context) error {
		if err := r.ensureConnection(txCtx, tenant, input.InstanceID); err != nil {
			return err
		}
		sets := []string{"updated_at = ?"}
		args := []any{time.Now().UTC()}
		if input.ConnectionStatus != nil {
			if !input.ConnectionStatus.IsValid() {
				return ErrInvalidEnum
			}
			sets, args = append(sets, "connection_status = ?"), append(args, *input.ConnectionStatus)
		}
		if input.LastConnectedAt != nil {
			sets, args = append(sets, "last_connected_at = ?"), append(args, input.LastConnectedAt.UTC())
		}
		if input.LastDisconnectedAt != nil {
			sets, args = append(sets, "last_disconnected_at = ?"), append(args, input.LastDisconnectedAt.UTC())
		}
		if input.LastConnectionAttemptAt != nil {
			sets, args = append(sets, "last_connection_attempt_at = ?"), append(args, input.LastConnectionAttemptAt.UTC())
		}
		if input.LastConnectionError.Set {
			sets, args = append(sets, "last_connection_error = ?"), append(args, input.LastConnectionError.Value)
		}
		if input.LastConnectionEvent.Set {
			sets, args = append(sets, "last_connection_event = ?"), append(args, input.LastConnectionEvent.Value)
		}
		if input.IncrementAttempts {
			sets = append(sets, "connection_attempts = connection_attempts + 1")
		}
		if input.ResetAttempts {
			sets = append(sets, "connection_attempts = 0")
		}
		args = append(args, tenant, input.InstanceID)
		_, execErr := r.base.db.ExecContext(txCtx,
			`UPDATE whatsapp_instance_connections SET `+strings.Join(sets, ", ")+` WHERE tenant_id = ? AND instance_id = ?`,
			args...)
		return execErr
	})
}

func (r *SQLInstanceRepository) SaveWhatsAppDevice(ctx context.Context, grant security.Grant, input types.SaveWhatsAppDeviceInput) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	err = data.Transaction(ctx, r.base.db, func(txCtx context.Context) error {
		if err := r.ensureConnection(txCtx, tenant, input.InstanceID); err != nil {
			return err
		}
		result, execErr := r.base.db.ExecContext(txCtx, `UPDATE whatsapp_instance_connections
			SET whatsapp_device_jid = ?, whatsapp_owner_jid = ?, whatsapp_phone_number = ?, updated_at = ?
			WHERE tenant_id = ? AND instance_id = ?`,
			input.DeviceJID, input.OwnerJID, input.PhoneNumber, time.Now().UTC(), tenant, input.InstanceID)
		if execErr != nil {
			return execErr
		}
		if affected(result) == 0 {
			return ErrInstanceNotFound
		}
		return nil
	})
	if err != nil && looksUnique(err) {
		duplicate, checkErr := r.base.rowExists(ctx,
			`SELECT 1 FROM whatsapp_instance_connections WHERE whatsapp_device_jid = ?
			 AND (tenant_id <> ? OR instance_id <> ?)`, input.DeviceJID, tenant, input.InstanceID)
		if checkErr == nil && duplicate {
			return fmt.Errorf("%w: %s", ErrWhatsAppDeviceAlreadyLinked, input.DeviceJID)
		}
	}
	if err != nil {
		return fmt.Errorf("save whatsapp device: %w", err)
	}
	return nil
}

func (r *SQLInstanceRepository) ClearWhatsAppDevice(ctx context.Context, grant security.Grant, instanceID int64) error {
	if err := authz.CheckClearDevice(grant); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	result, err := r.base.db.ExecContext(ctx, `UPDATE whatsapp_instance_connections SET
		whatsapp_device_jid = NULL, whatsapp_owner_jid = NULL,
		whatsapp_phone_number = NULL, profile_pic_id = NULL, updated_at = ?
		WHERE tenant_id = ? AND instance_id = ?`, time.Now().UTC(), tenant, instanceID)
	if err != nil {
		return fmt.Errorf("clear whatsapp device: %w", err)
	}
	if affected(result) == 0 {
		exists, checkErr := r.base.instanceExists(ctx, tenant, instanceID)
		if checkErr != nil {
			return checkErr
		}
		if !exists {
			return ErrInstanceNotFound
		}
	}
	return nil
}

func (r *SQLInstanceRepository) UpdateProfilePicture(ctx context.Context, grant security.Grant, instanceID int64, profilePicURL *string, profilePicID *string) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	return data.Transaction(ctx, r.base.db, func(txCtx context.Context) error {
		result, execErr := r.base.db.ExecContext(txCtx,
			`UPDATE whatsapp_instances SET profile_pic_url = ?, updated_at = ? WHERE tenant_id = ? AND id = ?`,
			profilePicURL, time.Now().UTC(), tenant, instanceID)
		if execErr != nil {
			return execErr
		}
		if affected(result) == 0 {
			return ErrInstanceNotFound
		}
		if err := r.ensureConnection(txCtx, tenant, instanceID); err != nil {
			return err
		}
		_, execErr = r.base.db.ExecContext(txCtx,
			`UPDATE whatsapp_instance_connections SET profile_pic_id = ?, updated_at = ? WHERE tenant_id = ? AND instance_id = ?`,
			profilePicID, time.Now().UTC(), tenant, instanceID)
		return execErr
	})
}

func (r *SQLInstanceRepository) TryAcquireConnectionLock(ctx context.Context, grant security.Grant, rawID string) (bool, error) {
	if err := authz.CheckConnectionLock(grant); err != nil {
		return false, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return false, err
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return false, ErrInvalidInput
	}
	now := time.Now().UTC()
	result, err := r.base.db.ExecContext(ctx, `UPDATE whatsapp_instances
		SET connection_lock_owner = ?, connection_lock_until = ?
		WHERE tenant_id = ? AND id = ?
		  AND (connection_lock_until IS NULL OR connection_lock_until <= ? OR connection_lock_owner = ?)`,
		r.lockOwner, now.Add(2*time.Minute), tenant, id, now, r.lockOwner)
	if err != nil {
		return false, fmt.Errorf("acquire whatsapp connection lock: %w", err)
	}
	return affected(result) > 0, nil
}

func (r *SQLInstanceRepository) ReleaseConnectionLock(ctx context.Context, grant security.Grant, rawID string) error {
	if err := authz.CheckConnectionLock(grant); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return ErrInvalidInput
	}
	_, err = r.base.db.ExecContext(ctx, `UPDATE whatsapp_instances
		SET connection_lock_owner = NULL, connection_lock_until = NULL
		WHERE tenant_id = ? AND id = ? AND connection_lock_owner = ?`,
		tenant, id, r.lockOwner)
	return err
}

func (r *SQLInstanceRepository) EnsureDeletable(ctx context.Context, grant security.Grant, instanceID int64) error {
	if err := grant.Check(authz.ActionInstanceDelete); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	return r.ensureDeletable(ctx, tenant, instanceID)
}

func (r *SQLInstanceRepository) ensureDeletable(ctx context.Context, tenant string, instanceID int64) error {
	counts := &InstanceDependenciesError{InstanceID: instanceID}
	queries := []struct {
		table string
		out   *int64
	}{
		{"whatsapp_messages", &counts.Messages},
		{"whatsapp_chats", &counts.Chats},
		{"whatsapp_contacts", &counts.Contacts},
		{"whatsapp_webhooks", &counts.Webhooks},
	}
	for _, item := range queries {
		query := `SELECT COUNT(*) FROM ` + item.table + ` WHERE tenant_id = ? AND instance_id = ?`
		if err := r.base.db.QueryRowContext(ctx, query, tenant, instanceID).Scan(item.out); err != nil {
			return fmt.Errorf("count whatsapp instance dependencies: %w", err)
		}
	}
	if counts.Messages+counts.Chats+counts.Contacts+counts.Webhooks > 0 {
		return counts
	}
	return nil
}

func (r *SQLInstanceRepository) Delete(ctx context.Context, grant security.Grant, instanceID int64, force bool) error {
	if err := grant.Check(authz.ActionInstanceDelete); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	if !force {
		if err := r.ensureDeletable(ctx, tenant, instanceID); err != nil {
			return err
		}
	}
	result, err := r.base.db.ExecContext(ctx,
		`DELETE FROM whatsapp_instances WHERE tenant_id = ? AND id = ?`, tenant, instanceID)
	if err != nil {
		return fmt.Errorf("delete whatsapp instance: %w", err)
	}
	if affected(result) == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

func (r *SQLInstanceRepository) ensureConnection(ctx context.Context, tenant string, instanceID int64) error {
	exists, err := r.base.instanceExists(ctx, tenant, instanceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInstanceNotFound
	}
	now := time.Now().UTC()
	_, err = r.base.db.ExecContext(ctx, `INSERT INTO whatsapp_instance_connections (
		tenant_id, instance_id, connection_status, connection_attempts, created_at, updated_at
	) VALUES (?, ?, ?, 0, ?, ?) ON CONFLICT (tenant_id, instance_id) DO NOTHING`,
		tenant, instanceID, types.InstanceConnectionStatusOffline, now, now)
	return err
}

func (r *SQLInstanceRepository) findByID(ctx context.Context, tenant string, id int64) (types.InstanceRecord, error) {
	row := r.base.db.QueryRowContext(ctx, `SELECT `+instanceSelectColumns+`
		FROM whatsapp_instances i
		LEFT JOIN whatsapp_instance_connections c
		  ON c.tenant_id = i.tenant_id AND c.instance_id = i.id
		WHERE i.tenant_id = ? AND i.id = ?`, tenant, id)
	instance, err := scanInstance(row)
	return types.InstanceRecord{Instance: instance}, err
}

func (r *SQLInstanceRepository) findWebhook(ctx context.Context, tenant string, instanceID int64) (types.Webhook, error) {
	row := r.base.db.QueryRowContext(ctx, `SELECT id, url, enabled, events, created_at, updated_at, instance_id
		FROM whatsapp_webhooks WHERE tenant_id = ? AND instance_id = ?`, tenant, instanceID)
	return scanWebhook(row)
}

func scanInstance(row rowScanner) (types.Instance, error) {
	var (
		item                                             types.Instance
		description, ownerJID, profilePicURL             sql.NullString
		deviceJID, whatsappOwnerJID, phone, profilePicID sql.NullString
		lastConnected, lastDisconnected, lastAttempt     sql.NullTime
		lastError, lastEvent                             sql.NullString
		external                                         string
		status, connectionStatus                         string
	)
	err := row.Scan(
		&item.ID, &item.TenantID, &item.Name, &description, &status, &ownerJID,
		&profilePicURL, &external, &item.CreatedAt, &item.UpdatedAt,
		&connectionStatus, &deviceJID, &whatsappOwnerJID, &phone, &profilePicID,
		&lastConnected, &lastDisconnected, &lastAttempt, &lastError, &lastEvent,
		&item.ConnectionAttempts,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Instance{}, ErrInstanceNotFound
	}
	if err != nil {
		return types.Instance{}, fmt.Errorf("scan whatsapp instance: %w", err)
	}
	item.Description = stringPointer(description)
	item.OwnerJid = stringPointer(ownerJID)
	item.ProfilePicUrl = stringPointer(profilePicURL)
	item.WhatsAppDeviceJid = stringPointer(deviceJID)
	item.WhatsAppOwnerJid = stringPointer(whatsappOwnerJID)
	item.WhatsAppPhone = stringPointer(phone)
	item.ProfilePicID = stringPointer(profilePicID)
	item.LastConnectedAt = timePointer(lastConnected)
	item.LastDisconnectedAt = timePointer(lastDisconnected)
	item.LastAttemptAt = timePointer(lastAttempt)
	item.LastError = stringPointer(lastError)
	item.LastEvent = stringPointer(lastEvent)
	item.Status = types.InstanceStatus(status)
	item.ConnectionStatus = types.InstanceConnectionStatus(connectionStatus)
	if !item.ConnectionStatus.IsValid() {
		item.ConnectionStatus = types.InstanceConnectionStatusOffline
	}
	item.ExternalAttributes = json.RawMessage(external)
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}
