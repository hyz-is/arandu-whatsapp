package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	value.Time = value.Time.UTC()
	return &value.Time
}

func boolPointer(value sql.NullBool) *bool {
	if !value.Valid {
		return nil
	}
	return &value.Bool
}

func jsonText(value json.RawMessage, fallback string) string {
	if len(value) == 0 || !json.Valid(value) {
		return fallback
	}
	return string(value)
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func (b *Base) instanceExists(ctx context.Context, tenant string, instanceID int64) (bool, error) {
	exists, err := b.rowExists(ctx,
		`SELECT 1 FROM whatsapp_instances WHERE tenant_id = ? AND id = ?`,
		tenant, instanceID,
	)
	if err != nil {
		return false, fmt.Errorf("check whatsapp instance: %w", err)
	}
	return exists, nil
}

func (b *Base) rowExists(ctx context.Context, query string, args ...any) (bool, error) {
	var one int
	err := b.db.QueryRowContext(ctx, query, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func affected(result sql.Result) int64 {
	n, err := result.RowsAffected()
	if err != nil {
		return -1
	}
	return n
}
