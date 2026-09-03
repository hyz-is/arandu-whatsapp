package migrations

import (
	"context"

	"github.com/arandu-io/framework/foundation"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// createWhatsAppTables creates the final tenant-scoped domain schema.
type createWhatsAppTables struct{ hesapemigrations.BaseMigration }

func (createWhatsAppTables) GetName() string {
	return "20260825_0001_create_whatsapp_tables"
}

// Up creates the eight tables the package owns, parents before children.
//
// Every child carries tenant_id and points at the parent through it, so a row
// cannot be attached across customers even when the writer forgets to filter:
// the engine has no matching parent key to accept. That is why the parents
// carry a unique key on (tenant_id, id) beside their own primary key, and why
// the foreign keys are composite rather than on the identifier alone.
func (createWhatsAppTables) Up(ctx context.Context, conn hesapemigrations.Connection) error {
	create := []struct {
		table string
		build func(*schema.Blueprint)
	}{
		{"whatsapp_instances", instancesTable},
		{"whatsapp_messages", messagesTable},
		{"whatsapp_message_updates", messageUpdatesTable},
		{"whatsapp_chats", chatsTable},
		{"whatsapp_contacts", contactsTable},
		{"whatsapp_webhooks", webhooksTable},
		{"whatsapp_instance_connections", instanceConnectionsTable},
		{"whatsapp_address_mappings", addressMappingsTable},
	}
	for _, definition := range create {
		if err := conn.Schema().Create(ctx, definition.table, definition.build); err != nil {
			return err
		}
	}
	return nil
}

func instancesTable(table *schema.Blueprint) {
	table.BigInteger("id").Primary()
	table.String("tenant_id", 255)
	table.String("name", 255)
	table.String("description", 255).Nullable()
	table.String("status", 32)
	table.String("owner_jid", 100).Nullable()
	table.String("profile_pic_url", 500).Nullable()
	table.Text("external_attributes")
	table.String("connection_lock_owner", 255).Nullable()
	table.Timestamp("connection_lock_until", microsecondPrecision).Nullable()
	table.Timestamp("created_at", microsecondPrecision)
	table.Timestamp("updated_at", microsecondPrecision)

	// The composite key every child points at. Without it the children could
	// only reference the identifier, and a row would be reachable from another
	// customer's tenant.
	table.Unique([]string{"tenant_id", "id"}, "whatsapp_instances_tenant_id_uidx")
	table.Unique([]string{"tenant_id", "name"}, "whatsapp_instances_tenant_name_uidx")

	table.Index([]string{"tenant_id", "created_at", "id"}, "whatsapp_instances_tenant_created_idx")
	table.Index([]string{"tenant_id", "status", "id"}, "whatsapp_instances_tenant_status_idx")
}

func messagesTable(table *schema.Blueprint) {
	table.BigInteger("id").Primary()
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.String("key_id", 100)
	table.String("key_remote_jid", 100).Nullable()
	table.String("key_lid", 100).Nullable()
	table.Boolean("key_from_me")
	table.String("key_participant", 100).Nullable()
	table.String("key_participant_lid", 100).Nullable()
	table.String("push_name", 100).Nullable()
	table.String("message_type", 100)
	table.Text("content")
	table.BigInteger("message_timestamp")
	table.String("device", 32)
	table.Boolean("is_group").Nullable()
	table.Text("metadata").Nullable()
	table.Timestamp("created_at", microsecondPrecision)

	// The key the status history points at, which is why it is unique rather
	// than an index.
	table.Unique([]string{"tenant_id", "instance_id", "id"}, "whatsapp_messages_tenant_instance_id_uidx")
	table.Unique([]string{"tenant_id", "instance_id", "key_id"}, "whatsapp_messages_tenant_instance_key_uidx")
	table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_messages_instance_fk").
		References("tenant_id", "id").
		On("whatsapp_instances").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "instance_id", "id"}, "whatsapp_messages_tenant_instance_id_idx")
	table.Index([]string{"tenant_id", "instance_id", "key_remote_jid", "id"}, "whatsapp_messages_tenant_remote_idx")
	table.Index([]string{"tenant_id", "instance_id", "key_from_me", "id"}, "whatsapp_messages_tenant_from_me_idx")
	table.Index([]string{"tenant_id", "instance_id", "message_type", "id"}, "whatsapp_messages_tenant_type_idx")
	table.Index([]string{"tenant_id", "instance_id", "device", "id"}, "whatsapp_messages_tenant_device_idx")
	table.Index([]string{"tenant_id", "instance_id", "message_timestamp", "id"}, "whatsapp_messages_tenant_timestamp_idx")
}

func messageUpdatesTable(table *schema.Blueprint) {
	table.BigInteger("id").Primary()
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.BigInteger("message_id")
	table.Timestamp("date_time", microsecondPrecision)
	table.String("status", 30)

	table.Unique([]string{"tenant_id", "instance_id", "message_id", "status", "date_time"}, "whatsapp_updates_message_status_uidx")
	table.Foreign([]string{"tenant_id", "instance_id", "message_id"}, "whatsapp_updates_message_fk").
		References("tenant_id", "instance_id", "id").
		On("whatsapp_messages").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "instance_id", "message_id", "status", "id"}, "whatsapp_updates_message_status_idx")
	table.Index([]string{"tenant_id", "instance_id", "message_id", "date_time", "id"}, "whatsapp_updates_message_time_idx")
}

func chatsTable(table *schema.Blueprint) {
	table.BigInteger("id").Primary()
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.String("remote_jid", 100)
	table.Text("content").Nullable()
	table.Timestamp("created_at", microsecondPrecision)
	table.Timestamp("updated_at", microsecondPrecision)

	table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_chats_instance_fk").
		References("tenant_id", "id").
		On("whatsapp_instances").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "instance_id", "created_at", "id"}, "whatsapp_chats_tenant_created_idx")
}

func contactsTable(table *schema.Blueprint) {
	table.BigInteger("id").Primary()
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.String("remote_jid", 100)
	table.String("push_name", 100).Nullable()
	table.String("profile_pic_url", 500).Nullable()
	table.Timestamp("created_at", microsecondPrecision)
	table.Timestamp("updated_at", microsecondPrecision)

	table.Unique([]string{"tenant_id", "instance_id", "remote_jid"}, "whatsapp_contacts_tenant_remote_uidx")
	table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_contacts_instance_fk").
		References("tenant_id", "id").
		On("whatsapp_instances").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "instance_id", "created_at", "id"}, "whatsapp_contacts_tenant_created_idx")
}

func webhooksTable(table *schema.Blueprint) {
	table.BigInteger("id").Primary()
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.String("url", 500)
	table.Boolean("enabled")
	table.Text("events")
	table.Timestamp("created_at", microsecondPrecision)
	table.Timestamp("updated_at", microsecondPrecision)

	// One configuration per instance, held by the engine rather than by the
	// service that writes it.
	table.Unique([]string{"tenant_id", "instance_id"}, "whatsapp_webhooks_tenant_instance_uidx")
	table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_webhooks_instance_fk").
		References("tenant_id", "id").
		On("whatsapp_instances").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "enabled", "id"}, "whatsapp_webhooks_tenant_enabled_idx")
}

func instanceConnectionsTable(table *schema.Blueprint) {
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.String("connection_status", 64)
	table.String("whatsapp_device_jid", 100).Nullable()
	table.String("whatsapp_owner_jid", 100).Nullable()
	table.String("whatsapp_phone_number", 32).Nullable()
	table.String("profile_pic_id", 255).Nullable()
	table.Timestamp("last_connected_at", microsecondPrecision).Nullable()
	table.Timestamp("last_disconnected_at", microsecondPrecision).Nullable()
	table.Timestamp("last_connection_attempt_at", microsecondPrecision).Nullable()
	table.String("last_connection_error", 255).Nullable()
	table.String("last_connection_event", 100).Nullable()
	table.BigInteger("connection_attempts")
	table.Timestamp("created_at", microsecondPrecision)
	table.Timestamp("updated_at", microsecondPrecision)

	table.Primary([]string{"tenant_id", "instance_id"})
	// A linked device belongs to one instance across every customer, which is
	// what stops the same phone being paired twice.
	table.Unique([]string{"whatsapp_device_jid"}, "whatsapp_connections_device_jid_uidx")
	table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_connections_instance_fk").
		References("tenant_id", "id").
		On("whatsapp_instances").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "connection_status", "instance_id"}, "whatsapp_connections_tenant_status_idx")
}

func addressMappingsTable(table *schema.Blueprint) {
	table.String("tenant_id", 255)
	table.BigInteger("instance_id")
	table.String("alias", 100)
	table.String("normalized_phone", 32)
	table.String("canonical_jid", 100)
	table.String("lid_jid", 100).Nullable()
	table.Timestamp("resolved_at", microsecondPrecision)
	table.Timestamp("expires_at", microsecondPrecision)
	table.Timestamp("created_at", microsecondPrecision)
	table.Timestamp("updated_at", microsecondPrecision)

	table.Primary([]string{"tenant_id", "instance_id", "alias"})
	table.Foreign([]string{"tenant_id", "instance_id"}, "whatsapp_address_mappings_instance_fk").
		References("tenant_id", "id").
		On("whatsapp_instances").
		CascadeOnDelete()

	table.Index([]string{"tenant_id", "instance_id", "canonical_jid"}, "whatsapp_address_mappings_canonical_idx")
	table.Index([]string{"tenant_id", "instance_id", "expires_at"}, "whatsapp_address_mappings_expires_idx")
}

// Down drops the tables children first, so no drop is refused by a foreign key
// still pointing at the table being removed.
func (createWhatsAppTables) Down(ctx context.Context, conn hesapemigrations.Connection) error {
	for _, table := range []string{
		"whatsapp_address_mappings",
		"whatsapp_instance_connections",
		"whatsapp_webhooks",
		"whatsapp_contacts",
		"whatsapp_chats",
		"whatsapp_message_updates",
		"whatsapp_messages",
		"whatsapp_instances",
	} {
		if err := conn.Schema().DropIfExists(ctx, table); err != nil {
			return err
		}
	}
	return nil
}

var (
	_ foundation.Migration                 = createWhatsAppTables{}
	_ hesapemigrations.ReversibleMigration = createWhatsAppTables{}
)
