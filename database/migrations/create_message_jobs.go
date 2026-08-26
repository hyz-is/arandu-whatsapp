package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
)

// createMessageJobs creates the durable mention-all snapshots. The native
// Hesape queue owns its jobs table and stores only the snapshot process id.
type createMessageJobs struct{ hesapemigrations.BaseMigration }

func (createMessageJobs) GetName() string {
	return "20260825_0004_create_message_jobs"
}

func (createMessageJobs) Up(ctx context.Context, conn hesapemigrations.Connection) error {
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

func (createMessageJobs) Down(ctx context.Context, conn hesapemigrations.Connection) error {
	return statements(ctx, conn, `DROP TABLE whatsapp_message_jobs`)
}

var (
	_ foundation.Migration                 = createMessageJobs{}
	_ hesapemigrations.ReversibleMigration = createMessageJobs{}
)
