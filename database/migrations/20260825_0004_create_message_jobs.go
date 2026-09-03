package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// createMessageJobs creates the durable mention-all snapshots. The native
// Hesape queue owns its jobs table and stores only the snapshot process id.
type createMessageJobs struct{ hesapemigrations.BaseMigration }

func (createMessageJobs) GetName() string {
	return "20260825_0004_create_message_jobs"
}

func (createMessageJobs) Up(ctx context.Context, conn hesapemigrations.Connection) error {
	return conn.Schema().Create(ctx, "whatsapp_message_jobs", func(table *schema.Blueprint) {
		table.String("process_id", 36).Primary()
		table.String("message_job_id", 36)
		table.String("cleanup_job_id", 36)
		table.String("tenant_id", 255)
		table.BigInteger("instance_id")
		table.String("instance_name", 255)
		table.String("remote_jid", 100)
		table.String("message_id", 100)
		table.String("message_type", 100)
		table.Text("message_payload")
		table.Text("content")
		table.String("presence", 32).Nullable()
		table.BigInteger("delay_ms")
		table.Text("external_attributes")
		table.Text("webhook_instance")
		table.Timestamp("created_at", microsecondPrecision)
		table.Timestamp("updated_at", microsecondPrecision)

		table.Unique([]string{"tenant_id", "process_id"}, "whatsapp_message_jobs_tenant_process_uidx")
		// One snapshot per outbound message: the retry that re-enqueues a
		// mention-all is refused by the engine rather than sending twice.
		table.Unique([]string{"tenant_id", "instance_id", "message_id"}, "whatsapp_message_jobs_tenant_message_uidx")
		table.Unique([]string{"message_job_id"}, "whatsapp_message_jobs_message_job_uidx")
		table.Unique([]string{"cleanup_job_id"}, "whatsapp_message_jobs_cleanup_job_uidx")
		table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_message_jobs_instance_fk").
			References("tenant_id", "id").
			On("whatsapp_instances").
			CascadeOnDelete()

		table.Index([]string{"tenant_id", "instance_id", "created_at", "process_id"}, "whatsapp_message_jobs_instance_idx")
		table.Index([]string{"tenant_id", "created_at", "process_id"}, "whatsapp_message_jobs_retention_idx")
	})
}

func (createMessageJobs) Down(ctx context.Context, conn hesapemigrations.Connection) error {
	return conn.Schema().DropIfExists(ctx, "whatsapp_message_jobs")
}

var (
	_ foundation.Migration                 = createMessageJobs{}
	_ hesapemigrations.ReversibleMigration = createMessageJobs{}
)
