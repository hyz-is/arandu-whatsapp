package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
)

// createWhatsAppTables creates the final tenant-scoped domain schema.
type createWhatsAppTables struct{ hesapemigrations.BaseMigration }

func (createWhatsAppTables) GetName() string {
	return "20260825_0001_create_whatsapp_tables"
}

func (createWhatsAppTables) Up(ctx context.Context, conn hesapemigrations.Connection) error {
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

func (createWhatsAppTables) Down(ctx context.Context, conn hesapemigrations.Connection) error {
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

var (
	_ foundation.Migration                 = createWhatsAppTables{}
	_ hesapemigrations.ReversibleMigration = createWhatsAppTables{}
)
