package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

type SQLAddressMappingRepository struct{ base *Base }

func NewAddressMappingRepository(base *Base) *SQLAddressMappingRepository {
	return &SQLAddressMappingRepository{base: base}
}

var _ address.AddressMappingRepository = (*SQLAddressMappingRepository)(nil)

func (r *SQLAddressMappingRepository) FindByAlias(ctx context.Context, grant security.Grant, instanceID int64, alias string) (*address.AddressMapping, error) {
	if err := authz.CheckAddressResolution(grant); err != nil {
		return nil, err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return nil, err
	}
	var item address.AddressMapping
	var lid sql.NullString
	err = r.base.db.QueryRowContext(ctx, `SELECT instance_id, normalized_phone, canonical_jid,
		lid_jid, resolved_at, expires_at FROM whatsapp_address_mappings
		WHERE tenant_id = ? AND instance_id = ? AND alias = ?`, tenant, instanceID, alias).
		Scan(&item.InstanceID, &item.NormalizedPhone, &item.CanonicalJID, &lid, &item.ResolvedAt, &item.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, address.ErrAddressMappingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find whatsapp address mapping: %w", err)
	}
	item.LIDJID = stringPointer(lid)
	rows, err := r.base.db.QueryContext(ctx, `SELECT alias FROM whatsapp_address_mappings
		WHERE tenant_id = ? AND instance_id = ? AND canonical_jid = ? ORDER BY alias`,
		tenant, instanceID, item.CanonicalJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var aliasValue string
		if err := rows.Scan(&aliasValue); err != nil {
			return nil, err
		}
		item.Aliases = append(item.Aliases, aliasValue)
	}
	return &item, rows.Err()
}

func (r *SQLAddressMappingRepository) Upsert(ctx context.Context, grant security.Grant, mapping address.AddressMapping) error {
	if err := authz.CheckAddressResolution(grant); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	return data.Transaction(ctx, r.base.db, func(txCtx context.Context) error {
		if _, err := r.base.db.ExecContext(txCtx, `DELETE FROM whatsapp_address_mappings
			WHERE tenant_id = ? AND instance_id = ? AND canonical_jid = ?`,
			tenant, mapping.InstanceID, mapping.CanonicalJID); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, aliasValue := range mapping.Aliases {
			if aliasValue == "" {
				continue
			}
			_, err := r.base.db.ExecContext(txCtx, `INSERT INTO whatsapp_address_mappings (
				tenant_id, instance_id, alias, normalized_phone, canonical_jid, lid_jid,
				resolved_at, expires_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id, instance_id, alias) DO UPDATE SET
				normalized_phone = excluded.normalized_phone,
				canonical_jid = excluded.canonical_jid,
				lid_jid = excluded.lid_jid,
				resolved_at = excluded.resolved_at,
				expires_at = excluded.expires_at,
				updated_at = excluded.updated_at`,
				tenant, mapping.InstanceID, aliasValue, mapping.NormalizedPhone,
				mapping.CanonicalJID, mapping.LIDJID, mapping.ResolvedAt.UTC(),
				mapping.ExpiresAt.UTC(), now, now)
			if err != nil {
				return fmt.Errorf("upsert whatsapp address mapping: %w", err)
			}
		}
		return nil
	})
}

func (r *SQLAddressMappingRepository) DeleteByCanonicalJID(ctx context.Context, grant security.Grant, instanceID int64, canonicalJID string) error {
	if err := authz.CheckAddressResolution(grant); err != nil {
		return err
	}
	tenant, err := tenantFromGrant(grant)
	if err != nil {
		return err
	}
	_, err = r.base.db.ExecContext(ctx, `DELETE FROM whatsapp_address_mappings
		WHERE tenant_id = ? AND instance_id = ? AND canonical_jid = ?`, tenant, instanceID, canonicalJID)
	return err
}
