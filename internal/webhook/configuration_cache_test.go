package webhook

import (
	"context"
	"testing"
	"time"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/cache"
	hlog "github.com/arandu-io/hesape/log"

	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// countingConfigurationRepository counts how often a dispatch reached the
// database for the configuration it is about to act on.
type countingConfigurationRepository struct {
	deliveryRepository
	reads int
}

func (r *countingConfigurationRepository) FindConfiguration(ctx context.Context, grant security.Grant, instanceID int64) (types.Webhook, error) {
	r.reads++
	return r.deliveryRepository.FindConfiguration(ctx, grant, instanceID)
}

func TestDispatchReadsTheConfigurationOncePerCacheWindow(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})

	manager := newTestManager(t, testDB.db, ManagerConfig{
		ConfigurationCache:    cache.New(cache.NewArrayStore()),
		ConfigurationCacheTTL: time.Minute,
	})
	counter := &countingConfigurationRepository{deliveryRepository: manager.repository}
	manager.repository = counter

	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-cache"))
	for range 3 {
		if err := manager.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated,
			ConnectionWebhookData{Type: "connected", Connection: "open"}); err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
	}

	if counter.reads != 1 {
		t.Fatalf("configuration reads = %d, want 1", counter.reads)
	}
	if count := testDB.count(t, "whatsapp_webhook_deliveries"); count != 3 {
		t.Fatalf("deliveries = %d, want 3: caching the configuration must not drop an event", count)
	}
}

func TestDispatchReadsTheConfigurationEveryTimeWithoutACache(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})

	manager := newTestManager(t, testDB.db, ManagerConfig{})
	counter := &countingConfigurationRepository{deliveryRepository: manager.repository}
	manager.repository = counter
	manager.configCache = configurationCache{}

	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-nocache"))
	for range 3 {
		if err := manager.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated,
			ConnectionWebhookData{Type: "connected", Connection: "open"}); err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
	}

	if counter.reads != 3 {
		t.Fatalf("configuration reads = %d, want 3", counter.reads)
	}
}

func TestDispatchCachesTheAbsenceOfAConfiguration(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)

	manager := newTestManager(t, testDB.db, ManagerConfig{
		GlobalURL: "https://global.example/hook", GlobalEnabled: true,
		ConfigurationCache:    cache.New(cache.NewArrayStore()),
		ConfigurationCacheTTL: time.Minute,
	})
	counter := &countingConfigurationRepository{deliveryRepository: manager.repository}
	manager.repository = counter

	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-absent"))
	for range 3 {
		if err := manager.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated,
			ConnectionWebhookData{Type: "connected", Connection: "open"}); err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
	}

	if counter.reads != 1 {
		t.Fatalf("configuration reads = %d, want 1: the absent configuration was read again", counter.reads)
	}
	if count := testDB.count(t, "whatsapp_webhook_deliveries"); count != 3 {
		t.Fatalf("global deliveries = %d, want 3", count)
	}
}

func TestSetDropsTheCachedConfigurationItReplaced(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://original.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})

	shared := cache.New(cache.NewArrayStore())
	manager := newTestManager(t, testDB.db, ManagerConfig{
		ConfigurationCache:    shared,
		ConfigurationCacheTTL: time.Minute,
	})
	base := repository.NewBase(testDB.db)
	service := NewService(
		testDB.db,
		repository.NewInstanceRepository(base),
		repository.NewWebhookRepository(base),
		shared, time.Minute,
		true,
	)

	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-invalidate"))
	dispatch := func() {
		t.Helper()
		if err := manager.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated,
			ConnectionWebhookData{Type: "connected", Connection: "open"}); err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
	}

	dispatch()

	enabled := true
	if _, err := service.Set(ctx, testWebhookSetGrant(), "beplus", SetInput{
		URL:       "https://replacement.example/hook",
		Enabled:   &enabled,
		Events:    map[string]bool{"connectionUpdated": true},
		EventsSet: true,
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	dispatch()

	rows, err := testDB.raw.Query(`SELECT url FROM whatsapp_webhook_deliveries ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			t.Fatal(err)
		}
		urls = append(urls, url)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(urls) != 2 {
		t.Fatalf("deliveries = %d, want 2", len(urls))
	}
	if urls[0] != "https://original.example/hook" {
		t.Fatalf("first delivery url = %q, want the original", urls[0])
	}
	if urls[1] != "https://replacement.example/hook" {
		t.Fatalf("second delivery url = %q, want the replacement: the cache outlived the change", urls[1])
	}
}
