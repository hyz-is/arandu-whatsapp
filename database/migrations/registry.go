// Package migrations declares the ordered schema changes owned by the WhatsApp
// module.
package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
)

// SchemaUpgrader is the narrow upstream schema contract required by the
// delegated WhatsMeow migration.
type SchemaUpgrader interface {
	Upgrade(context.Context) error
}

// Migrations returns the module migrations in their immutable application
// order. The module owns the whatsapp_* tables, while WhatsMeow upgrades its
// own store through the supplied upgrader.
func Migrations(upgrader SchemaUpgrader) []foundation.Migration {
	return []foundation.Migration{
		createWhatsAppTables{},
		upgradeWhatsMeowStore{
			BaseMigration: hesapemigrations.BaseMigration{OutsideTransaction: true},
			upgrader:      upgrader,
		},
		createWebhookDeliveries{},
		createMessageJobs{},
	}
}
