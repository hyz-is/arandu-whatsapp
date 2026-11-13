package whatsapp

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/arandu-io/hesape/database"
	_ "github.com/lib/pq"

	packagemigrations "github.com/hyz-is/arandu-whatsapp/database/migrations"
)

// postgresTestEnv names a PostgreSQL database the test may create and drop
// schemas in, for example
// postgres://user:pass@127.0.0.1:5432/scratch?sslmode=disable. Without it the
// test is skipped: the module's other tests run on SQLite.
const postgresTestEnv = "WHATSAPP_TEST_POSTGRES_URL"

// TestMigrationsHonourTheConnectionSchemaOnPostgreSQL proves the WhatsMeow store
// is created in the schema the connection's search_path names, even when a
// table called whatsmeow_version lives in another schema of the same database.
// WhatsMeow's own upgrader asks information_schema by table name alone, so it
// used to find that table, skip creating its own and fail reading it.
func TestMigrationsHonourTheConnectionSchemaOnPostgreSQL(t *testing.T) {
	raw := os.Getenv(postgresTestEnv)
	if raw == "" {
		t.Skipf("%s is not set", postgresTestEnv)
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	other := "wa_other_" + hex.EncodeToString(suffix)
	own := "wa_own_" + hex.EncodeToString(suffix)
	for _, schema := range []string{other, own} {
		if _, err := admin.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %q", schema)); err != nil {
			t.Fatal(err)
		}
		schema := schema
		t.Cleanup(func() {
			_, _ = admin.ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %q CASCADE", schema))
		})
	}
	// A WhatsMeow version table with its compat column in the other schema: the
	// answer information_schema gives for the name, whichever schema is asked.
	if _, err := admin.ExecContext(ctx, fmt.Sprintf(
		`CREATE TABLE %q.whatsmeow_version (version INTEGER, compat INTEGER)`, other)); err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", own)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	connection := database.ForMigrations(database.NewConnection(db, "", "", map[string]any{
		"driver": string(database.DialectPostgres),
		"name":   migrationConnectionName,
	}))
	for _, migration := range packagemigrations.Migrations(nil) {
		if err := migration.Up(ctx, connection); err != nil {
			t.Fatalf("apply %s: %v", migration.GetName(), err)
		}
	}

	var version int
	if err := db.QueryRowContext(ctx, `SELECT version FROM whatsmeow_version LIMIT 1`).Scan(&version); err != nil {
		t.Fatalf("the version table is not in the connection's schema: %v", err)
	}
	if version < 1 {
		t.Fatalf("WhatsMeow store version = %d, want an applied version", version)
	}
	var devices int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'whatsmeow_device'`, own).Scan(&devices); err != nil || devices != 1 {
		t.Fatalf("whatsmeow_device in schema %s: count %d, err %v", own, devices, err)
	}
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'whatsmeow_device'`, other).Scan(&devices); err != nil || devices != 0 {
		t.Fatalf("whatsmeow_device leaked into schema %s: count %d, err %v", other, devices, err)
	}
}
