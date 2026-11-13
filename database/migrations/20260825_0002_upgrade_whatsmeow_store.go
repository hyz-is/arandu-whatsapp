package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
	waLog "go.mau.fi/whatsmeow/util/log"

	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

// upgradeWhatsMeowStore delegates schema ownership to WhatsMeow. The upstream
// upgrader manages its own transactions, so Arandu must not wrap this migration
// in another one.
//
// The module registers it without an upgrader: the store is opened at Up, over
// the handle of the connection the migrator runs on. That keeps the registered
// value the same from one module to the next, so a process that builds the
// module twice -- every test that boots the application again -- registers
// one migration under its name rather than two that compare different.
type upgradeWhatsMeowStore struct {
	hesapemigrations.BaseMigration
	upgrader SchemaUpgrader
}

// rawConnection is the migrator's connection as Hesape builds it: the handle
// and the driver it was opened for.
type rawConnection interface {
	GetRawPDO() *sql.DB
	GetDriverName() string
}

// upgraderFor is the WhatsMeow store over the migrator's own handle. On
// PostgreSQL it first makes sure the version table exists in the connection's
// own schema; see ensureVersionTable.
func upgraderFor(ctx context.Context, conn hesapemigrations.Connection) (SchemaUpgrader, error) {
	raw, ok := conn.(rawConnection)
	if !ok || raw.GetRawPDO() == nil {
		return nil, errors.New("whatsapp: WhatsMeow schema upgrader is unavailable")
	}
	dialect, err := data.ParseDialect(raw.GetDriverName())
	if err != nil {
		return nil, fmt.Errorf("whatsapp: WhatsMeow schema upgrader: %w", err)
	}
	handle := data.Wrap(raw.GetRawPDO(), dialect)
	if err := ensureVersionTable(ctx, handle); err != nil {
		return nil, err
	}
	return internalwhatsapp.NewWhatsAppSessionContainer(handle, waLog.Noop)
}

// ensureVersionTable creates WhatsMeow's version table in the schema the
// connection resolves unqualified names to (its search_path), PostgreSQL only.
//
// WhatsMeow's upgrader asks information_schema whether the version table (and
// its compat column) exists by name alone, across every schema of the database,
// and then reads and writes it by name alone, through the search_path. When a
// table of that name lives in another schema -- a development database that
// also holds a copy in public, a test that migrates into a schema of its own --
// the first answer is yes and the read that follows fails with `relation
// "whatsmeow_version" does not exist`. Production uses the one public schema,
// where both questions agree, which is why only the other layouts broke.
//
// With the table already in place in the connection's own schema, both
// questions are true for the right reason and the upgrader proceeds to create
// the rest. The statements are idempotent and change nothing on a store that is
// already upgraded.
func ensureVersionTable(ctx context.Context, db *data.DB) error {
	if db.Dialect() != data.DialectPostgres {
		return nil
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS whatsmeow_version (version INTEGER, compat INTEGER)`,
		`ALTER TABLE whatsmeow_version ADD COLUMN IF NOT EXISTS compat INTEGER`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("whatsapp: prepare WhatsMeow version table: %w", err)
		}
	}
	return nil
}

func (upgradeWhatsMeowStore) GetName() string {
	return "20260825_0002_upgrade_whatsmeow_store"
}

func (m upgradeWhatsMeowStore) Up(ctx context.Context, conn hesapemigrations.Connection) error {
	if pretending, ok := conn.(interface{ Pretending() bool }); ok && pretending.Pretending() {
		return nil
	}
	upgrader := m.upgrader
	if upgrader == nil {
		resolved, err := upgraderFor(ctx, conn)
		if err != nil {
			return err
		}
		upgrader = resolved
	}
	if err := upgrader.Upgrade(ctx); err != nil {
		return fmt.Errorf("whatsapp: upgrade WhatsMeow store schema: %w", err)
	}
	return nil
}

var _ foundation.Migration = upgradeWhatsMeowStore{}

// Irreversible says why a rollback leaves the WhatsMeow store as it is: the
// schema is WhatsMeow's, which publishes upgrades and no downgrade. A rollback
// of the module's own tables proceeds around it, and applying this again
// later is a no-op on a store already at its version.
func (upgradeWhatsMeowStore) Irreversible() string {
	return "WhatsMeow owns its store schema and publishes no downgrade"
}

var _ hesapemigrations.IrreversibleMigration = upgradeWhatsMeowStore{}
