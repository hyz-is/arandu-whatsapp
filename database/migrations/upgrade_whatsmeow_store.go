package migrations

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
)

// upgradeWhatsMeowStore delegates schema ownership to WhatsMeow. The upstream
// upgrader manages its own transactions, so Arandu must not wrap this migration
// in another one.
type upgradeWhatsMeowStore struct {
	hesapemigrations.BaseMigration
	upgrader SchemaUpgrader
}

func (upgradeWhatsMeowStore) GetName() string {
	return "20260825_0002_upgrade_whatsmeow_store"
}

func (m upgradeWhatsMeowStore) Up(ctx context.Context, conn hesapemigrations.Connection) error {
	if pretending, ok := conn.(interface{ Pretending() bool }); ok && pretending.Pretending() {
		return nil
	}
	if m.upgrader == nil {
		return errors.New("whatsapp: WhatsMeow schema upgrader is unavailable")
	}
	if err := m.upgrader.Upgrade(ctx); err != nil {
		return fmt.Errorf("whatsapp: upgrade WhatsMeow store schema: %w", err)
	}
	return nil
}

var _ foundation.Migration = upgradeWhatsMeowStore{}
