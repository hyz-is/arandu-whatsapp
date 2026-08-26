package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
)

// createWebhookDeliveries creates the durable snapshot owned by this package.
// The native Hesape queue owns its jobs table and migrations separately.
type createWebhookDeliveries struct{ hesapemigrations.BaseMigration }

func (createWebhookDeliveries) GetName() string {
	return "20260825_0003_create_webhook_deliveries"
}

func (createWebhookDeliveries) Up(ctx context.Context, conn hesapemigrations.Connection) error {
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

func (createWebhookDeliveries) Down(ctx context.Context, conn hesapemigrations.Connection) error {
	return statements(ctx, conn,
		`DROP INDEX idx_whatsapp_webhook_deliveries_retention`,
		`DROP TABLE whatsapp_webhook_deliveries`,
	)
}

var (
	_ foundation.Migration                 = createWebhookDeliveries{}
	_ hesapemigrations.ReversibleMigration = createWebhookDeliveries{}
)
