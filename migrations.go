package whatsapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/hesape/database/migrations"
)

// whatsMeowSchemaUpgrader is the narrow upstream schema contract used by the
// migration. The sqlstore container built by New satisfies it directly.
type whatsMeowSchemaUpgrader interface {
	Upgrade(context.Context) error
}

// whatsappMigrations is intentionally explicit. The package owns the
// tenant-scoped whatsapp_* tables; WhatsMeow remains the owner of its own store
// schema and upgrades it through the container supplied here.
func whatsappMigrations(upgrader whatsMeowSchemaUpgrader) []foundation.Migration {
	return []foundation.Migration{
		createWhatsAppTables{},
		upgradeWhatsMeowStore{
			BaseMigration: migrations.BaseMigration{OutsideTransaction: true},
			upgrader:      upgrader,
		},
		createWebhookDeliveries{},
		createMessageJobs{},
	}
}

func statements(ctx context.Context, conn migrations.Connection, queries ...string) error {
	for _, query := range queries {
		if _, err := conn.Statement(ctx, query, nil); err != nil {
			return err
		}
	}
	return nil
}

// createWhatsAppTables creates the final tenant-scoped domain schema.
type createWhatsAppTables struct{ migrations.BaseMigration }

func (createWhatsAppTables) GetName() string {
	return "20260825_0001_create_whatsapp_tables"
}

func (createWhatsAppTables) Up(ctx context.Context, conn migrations.Connection) error {
	return statements(ctx, conn,
		`CREATE TABLE whatsapp_instances (
    id                       BIGINT NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    name                     VARCHAR(255) NOT NULL,
    description              VARCHAR(255),
    status                   VARCHAR(32) NOT NULL,
    owner_jid                VARCHAR(100),
    profile_pic_url          VARCHAR(500),
    external_attributes      TEXT NOT NULL,
    connection_lock_owner    VARCHAR(255),
    connection_lock_until    TIMESTAMP,
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
)`,
		`CREATE INDEX whatsapp_instances_tenant_created_idx
    ON whatsapp_instances (tenant_id, created_at, id)`,
		`CREATE INDEX whatsapp_instances_tenant_status_idx
    ON whatsapp_instances (tenant_id, status, id)`,
		`CREATE TABLE whatsapp_messages (
    id                       BIGINT NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    key_id                   VARCHAR(100) NOT NULL,
    key_remote_jid           VARCHAR(100),
    key_lid                  VARCHAR(100),
    key_from_me              BOOLEAN NOT NULL,
    key_participant          VARCHAR(100),
    key_participant_lid      VARCHAR(100),
    push_name                VARCHAR(100),
    message_type             VARCHAR(100) NOT NULL,
    content                  TEXT NOT NULL,
    message_timestamp        BIGINT NOT NULL,
    device                   VARCHAR(32) NOT NULL,
    is_group                 BOOLEAN,
    metadata                 TEXT,
    created_at               TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, instance_id, id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_messages_tenant_instance_id_idx
    ON whatsapp_messages (tenant_id, instance_id, id)`,
		`CREATE INDEX whatsapp_messages_tenant_remote_idx
    ON whatsapp_messages (tenant_id, instance_id, key_remote_jid, id)`,
		`CREATE INDEX whatsapp_messages_tenant_from_me_idx
    ON whatsapp_messages (tenant_id, instance_id, key_from_me, id)`,
		`CREATE INDEX whatsapp_messages_tenant_type_idx
    ON whatsapp_messages (tenant_id, instance_id, message_type, id)`,
		`CREATE INDEX whatsapp_messages_tenant_device_idx
    ON whatsapp_messages (tenant_id, instance_id, device, id)`,
		`CREATE INDEX whatsapp_messages_tenant_timestamp_idx
    ON whatsapp_messages (tenant_id, instance_id, message_timestamp, id)`,
		`CREATE UNIQUE INDEX whatsapp_messages_tenant_instance_key_uidx
    ON whatsapp_messages (tenant_id, instance_id, key_id)`,
		`CREATE TABLE whatsapp_message_updates (
    id                       BIGINT NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    message_id               BIGINT NOT NULL,
    date_time                TIMESTAMP NOT NULL,
    status                   VARCHAR(30) NOT NULL,
    FOREIGN KEY (tenant_id, instance_id, message_id)
        REFERENCES whatsapp_messages (tenant_id, instance_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_updates_message_status_idx
    ON whatsapp_message_updates (tenant_id, instance_id, message_id, status, id)`,
		`CREATE INDEX whatsapp_updates_message_time_idx
    ON whatsapp_message_updates (tenant_id, instance_id, message_id, date_time, id)`,
		`CREATE UNIQUE INDEX whatsapp_updates_message_status_uidx
    ON whatsapp_message_updates (tenant_id, instance_id, message_id, status, date_time)`,
		`CREATE TABLE whatsapp_chats (
    id                       BIGINT NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    remote_jid               VARCHAR(100) NOT NULL,
    content                  TEXT,
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_chats_tenant_created_idx
    ON whatsapp_chats (tenant_id, instance_id, created_at, id)`,
		`CREATE TABLE whatsapp_contacts (
    id                       BIGINT NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    remote_jid               VARCHAR(100) NOT NULL,
    push_name                VARCHAR(100),
    profile_pic_url          VARCHAR(500),
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_contacts_tenant_created_idx
    ON whatsapp_contacts (tenant_id, instance_id, created_at, id)`,
		`CREATE UNIQUE INDEX whatsapp_contacts_tenant_remote_uidx
    ON whatsapp_contacts (tenant_id, instance_id, remote_jid)`,
		`CREATE TABLE whatsapp_webhooks (
    id                       BIGINT NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    url                      VARCHAR(500) NOT NULL,
    enabled                  BOOLEAN NOT NULL,
    events                   TEXT NOT NULL,
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, instance_id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_webhooks_tenant_enabled_idx
    ON whatsapp_webhooks (tenant_id, enabled, id)`,
		`CREATE TABLE whatsapp_instance_connections (
    tenant_id                  VARCHAR(255) NOT NULL,
    instance_id                BIGINT NOT NULL,
    connection_status          VARCHAR(64) NOT NULL,
    whatsapp_device_jid        VARCHAR(100),
    whatsapp_owner_jid         VARCHAR(100),
    whatsapp_phone_number      VARCHAR(32),
    profile_pic_id             VARCHAR(255),
    last_connected_at          TIMESTAMP,
    last_disconnected_at       TIMESTAMP,
    last_connection_attempt_at TIMESTAMP,
    last_connection_error      VARCHAR(255),
    last_connection_event      VARCHAR(100),
    connection_attempts        BIGINT NOT NULL,
    created_at                 TIMESTAMP NOT NULL,
    updated_at                 TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, instance_id),
    UNIQUE (whatsapp_device_jid),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_connections_tenant_status_idx
    ON whatsapp_instance_connections (tenant_id, connection_status, instance_id)`,
		`CREATE TABLE whatsapp_address_mappings (
    tenant_id          VARCHAR(255) NOT NULL,
    instance_id        BIGINT NOT NULL,
    alias              VARCHAR(100) NOT NULL,
    normalized_phone   VARCHAR(32) NOT NULL,
    canonical_jid      VARCHAR(100) NOT NULL,
    lid_jid            VARCHAR(100),
    resolved_at        TIMESTAMP NOT NULL,
    expires_at         TIMESTAMP NOT NULL,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, instance_id, alias),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_address_mappings_canonical_idx
    ON whatsapp_address_mappings (tenant_id, instance_id, canonical_jid)`,
		`CREATE INDEX whatsapp_address_mappings_expires_idx
    ON whatsapp_address_mappings (tenant_id, instance_id, expires_at)`,
	)
}

func (createWhatsAppTables) Down(ctx context.Context, conn migrations.Connection) error {
	return statements(ctx, conn,
		`DROP TABLE whatsapp_address_mappings`,
		`DROP TABLE whatsapp_instance_connections`,
		`DROP TABLE whatsapp_webhooks`,
		`DROP TABLE whatsapp_contacts`,
		`DROP TABLE whatsapp_chats`,
		`DROP TABLE whatsapp_message_updates`,
		`DROP TABLE whatsapp_messages`,
		`DROP TABLE whatsapp_instances`,
	)
}

// upgradeWhatsMeowStore delegates schema ownership to WhatsMeow. The upstream
// upgrader manages its own transactions, so Arandu must not wrap this migration
// in another one.
type upgradeWhatsMeowStore struct {
	migrations.BaseMigration
	upgrader whatsMeowSchemaUpgrader
}

func (upgradeWhatsMeowStore) GetName() string {
	return "20260825_0002_upgrade_whatsmeow_store"
}

func (m upgradeWhatsMeowStore) Up(ctx context.Context, conn migrations.Connection) error {
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

// createWebhookDeliveries creates the durable snapshot owned by this package.
// The native Hesape queue owns its jobs table and migrations separately.
type createWebhookDeliveries struct{ migrations.BaseMigration }

func (createWebhookDeliveries) GetName() string {
	return "20260825_0003_create_webhook_deliveries"
}

func (createWebhookDeliveries) Up(ctx context.Context, conn migrations.Connection) error {
	return statements(ctx, conn,
		`CREATE TABLE whatsapp_webhook_deliveries (
    id                       VARCHAR(36) NOT NULL PRIMARY KEY,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    event                    VARCHAR(100) NOT NULL,
    target                   VARCHAR(32) NOT NULL,
    url                      VARCHAR(500) NOT NULL,
    body                     TEXT NOT NULL,
    headers                  TEXT NOT NULL,
    status                   VARCHAR(32) NOT NULL,
    attempts                 INTEGER NOT NULL,
    response_status          INTEGER,
    response_body            TEXT,
    last_error               TEXT,
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    delivered_at             TIMESTAMP,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_webhook_deliveries_status_idx
    ON whatsapp_webhook_deliveries (tenant_id, status, created_at, id)`,
		`CREATE INDEX whatsapp_webhook_deliveries_instance_idx
    ON whatsapp_webhook_deliveries (tenant_id, instance_id, created_at, id)`,
		`CREATE INDEX idx_whatsapp_webhook_deliveries_retention
    ON whatsapp_webhook_deliveries (tenant_id, created_at, id)`,
	)
}

func (createWebhookDeliveries) Down(ctx context.Context, conn migrations.Connection) error {
	return statements(ctx, conn,
		`DROP INDEX idx_whatsapp_webhook_deliveries_retention`,
		`DROP TABLE whatsapp_webhook_deliveries`,
	)
}

// createMessageJobs creates the durable mention-all snapshots. The native
// Hesape queue owns its jobs table and stores only the snapshot process id.
type createMessageJobs struct{ migrations.BaseMigration }

func (createMessageJobs) GetName() string {
	return "20260825_0004_create_message_jobs"
}

func (createMessageJobs) Up(ctx context.Context, conn migrations.Connection) error {
	return statements(ctx, conn,
		`CREATE TABLE whatsapp_message_jobs (
    process_id               VARCHAR(36) NOT NULL PRIMARY KEY,
    message_job_id           VARCHAR(36) NOT NULL,
    cleanup_job_id           VARCHAR(36) NOT NULL,
    tenant_id                VARCHAR(255) NOT NULL,
    instance_id              BIGINT NOT NULL,
    instance_name            VARCHAR(255) NOT NULL,
    remote_jid               VARCHAR(100) NOT NULL,
    message_id               VARCHAR(100) NOT NULL,
    message_type             VARCHAR(100) NOT NULL,
    message_payload          TEXT NOT NULL,
    content                  TEXT NOT NULL,
    presence                 VARCHAR(32),
    delay_ms                 BIGINT NOT NULL,
    external_attributes      TEXT NOT NULL,
    webhook_instance         TEXT NOT NULL,
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, process_id),
    UNIQUE (tenant_id, instance_id, message_id),
    UNIQUE (message_job_id),
    UNIQUE (cleanup_job_id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
		`CREATE INDEX whatsapp_message_jobs_instance_idx
    ON whatsapp_message_jobs (tenant_id, instance_id, created_at, process_id)`,
		`CREATE INDEX whatsapp_message_jobs_retention_idx
    ON whatsapp_message_jobs (tenant_id, created_at, process_id)`,
	)
}

func (createMessageJobs) Down(ctx context.Context, conn migrations.Connection) error {
	return statements(ctx, conn, `DROP TABLE whatsapp_message_jobs`)
}

var (
	_ foundation.Migration           = createWhatsAppTables{}
	_ foundation.Migration           = upgradeWhatsMeowStore{}
	_ foundation.Migration           = createWebhookDeliveries{}
	_ foundation.Migration           = createMessageJobs{}
	_ migrations.ReversibleMigration = createWhatsAppTables{}
	_ migrations.ReversibleMigration = createWebhookDeliveries{}
	_ migrations.ReversibleMigration = createMessageJobs{}
)
