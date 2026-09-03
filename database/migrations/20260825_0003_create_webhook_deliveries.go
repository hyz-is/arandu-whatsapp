package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// createWebhookDeliveries creates the durable snapshot owned by this package.
// The native Hesape queue owns its jobs table and migrations separately.
type createWebhookDeliveries struct{ hesapemigrations.BaseMigration }

func (createWebhookDeliveries) GetName() string {
	return "20260825_0003_create_webhook_deliveries"
}

func (createWebhookDeliveries) Up(ctx context.Context, conn hesapemigrations.Connection) error {
	return conn.Schema().Create(ctx, "whatsapp_webhook_deliveries", func(table *schema.Blueprint) {
		table.String("id", 36).Primary()
		table.String("tenant_id", 255)
		table.BigInteger("instance_id")
		table.String("event", 100)
		table.String("target", 32)
		table.String("url", 500)
		table.Text("body")
		table.Text("headers")
		table.String("status", 32)
		table.Integer("attempts")
		table.Integer("response_status").Nullable()
		table.Text("response_body").Nullable()
		table.Text("last_error").Nullable()
		table.Timestamp("created_at", microsecondPrecision)
		table.Timestamp("updated_at", microsecondPrecision)
		table.Timestamp("delivered_at", microsecondPrecision).Nullable()

		// The tenant is part of the key the child rows point at, so a delivery
		// cannot be attached to an instance belonging to another customer: the
		// engine refuses the pair rather than trusting the writer to check it.
		table.Unique([]string{"tenant_id", "id"}, "whatsapp_webhook_deliveries_tenant_id_uidx")
		table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_webhook_deliveries_instance_fk").
			References("tenant_id", "id").
			On("whatsapp_instances").
			CascadeOnDelete()

		table.Index([]string{"tenant_id", "status", "created_at", "id"}, "whatsapp_webhook_deliveries_status_idx")
		table.Index([]string{"tenant_id", "instance_id", "created_at", "id"}, "whatsapp_webhook_deliveries_instance_idx")
		table.Index([]string{"tenant_id", "created_at", "id"}, "idx_whatsapp_webhook_deliveries_retention")
	})
}

func (createWebhookDeliveries) Down(ctx context.Context, conn hesapemigrations.Connection) error {
	return conn.Schema().DropIfExists(ctx, "whatsapp_webhook_deliveries")
}

var (
	_ foundation.Migration                 = createWebhookDeliveries{}
	_ hesapemigrations.ReversibleMigration = createWebhookDeliveries{}
)
