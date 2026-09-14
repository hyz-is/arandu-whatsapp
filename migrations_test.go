package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/migrations"
	"go.mau.fi/whatsmeow/store/sqlstore"
	_ "modernc.org/sqlite"

	packagemigrations "github.com/hyz-is/arandu-whatsapp/database/migrations"
)

const migrationConnectionName = "default"

func TestMigrationsRunThroughHesapeMigratorAndRollbackOwnedSchema(t *testing.T) {
	db, err := sql.Open("sqlite", "file:arandu-whatsapp-migrations?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	declared := packagemigrations.Migrations(container)
	wantNames := []string{
		"20260825_0001_create_whatsapp_tables",
		"20260825_0002_upgrade_whatsmeow_store",
		"20260825_0003_create_webhook_deliveries",
		"20260825_0004_create_message_jobs",
		"20260825_0005_expand_webhook_deliveries",
	}
	if got := migrationNames(declared); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("migration names = %v, want %v", got, wantNames)
	}
	if _, ok := declared[0].(migrations.ReversibleMigration); !ok {
		t.Fatal("the package-owned schema migration is not reversible")
	}
	if _, ok := declared[1].(migrations.ReversibleMigration); ok {
		t.Fatal("the delegated WhatsMeow migration falsely declares a Down method")
	}
	if _, ok := declared[2].(migrations.ReversibleMigration); !ok {
		t.Fatal("the webhook delivery migration is not reversible")
	}
	if _, ok := declared[3].(migrations.ReversibleMigration); !ok {
		t.Fatal("the message job migration is not reversible")
	}
	if _, ok := declared[4].(migrations.ReversibleMigration); !ok {
		t.Fatal("the webhook delivery expansion is not reversible")
	}
	if declared[1].WithinTransaction() {
		t.Fatal("the delegated WhatsMeow migration must run outside Arandu's transaction")
	}

	connection := database.NewConnection(db, "", "", map[string]any{
		"driver": string(database.DialectSQLite),
		"name":   migrationConnectionName,
	})
	connections := database.NewConnectionResolver(map[string]database.ConnectionInterface{
		migrationConnectionName: connection,
	})
	connections.SetDefaultConnection(migrationConnectionName)
	resolver := database.MigrationResolver{Resolver: connections}
	repository := migrations.NewDatabaseMigrationRepository(resolver, "arandu_whatsapp_migration_test")
	if err := repository.CreateRepository(ctx); err != nil {
		t.Fatalf("create migration repository: %v", err)
	}
	migrator := migrations.NewMigrator(repository, resolver, nil)
	migrator.SetConnection(migrationConnectionName)
	const migrationTestPath = "database/migrations/arandu-whatsapp-test"
	for _, migration := range declared {
		migrations.Register(migration, migrationTestPath)
	}
	if err := migrator.RunPending(ctx, declared, migrations.Options{Step: true}); err != nil {
		t.Fatalf("run pending migrations: %v", err)
	}

	for _, table := range []string{
		"whatsapp_instances", "whatsapp_instance_connections", "whatsapp_messages",
		"whatsapp_message_updates", "whatsapp_chats", "whatsapp_contacts",
		"whatsapp_webhooks", "whatsapp_webhook_deliveries", "whatsapp_message_jobs",
		"whatsapp_address_mappings", "whatsmeow_device",
		"whatsmeow_retry_buffer", "whatsmeow_version",
	} {
		if !sqliteTableExists(t, db, table) {
			t.Errorf("table %s was not created", table)
		}
	}

	var version int
	if err := db.QueryRow(`SELECT version FROM whatsmeow_version LIMIT 1`).Scan(&version); err != nil {
		t.Fatalf("read WhatsMeow schema version: %v", err)
	}
	if version < 1 {
		t.Fatalf("WhatsMeow schema version = %d, want a migrated schema", version)
	}

	now := time.Now().UTC()
	insertInstance := `INSERT INTO whatsapp_instances
	        (id, tenant_id, name, status, external_attributes, created_at, updated_at)
	        VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := db.Exec(insertInstance, 1, "acme", "shared", "ONLINE", `{}`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insertInstance, 2, "globex", "shared", "ONLINE", `{}`, now, now); err != nil {
		t.Fatalf("same name in another tenant was rejected: %v", err)
	}
	if _, err := db.Exec(insertInstance, 3, "acme", "shared", "ONLINE", `{}`, now, now); err == nil {
		t.Fatal("duplicate name in one tenant was accepted")
	}
	if _, err := db.Exec(`INSERT INTO whatsapp_messages
	        (id, tenant_id, instance_id, key_id, key_from_me, message_type, content,
	         message_timestamp, device, metadata, created_at)
	        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		10, "globex", 1, "cross-tenant", true, "conversation", `{}`, 1, "web", `{}`, now); err == nil {
		t.Fatal("cross-tenant child foreign key was accepted")
	}
	if _, err := db.Exec(`INSERT INTO whatsapp_webhook_deliveries
		(id, tenant_id, instance_id, event, target, url, body, headers, status,
		 attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "delivery-1", "globex", 1,
		"connection.update", "instance", "https://example.com/hook", `{}`, `{}`,
		"pending", 0, now, now); err == nil {
		t.Fatal("cross-tenant webhook delivery foreign key was accepted")
	}
	if _, err := db.Exec(`INSERT INTO whatsapp_webhook_deliveries
		(id, tenant_id, instance_id, event, target, url, body, headers, status,
		 attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "legacy-delivery", "acme", 1,
		"connection.update", "instance", "https://example.com/hook", `{}`, `{}`,
		"pending", 0, now, now); err != nil {
		t.Fatalf("preceding binary INSERT failed after 0005: %v", err)
	}
	var legacyEvent, legacyEndpoint sql.NullString
	var secretRef string
	var permanent bool
	var claimVersion int64
	if err := db.QueryRow(`SELECT event_id, endpoint_id, secret_ref, permanent, claim_version
		FROM whatsapp_webhook_deliveries WHERE id = ?`, "legacy-delivery").Scan(
		&legacyEvent, &legacyEndpoint, &secretRef, &permanent, &claimVersion); err != nil {
		t.Fatal(err)
	}
	if legacyEvent.Valid || legacyEndpoint.Valid || secretRef != "" || permanent || claimVersion != 0 {
		t.Fatalf("legacy defaults event=%v endpoint=%v secret=%q permanent=%v claim=%d",
			legacyEvent, legacyEndpoint, secretRef, permanent, claimVersion)
	}
	if _, err := db.Exec(`INSERT INTO whatsapp_message_jobs
		(process_id, message_job_id, cleanup_job_id, tenant_id, instance_id,
		 instance_name, remote_jid, message_id,
		 message_type, message_payload, content, delay_ms, external_attributes,
		 webhook_instance, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"process-1", "message-job-1", "cleanup-job-1", "globex", 1,
		"shared", "120363000000000000@g.us", "message-1", "extendedTextMessage",
		"payload", `{}`, 0, `{}`, `{}`, now, now); err == nil {
		t.Fatal("cross-tenant message job foreign key was accepted")
	}

	rolledBack, err := migrator.Rollback(ctx, []string{migrationTestPath}, migrations.Options{})
	if err != nil {
		t.Fatalf("rollback latest migration batch: %v", err)
	}
	if !reflect.DeepEqual(rolledBack, []string{"20260825_0005_expand_webhook_deliveries"}) {
		t.Fatalf("rolled back migrations = %v, want only the latest reversible migration", rolledBack)
	}
	if sqliteColumnExists(t, db, "whatsapp_webhook_deliveries", "event_id") {
		t.Fatal("official rollback left the delivery expansion behind")
	}
	if !sqliteTableExists(t, db, "whatsapp_message_jobs") || !sqliteTableExists(t, db, "whatsapp_webhook_deliveries") || !sqliteTableExists(t, db, "whatsmeow_device") {
		t.Fatal("rolling back the delivery expansion crossed into an earlier migration")
	}
	ran, err := repository.GetRan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(ran, wantNames) || containsMigration(ran, "20260825_0005_expand_webhook_deliveries") {
		t.Fatalf("migration tracking still contains rolled back delivery expansion: %v", ran)
	}

	migrationConnection, err := resolver.Connection("")
	if err != nil {
		t.Fatalf("resolve migration connection: %v", err)
	}
	messageJobsSchema := declared[3].(migrations.ReversibleMigration)
	if err := messageJobsSchema.Down(ctx, migrationConnection); err != nil {
		t.Fatalf("rollback %s: %v", messageJobsSchema.GetName(), err)
	}
	deliveriesSchema := declared[2].(migrations.ReversibleMigration)
	if err := deliveriesSchema.Down(ctx, migrationConnection); err != nil {
		t.Fatalf("rollback %s: %v", deliveriesSchema.GetName(), err)
	}
	if sqliteTableExists(t, db, "whatsapp_webhook_deliveries") {
		t.Fatal("rollback left the webhook delivery schema behind")
	}
	ownedSchema := declared[0].(migrations.ReversibleMigration)
	if err := ownedSchema.Down(ctx, migrationConnection); err != nil {
		t.Fatalf("rollback %s: %v", ownedSchema.GetName(), err)
	}
	if sqliteTableExists(t, db, "whatsapp_instances") {
		t.Fatal("rollback left the package-owned schema behind")
	}
	if !sqliteTableExists(t, db, "whatsmeow_device") {
		t.Fatal("rolling back the package-owned schema removed the upstream WhatsMeow schema")
	}
}

func TestWebhookDeliveryExpansionRendersForPostgres(t *testing.T) {
	db, err := sql.Open("sqlite", "file:postgres-webhook-migration-render?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	connection := database.ForMigrations(database.NewConnection(db, "", "", map[string]any{
		"driver": string(database.DialectPostgres), "name": migrationConnectionName,
	}))
	pretender := connection.(migrations.PretendingConnection)
	migration := packagemigrations.Migrations(nil)[4]
	statements, err := pretender.Pretend(context.Background(), func() error {
		return migration.Up(context.Background(), connection)
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ToLower(strings.Join(statements, "\n"))
	for _, fragment := range []string{`add column "event_id"`, `add column "claim_version"`, "whatsapp_webhook_deliveries_event_endpoint_uidx"} {
		if !strings.Contains(joined, strings.ToLower(fragment)) {
			t.Errorf("PostgreSQL SQL missing %q: %s", fragment, joined)
		}
	}
}

func containsMigration(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func TestWhatsMeowMigrationDelegatesAndPropagatesErrors(t *testing.T) {
	boom := errors.New("boom")
	called := 0
	upgrader := schemaUpgraderFunc(func(context.Context) error {
		called++
		return boom
	})
	migration := packagemigrations.Migrations(upgrader)[1]
	if err := migration.Up(context.Background(), nil); !errors.Is(err, boom) {
		t.Fatalf("Up error = %v, want wrapped boom", err)
	}
	if called != 1 {
		t.Fatalf("upgrader called %d times, want 1", called)
	}
	if err := packagemigrations.Migrations(nil)[1].Up(context.Background(), nil); err == nil {
		t.Fatal("migration accepted a missing WhatsMeow schema upgrader")
	}
}

func TestWhatsMeowMigrationDoesNotUpgradeDuringPretend(t *testing.T) {
	db, err := sql.Open("sqlite", "file:arandu-whatsapp-migrations-pretend?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	connection := database.ForMigrations(database.NewConnection(db, "", "", map[string]any{
		"driver": string(database.DialectSQLite),
		"name":   migrationConnectionName,
	}))
	pretender, ok := connection.(migrations.PretendingConnection)
	if !ok {
		t.Fatal("Hesape migration connection does not support pretend mode")
	}
	called := 0
	migration := packagemigrations.Migrations(schemaUpgraderFunc(func(context.Context) error {
		called++
		return nil
	}))[1]
	if _, err := pretender.Pretend(context.Background(), func() error {
		return migration.Up(context.Background(), connection)
	}); err != nil {
		t.Fatalf("pretend migration: %v", err)
	}
	if called != 0 {
		t.Fatalf("upgrader ran %d times during pretend, want 0", called)
	}
}

type schemaUpgraderFunc func(context.Context) error

func (f schemaUpgraderFunc) Upgrade(ctx context.Context) error { return f(ctx) }

func migrationNames(declared []foundation.Migration) []string {
	names := make([]string, len(declared))
	for index, migration := range declared {
		names[index] = migration.GetName()
	}
	return names
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	return count == 1
}

func sqliteColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var sequence, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	return false
}
