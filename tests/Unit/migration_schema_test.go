package unit_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/database"
	hesapemigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"

	packagemigrations "github.com/hyz-is/arandu-whatsapp/database/migrations"
)

// postgresIdentifierLimit is the number of bytes Postgres keeps of a relation
// or constraint name. A longer one is truncated with a notice rather than
// refused, so two names that differ only past the limit become one object and
// the second statement fails on a name nobody wrote.
const postgresIdentifierLimit = 63

// noSchemaUpgrade stands in for the WhatsMeow store upgrader, which owns its
// own schema and issues no DDL through the Blueprint.
type noSchemaUpgrade struct{}

func (noSchemaUpgrade) Upgrade(context.Context) error { return nil }

// recordingSchemaConnection is the schema connection the Blueprint compiles
// against: it keeps the statements instead of running them, so the DDL of every
// dialect can be read without a server of that dialect.
type recordingSchemaConnection struct {
	schema.Connection
	statements *[]string
}

func (c recordingSchemaConnection) Statement(_ context.Context, query string) error {
	*c.statements = append(*c.statements, query)
	return nil
}

// recordingMigrationConnection hands migrations a Blueprint over the recorder
// and keeps whatever they send through the raw statement escape as well.
type recordingMigrationConnection struct {
	hesapemigrations.Connection
	builder    *schema.Builder
	statements *[]string
}

func (c recordingMigrationConnection) Schema() *schema.Builder { return c.builder }

func (c recordingMigrationConnection) Statement(_ context.Context, query string, _ []any) (bool, error) {
	*c.statements = append(*c.statements, query)
	return true, nil
}

// TestMigrationDDLKeepsEveryIdentifierWithinThePostgresLimit reads the compiled
// DDL rather than the source, because the names that overflow are the ones the
// Blueprint derives from a table and its columns rather than the ones somebody
// typed.
func TestMigrationDDLKeepsEveryIdentifierWithinThePostgresLimit(t *testing.T) {
	t.Parallel()

	quoted := regexp.MustCompile(`"([^"]+)"`)
	for _, statement := range compiledDDL(t, "pgsql") {
		for _, match := range quoted.FindAllStringSubmatch(statement, -1) {
			if len(match[1]) > postgresIdentifierLimit {
				t.Errorf("identifier %q is %d bytes, over the %d Postgres keeps", match[1], len(match[1]), postgresIdentifierLimit)
			}
		}
	}
}

// TestMigrationDDLKeepsMicrosecondTimestamps holds the precision the schema was
// created with. The Blueprint's default is whole seconds, and
// whatsapp_message_updates has date_time in a unique key: at that granularity
// two transitions of one message into the same status inside one second collide
// and the second is refused instead of recorded.
func TestMigrationDDLKeepsMicrosecondTimestamps(t *testing.T) {
	t.Parallel()

	found := 0
	for _, statement := range compiledDDL(t, "pgsql") {
		for _, occurrence := range regexp.MustCompile(`timestamp\(\d+\)`).FindAllString(statement, -1) {
			found++
			if occurrence != "timestamp(6)" {
				t.Errorf("a timestamp column is declared %s, which truncates what the schema stores", occurrence)
			}
		}
		if strings.Contains(statement, " timestamp without time zone") {
			t.Errorf("a timestamp column carries no explicit precision: %s", statement)
		}
	}
	if found == 0 {
		t.Fatal("no timestamp column was compiled, so this test proves nothing")
	}
}

// TestEveryForeignKeyCarriesTheTenant is the schema half of the tenant
// property. A child pointing at its parent by identifier alone is reachable
// from another customer's row as soon as two customers hold the same
// identifier; pointing through tenant_id leaves the engine no matching parent
// key to accept.
func TestEveryForeignKeyCarriesTheTenant(t *testing.T) {
	t.Parallel()

	for _, dialect := range []string{"pgsql", "sqlite"} {
		keys := 0
		for _, statement := range compiledDDL(t, dialect) {
			for _, clause := range regexp.MustCompile(`foreign key\s*\(([^)]*)\)`).FindAllStringSubmatch(statement, -1) {
				keys++
				if !strings.Contains(clause[1], "tenant_id") {
					t.Errorf("%s: foreign key (%s) does not carry the tenant", dialect, clause[1])
				}
				if !strings.Contains(statement, "on delete cascade") {
					t.Errorf("%s: foreign key (%s) does not cascade, so deleting an instance leaves its rows behind", dialect, clause[1])
				}
			}
		}
		if keys == 0 {
			t.Fatalf("%s: no foreign key was compiled, so this test proves nothing", dialect)
		}
	}
}

// compiledDDL returns every statement the declared migrations issue on dialect,
// in order, without running any of them.
func compiledDDL(t *testing.T, dialect string) []string {
	t.Helper()

	statements := []string{}
	connection := recordingConnection(t, dialect, &statements)
	for _, migration := range packagemigrations.Migrations(noSchemaUpgrade{}) {
		if err := migration.Up(context.Background(), connection); err != nil {
			t.Fatalf("%s: compiling %s: %v", dialect, migration.GetName(), err)
		}
	}
	if len(statements) == 0 {
		t.Fatalf("%s: the migrations issued no statement", dialect)
	}
	return statements
}

// recordingConnection builds a migration connection that compiles for dialect
// and runs nothing. The database handle is nil on purpose: a statement that
// escaped the recorder would panic rather than reach a server.
func recordingConnection(t *testing.T, dialect string, statements *[]string) hesapemigrations.Connection {
	t.Helper()

	underlying := database.ForMigrations(database.NewConnection(nil, "", "", map[string]any{
		"driver": dialect,
		"name":   "default",
	}))
	return recordingMigrationConnection{
		Connection: underlying,
		builder: schema.NewBuilder(recordingSchemaConnection{
			Connection: underlying.Schema().GetConnection(),
			statements: statements,
		}),
		statements: statements,
	}
}
