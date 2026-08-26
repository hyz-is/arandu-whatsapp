package whatsapp

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/queue"

	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
)

const lifecycleTestKey = "0123456789abcdef0123456789abcdef"

func TestModuleRegistersItsNativeJobHandlers(t *testing.T) {
	sessions := security.NewSessionStore([]byte(lifecycleTestKey), time.Hour, false, security.NewMemoryBackend())
	module, err := New(Config{Tenant: "acme"}, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	worker := queue.NewWorker(queue.NullQueue{}, queue.WorkerOptions{})
	if err := module.RegisterJobHandlers(worker); err != nil {
		t.Fatal(err)
	}
	if _, ok := worker.Handler(webhooksvc.WebhookDeliveryJobName); !ok {
		t.Fatalf("handler %q was not registered", webhooksvc.WebhookDeliveryJobName)
	}
	if _, ok := worker.Handler(message.MessageProcessingJobName); !ok {
		t.Fatalf("handler %q was not registered", message.MessageProcessingJobName)
	}
	if _, ok := worker.Handler(message.MessageProcessingCleanupJobName); !ok {
		t.Fatalf("handler %q was not registered", message.MessageProcessingCleanupJobName)
	}
	if err := module.RegisterJobHandlers(worker); err == nil {
		t.Fatal("RegisterJobHandlers accepted duplicate registration")
	}
	if err := module.RegisterJobHandlers(nil); err == nil {
		t.Fatal("RegisterJobHandlers accepted a nil worker")
	}
}

func TestDeprecatedWebhookWorkerSettingsAreIgnored(t *testing.T) {
	config := Config{Tenant: "acme", Webhooks: WebhookConfig{Workers: -1, QueueSize: -1}}
	if err := config.Validate(); err != nil {
		t.Fatalf("deprecated webhook worker settings affected validation: %v", err)
	}
	defaults := (Config{Tenant: "acme"}).withDefaults()
	if defaults.Webhooks.Workers != 0 || defaults.Webhooks.QueueSize != 0 {
		t.Fatalf("deprecated webhook settings received defaults: %#v", defaults.Webhooks)
	}
	if defaults.Webhooks.Retention != DefaultWebhookRetention {
		t.Fatalf("webhook retention = %s, want %s", defaults.Webhooks.Retention, DefaultWebhookRetention)
	}
}

func TestDeprecatedMessageWorkerSettingsAreIgnored(t *testing.T) {
	config := Config{Tenant: "acme", Processing: ProcessingConfig{Workers: -1, QueueSize: -1}}
	if err := config.Validate(); err != nil {
		t.Fatalf("deprecated processing worker settings affected validation: %v", err)
	}
	defaults := config.withDefaults()
	if defaults.Processing.Workers != -1 || defaults.Processing.QueueSize != -1 {
		t.Fatalf("deprecated processing settings were rewritten: %#v", defaults.Processing)
	}
	if defaults.Processing.Retention != DefaultProcessingRetention {
		t.Fatalf("processing retention = %s, want %s", defaults.Processing.Retention, DefaultProcessingRetention)
	}
}

func TestModuleLifecycleUsesMigratedBorrowedDatabase(t *testing.T) {
	db := migratedSQLite(t, "module-lifecycle")
	handle := data.Wrap(db, data.DialectSQLite)
	sessions := security.NewSessionStore([]byte(lifecycleTestKey), time.Hour, false, security.NewMemoryBackend())
	module, err := New(Config{Tenant: "acme"}, handle, sessions)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := module.Boot(ctx); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(ctx); err != nil {
		t.Fatalf("second Start was not idempotent: %v", err)
	}
	if err := module.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if err := module.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := module.Close(ctx); err != nil {
		t.Fatalf("second Close was not idempotent: %v", err)
	}
	if err := module.Start(ctx); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Start after Close returned %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("module closed host database: %v", err)
	}
}

func TestNewAndBootNeverRunMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", "file:no-automatic-migrations?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	sessions := security.NewSessionStore([]byte(lifecycleTestKey), time.Hour, false, security.NewMemoryBackend())
	module, err := New(Config{Tenant: "acme"}, data.Wrap(db, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	if err := module.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sqliteTableExists(t, db, "whatsapp_instances") || sqliteTableExists(t, db, "whatsmeow_version") {
		t.Fatal("New or Boot ran migrations")
	}
	if err := module.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "aru migrate") {
		t.Fatalf("Start without migrations returned %v", err)
	}
	if err := module.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStartRejectsMissingWhatsMeowVersionRow(t *testing.T) {
	for _, test := range []struct {
		name    string
		version *int
	}{
		{name: "empty_table"},
		{name: "zero_version", version: new(int)},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", "file:missing-whatsmeow-version-"+test.name+"?mode=memory&cache=shared")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE whatsmeow_version (version INTEGER, compat INTEGER)`); err != nil {
				t.Fatal(err)
			}
			if test.version != nil {
				if _, err := db.Exec(`INSERT INTO whatsmeow_version (version, compat) VALUES (?, ?)`, *test.version, *test.version); err != nil {
					t.Fatal(err)
				}
			}
			sessions := security.NewSessionStore([]byte(lifecycleTestKey), time.Hour, false, security.NewMemoryBackend())
			module, err := New(Config{Tenant: "acme"}, data.Wrap(db, data.DialectSQLite), sessions)
			if err != nil {
				t.Fatal(err)
			}
			if err := module.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "aru migrate") {
				t.Fatalf("Start with %s returned %v", test.name, err)
			}
			if err := module.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
