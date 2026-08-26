package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/arandu-io/framework/data"
	_ "modernc.org/sqlite"
)

func TestNewIDFromUsesTheFullPositive63BitRange(t *testing.T) {
	maximum, err := newIDFrom(bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}))
	if err != nil {
		t.Fatal(err)
	}
	if maximum != math.MaxInt64 {
		t.Fatalf("maximum identifier = %d, want %d", maximum, int64(math.MaxInt64))
	}

	zeroThenOne := append(make([]byte, 8), 0, 0, 0, 0, 0, 0, 0, 1)
	minimum, err := newIDFrom(bytes.NewReader(zeroThenOne))
	if err != nil {
		t.Fatal(err)
	}
	if minimum != 1 {
		t.Fatalf("minimum identifier = %d, want 1", minimum)
	}

	if _, err := newIDFrom(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("reader error = %v, want EOF", err)
	}
}

func TestNewIDGeneratesDistinctPositiveIdentifiers(t *testing.T) {
	const samples = 4096
	seen := make(map[int64]struct{}, samples)
	usesMoreThan31Bits := false
	for range samples {
		id, err := newID()
		if err != nil {
			t.Fatal(err)
		}
		if id <= 0 {
			t.Fatalf("identifier = %d, want a positive value", id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate identifier generated: %d", id)
		}
		if id > math.MaxInt32 {
			usesMoreThan31Bits = true
		}
		seen[id] = struct{}{}
	}
	if !usesMoreThan31Bits {
		t.Fatal("all generated identifiers were limited to 31 bits")
	}
}

func TestCreateErrorsOnlyMapSemanticUniqueConstraints(t *testing.T) {
	db, err := sql.Open("sqlite", "file:identifier-errors?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	statements := []string{
		`CREATE TABLE whatsapp_instances (
			id BIGINT NOT NULL PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			UNIQUE (tenant_id, name)
		)`,
		`CREATE TABLE whatsapp_webhooks (
			id BIGINT NOT NULL PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			instance_id BIGINT NOT NULL,
			UNIQUE (tenant_id, instance_id)
		)`,
		`INSERT INTO whatsapp_instances (id, tenant_id, name) VALUES (10, 'acme', 'primary')`,
		`INSERT INTO whatsapp_webhooks (id, tenant_id, instance_id) VALUES (20, 'acme', 10)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	base := NewBase(data.Wrap(db, data.DialectSQLite))
	instances := NewInstanceRepository(base)
	webhooks := NewWebhookRepository(base)
	ctx := context.Background()

	_, duplicateName := db.Exec(`INSERT INTO whatsapp_instances (id, tenant_id, name) VALUES (11, 'acme', 'primary')`)
	if got := instances.createError(ctx, "acme", "primary", duplicateName); !errors.Is(got, ErrInstanceNameAlreadyExists) {
		t.Fatalf("duplicate name mapped to %v", got)
	}
	_, duplicateInstanceID := db.Exec(`INSERT INTO whatsapp_instances (id, tenant_id, name) VALUES (10, 'acme', 'different')`)
	if got := instances.createError(ctx, "acme", "different", duplicateInstanceID); errors.Is(got, ErrInstanceNameAlreadyExists) {
		t.Fatalf("primary-key collision mapped as a duplicate name: %v", got)
	}

	_, duplicateWebhook := db.Exec(`INSERT INTO whatsapp_webhooks (id, tenant_id, instance_id) VALUES (21, 'acme', 10)`)
	if got := webhooks.createError(ctx, "acme", 10, duplicateWebhook); !errors.Is(got, ErrWebhookAlreadyExists) {
		t.Fatalf("duplicate webhook mapped to %v", got)
	}
	_, duplicateWebhookID := db.Exec(`INSERT INTO whatsapp_webhooks (id, tenant_id, instance_id) VALUES (20, 'acme', 11)`)
	if got := webhooks.createError(ctx, "acme", 11, duplicateWebhookID); errors.Is(got, ErrWebhookAlreadyExists) {
		t.Fatalf("webhook primary-key collision mapped as an existing configuration: %v", got)
	}
}
