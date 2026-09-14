package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// expandWebhookDeliveries adds identity and fenced claims in place. Each field
// is nullable or defaulted so the preceding binary can keep inserting rows.
type expandWebhookDeliveries struct{ hesapemigrations.BaseMigration }

func (expandWebhookDeliveries) GetName() string { return "20260825_0005_expand_webhook_deliveries" }

func (expandWebhookDeliveries) Up(ctx context.Context, conn hesapemigrations.Connection) error {
	return conn.Schema().Table(ctx, "whatsapp_webhook_deliveries", func(table *schema.Blueprint) {
		table.String("event_id").Nullable()
		table.String("endpoint_id").Nullable()
		table.String("secret_ref").Default("")
		table.Boolean("permanent").Default(false)
		table.String("claim_token").Nullable()
		table.BigInteger("claim_version").Default(0)
		table.Timestamp("lease_until", microsecondPrecision).Nullable()
		table.Unique([]string{"tenant_id", "event_id", "endpoint_id"}, "whatsapp_webhook_deliveries_event_endpoint_uidx")
	})
}

func (expandWebhookDeliveries) Down(ctx context.Context, conn hesapemigrations.Connection) error {
	return conn.Schema().Table(ctx, "whatsapp_webhook_deliveries", func(table *schema.Blueprint) {
		table.DropUnique("whatsapp_webhook_deliveries_event_endpoint_uidx")
		table.DropColumn("event_id", "endpoint_id", "secret_ref", "permanent", "claim_token", "claim_version", "lease_until")
	})
}

var (
	_ foundation.Migration                 = expandWebhookDeliveries{}
	_ hesapemigrations.ReversibleMigration = expandWebhookDeliveries{}
)
