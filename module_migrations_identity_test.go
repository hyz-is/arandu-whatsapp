package whatsapp

import (
	"context"
	"database/sql"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/migrations"
	_ "modernc.org/sqlite"
)

// A process may build the module more than once -- every test of an
// application that boots it again does. The migrations it declares are
// registered by name, and two registrations of one name have to be the same
// value, or the registry refuses them as a copied file.
func TestTwoModulesDeclareTheSameMigrations(t *testing.T) {
	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), 0, false, security.NewMemoryBackend())
	build := func(name string) *Module {
		db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		m, err := New(Config{Tenant: "realm"}, data.Wrap(db, data.DialectSQLite), sessions)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	first, second := build("identity-a").Migrations(), build("identity-b").Migrations()
	if len(first) != len(second) {
		t.Fatalf("the modules declare %d and %d migrations", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("migration %s differs between two modules", first[i].GetName())
		}
		migrations.Register(first[i], "whatsapp-identity-test")
		migrations.Register(second[i], "whatsapp-identity-test")
	}
}

// The WhatsMeow store is upgraded over the migrator's own handle when the
// module carries no upgrader.
func TestTheModulesMigrationsUpgradeTheStoreOverTheMigratorsHandle(t *testing.T) {
	db, err := sql.Open("sqlite", "file:identity-upgrade?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), 0, false, security.NewMemoryBackend())
	m, err := New(Config{Tenant: "realm"}, data.Wrap(db, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	connection := database.ForMigrations(database.NewConnection(db, "", "", map[string]any{
		"driver": string(database.DialectSQLite), "name": "default",
	}))
	for _, migration := range m.Migrations() {
		if err := migration.Up(context.Background(), connection); err != nil {
			t.Fatalf("apply %s: %v", migration.GetName(), err)
		}
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM whatsmeow_version LIMIT 1`).Scan(&version); err != nil || version < 1 {
		t.Fatalf("the WhatsMeow store is at version %d (%v), want an applied version", version, err)
	}
}
