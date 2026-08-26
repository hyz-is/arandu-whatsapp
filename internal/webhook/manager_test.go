package webhook

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"
	httpclient "github.com/arandu-io/hesape/http/client"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/queue/jobs"
	"github.com/arandu-io/hesape/str"
	_ "modernc.org/sqlite"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const testWebhookSigningSecret = "0123456789abcdef0123456789abcdef"

func TestDispatchSnapshotsDeliveryAndQueuesOnlyItsID(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	manager := newTestManager(t, testDB.db, ManagerConfig{})

	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-123"))
	if err := manager.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated,
		ConnectionWebhookData{Type: "connected", Connection: "open"}); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	var deliveryID, target, webhookURL, body, headersJSON, status string
	var attempts int
	if err := testDB.raw.QueryRow(`SELECT id, target, url, body, headers, status, attempts
		FROM whatsapp_webhook_deliveries`).Scan(
		&deliveryID, &target, &webhookURL, &body, &headersJSON, &status, &attempts); err != nil {
		t.Fatalf("read delivery: %v", err)
	}
	if target != string(deliveryTargetInstance) || webhookURL != "https://instance.example/hook" ||
		status != string(deliveryStatusPending) || attempts != 0 {
		t.Fatalf("unexpected delivery target=%q url=%q status=%q attempts=%d", target, webhookURL, status, attempts)
	}
	var snapshot WebhookPayload
	if err := json.Unmarshal([]byte(body), &snapshot); err != nil {
		t.Fatalf("decode delivery body: %v", err)
	}
	if snapshot.Event != types.WebhookEventConnectionUpdated || snapshot.Instance.Name != "beplus" {
		t.Fatalf("unexpected delivery body: %#v", snapshot)
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		t.Fatalf("decode delivery headers: %v", err)
	}
	if headers["x-request-id"] != "req-123" ||
		headers["x-webhook-event"] != string(types.WebhookEventConnectionUpdated) ||
		headers["x-owner-jid"] != "5531999999999@s.whatsapp.net" ||
		headers["X-Arandu-Delivery-ID"] != deliveryID ||
		headers[timestampHeader] != "" || headers[signatureHeader] != "" {
		t.Fatalf("unexpected delivery headers: %#v", headers)
	}

	var queueName, jobName, tenant, action, jobPayload string
	if err := testDB.raw.QueryRow(`SELECT queue, name, tenant_id, action, payload FROM jobs`).Scan(
		&queueName, &jobName, &tenant, &action, &jobPayload); err != nil {
		t.Fatalf("read queued job: %v", err)
	}
	if queueName != WebhookQueueName || jobName != WebhookDeliveryJobName || tenant != "acme" || action != string(authz.ActionRuntime) {
		t.Fatalf("unexpected job queue=%q name=%q tenant=%q action=%q", queueName, jobName, tenant, action)
	}
	var queued map[string]any
	if err := json.Unmarshal([]byte(jobPayload), &queued); err != nil {
		t.Fatalf("decode job payload: %v", err)
	}
	if len(queued) != 1 || queued["deliveryId"] != deliveryID {
		t.Fatalf("job payload = %#v, want only deliveryId %q", queued, deliveryID)
	}
}

func TestDispatchRollsBackDeliveryWhenQueueWriteFails(t *testing.T) {
	testDB := newWebhookTestDB(t, false)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	manager := newTestManager(t, testDB.db, ManagerConfig{})

	if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEventConnectionUpdated, nil); err == nil {
		t.Fatal("Dispatch() accepted a delivery without the native jobs table")
	}
	if got := testDB.count(t, "whatsapp_webhook_deliveries"); got != 0 {
		t.Fatalf("delivery rows after queue failure = %d, want 0", got)
	}
}

func TestDispatchReadsConfigurationWithoutAnInMemoryCache(t *testing.T) {
	t.Run("disabled event", func(t *testing.T) {
		testDB := newWebhookTestDB(t, true)
		testDB.insertInstance(t)
		testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{StatusInstance: true})
		manager := newTestManager(t, testDB.db, ManagerConfig{})

		if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
			types.WebhookEventConnectionUpdated, nil); err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
		if got := testDB.count(t, "whatsapp_webhook_deliveries"); got != 0 {
			t.Fatalf("delivery rows = %d, want 0", got)
		}
	})

	t.Run("global target without instance configuration", func(t *testing.T) {
		testDB := newWebhookTestDB(t, true)
		testDB.insertInstance(t)
		manager := newTestManager(t, testDB.db, ManagerConfig{
			GlobalEnabled: true,
			GlobalURL:     "https://global.example/hook",
		})

		if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
			types.WebhookEventConnectionUpdated, nil); err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
		var target, webhookURL string
		if err := testDB.raw.QueryRow(`SELECT target, url FROM whatsapp_webhook_deliveries`).Scan(&target, &webhookURL); err != nil {
			t.Fatal(err)
		}
		if target != string(deliveryTargetGlobal) || webhookURL != "https://global.example/hook" {
			t.Fatalf("global delivery target=%q url=%q", target, webhookURL)
		}
	})
}

func TestRegisteredHandlerDeliversOnceAndPersistsTheResult(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	requestCount := 0
	var sentBody string
	var sentHeader http.Header
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		sentBody = string(body)
		sentHeader = request.Header.Clone()
		return testResponse(request, http.StatusNoContent,
			strings.Repeat("x", 4*1024)), nil
	})
	manager := newTestManager(t, testDB.db, ManagerConfig{HTTPClient: &http.Client{Transport: transport}})
	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-deliver"))
	if err := manager.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated,
		map[string]string{"state": "open"}); err != nil {
		t.Fatal(err)
	}

	worker := queue.NewWorker(queue.NullQueue{}, queue.WorkerOptions{})
	if err := manager.RegisterJobHandlers(worker); err != nil {
		t.Fatal(err)
	}
	handler, ok := worker.Handler(WebhookDeliveryJobName)
	if !ok {
		t.Fatal("webhook delivery handler was not registered")
	}
	queued := popDeliveryJob(t, manager)
	if queued.Attributes.Tries != deliveryMaxTries {
		t.Fatalf("job tries = %d, want %d", queued.Attributes.Tries, deliveryMaxTries)
	}
	grant := jobs.GrantFor(queued)
	if err := handler.Handle(ctx, grant, queued); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var id, status, storedURL, storedBody, storedHeaders string
	var attempts, responseStatus int
	var responseBody sql.NullString
	var deliveredAt sql.NullTime
	if err := testDB.raw.QueryRow(`SELECT id, status, attempts, response_status, response_body,
		url, body, headers, delivered_at FROM whatsapp_webhook_deliveries`).Scan(
		&id, &status, &attempts, &responseStatus, &responseBody,
		&storedURL, &storedBody, &storedHeaders, &deliveredAt); err != nil {
		t.Fatal(err)
	}
	if status != string(deliveryStatusDelivered) || attempts != 1 || responseStatus != http.StatusNoContent || !deliveredAt.Valid {
		t.Fatalf("delivery status=%q attempts=%d response=%d delivered_at=%v", status, attempts, responseStatus, deliveredAt)
	}
	if responseBody.Valid || storedURL != "" || storedBody != "" || storedHeaders != "{}" {
		t.Fatalf("delivered snapshot retained sensitive data: response=%v url=%q body=%q headers=%q",
			responseBody, storedURL, storedBody, storedHeaders)
	}
	if requestCount != 1 || sentHeader.Get("X-Arandu-Delivery-ID") != id || sentHeader.Get("x-request-id") != "req-deliver" {
		t.Fatalf("request count=%d delivery header=%q request header=%q", requestCount,
			sentHeader.Get("X-Arandu-Delivery-ID"), sentHeader.Get("x-request-id"))
	}
	if !strings.Contains(sentBody, `"state":"open"`) {
		t.Fatalf("sent body = %s", sentBody)
	}
	wantSignature := signDelivery([]byte(testWebhookSigningSecret), sentHeader.Get(timestampHeader), id, []byte(sentBody))
	if !hmac.Equal([]byte(sentHeader.Get(signatureHeader)), []byte(wantSignature)) {
		t.Fatalf("signature = %q, want %q", sentHeader.Get(signatureHeader), wantSignature)
	}

	if err := handler.Handle(ctx, grant, queued); err != nil {
		t.Fatalf("idempotent Handle() error = %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("delivered snapshot sent %d times, want once", requestCount)
	}
}

func TestHandlerPersistsFailureAndRetriesTheSameSnapshot(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	requestCount := 0
	deliveryHeaders := make([]string, 0, 3)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		deliveryHeaders = append(deliveryHeaders, request.Header.Get("X-Arandu-Delivery-ID"))
		if requestCount == 1 {
			return nil, errors.New("dial failed")
		}
		if requestCount == 2 {
			return testResponse(request, http.StatusServiceUnavailable, "retry"), nil
		}
		return testResponse(request, http.StatusOK, "ok"), nil
	})
	manager := newTestManager(t, testDB.db, ManagerConfig{HTTPClient: &http.Client{Transport: transport}})
	if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEventConnectionUpdated, nil); err != nil {
		t.Fatal(err)
	}
	queued := popDeliveryJob(t, manager)
	grant := jobs.GrantFor(queued)
	if err := manager.handleDelivery(context.Background(), grant, queued); err == nil {
		t.Fatal("first delivery attempt unexpectedly succeeded")
	}
	var status, lastError string
	var attempts int
	if err := testDB.raw.QueryRow(`SELECT status, attempts, last_error FROM whatsapp_webhook_deliveries`).Scan(
		&status, &attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != string(deliveryStatusFailed) || attempts != 1 || lastError != "request_failed" {
		t.Fatalf("first attempt status=%q attempts=%d error=%q", status, attempts, lastError)
	}

	queued.Attempts = 2
	if err := manager.handleDelivery(context.Background(), grant, queued); err == nil {
		t.Fatal("non-2xx retry unexpectedly succeeded")
	}
	var responseStatus int
	if err := testDB.raw.QueryRow(`SELECT status, attempts, response_status, last_error
		FROM whatsapp_webhook_deliveries`).Scan(&status, &attempts, &responseStatus, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != string(deliveryStatusFailed) || attempts != 2 || responseStatus != http.StatusServiceUnavailable ||
		lastError != "http_status_503" {
		t.Fatalf("second attempt status=%q attempts=%d response=%d error=%q", status, attempts, responseStatus, lastError)
	}

	queued.Attempts = 3
	if err := manager.handleDelivery(context.Background(), grant, queued); err != nil {
		t.Fatalf("successful retry error = %v", err)
	}
	if err := testDB.raw.QueryRow(`SELECT status, attempts FROM whatsapp_webhook_deliveries`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != string(deliveryStatusDelivered) || attempts != 3 {
		t.Fatalf("retry status=%q attempts=%d", status, attempts)
	}
	if len(deliveryHeaders) != 3 || deliveryHeaders[0] == "" ||
		deliveryHeaders[0] != deliveryHeaders[1] || deliveryHeaders[1] != deliveryHeaders[2] {
		t.Fatalf("delivery headers across retries = %#v", deliveryHeaders)
	}
}

func TestWebhookRepositoryDeniesBeforeTouchingANilDatabase(t *testing.T) {
	repository := newSQLDeliveryRepository(data.Wrap(nil, data.DialectSQLite))
	wrongAction := security.SystemGrant(authz.ActionWebhookView, "acme")
	if _, err := repository.FindConfiguration(context.Background(), wrongAction, 1); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("wrong-action error = %v, want ErrForbidden", err)
	}
	withoutTenant := security.SystemGrant(authz.ActionRuntime, "")
	if _, err := repository.FindDelivery(context.Background(), withoutTenant, "delivery-1"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("missing-tenant error = %v, want ErrForbidden", err)
	}
	if _, err := repository.PruneBefore(context.Background(), wrongAction, time.Now()); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("prune wrong-action error = %v, want ErrForbidden", err)
	}
}

func TestManagerRejectsInvalidWorkBeforeDatabaseAccess(t *testing.T) {
	manager := newTestManager(t, data.Wrap(nil, data.DialectSQLite), ManagerConfig{})
	wrongAction := security.SystemGrant(security.Action("whatsapp.invalid"), "acme")
	if err := manager.Dispatch(context.Background(), wrongAction, testInstance(),
		types.WebhookEventConnectionUpdated, nil); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("wrong-action error = %v, want ErrForbidden", err)
	}
	if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEvent("not.supported"), nil); !errors.Is(err, ErrUnsupportedEvent) {
		t.Fatalf("unsupported-event error = %v, want ErrUnsupportedEvent", err)
	}
	if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEventConnectionUpdated, map[string]any{"bad": make(chan struct{})}); err == nil {
		t.Fatal("unserializable payload was accepted")
	}
}

func TestManagerHTTPClientGuardsProductionDestinationsAndResponses(t *testing.T) {
	t.Run("unsafe destination", func(t *testing.T) {
		manager := newTestManager(t, data.Wrap(nil, data.DialectSQLite), ManagerConfig{})
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://127.0.0.1:1/hook", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.client.Do(request)
		if result != nil {
			result.Body.Close()
		}
		if !errors.Is(err, httpclient.ErrInternalAddress) {
			t.Fatalf("error = %v, want ErrInternalAddress", err)
		}
	})

	t.Run("bounded response", func(t *testing.T) {
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			result := testResponse(request, http.StatusOK, "ok")
			result.ContentLength = httpclient.DefaultMaxResponseBytes + 1
			return result, nil
		})
		manager := newTestManager(t, data.Wrap(nil, data.DialectSQLite), ManagerConfig{
			HTTPClient: &http.Client{Transport: transport},
		})
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/hook", nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.client.Do(request)
		if result != nil {
			result.Body.Close()
		}
		if !errors.Is(err, httpclient.ErrResponseTooLarge) {
			t.Fatalf("error = %v, want ErrResponseTooLarge", err)
		}
	})
}

func TestNewManagerValidatesItsBoundary(t *testing.T) {
	db := data.Wrap(nil, data.DialectSQLite)
	if _, err := NewManager(nil, ManagerConfig{}); err == nil {
		t.Fatal("NewManager() accepted a nil database")
	}
	if _, err := NewManager(db, ManagerConfig{GlobalEnabled: true}); !errors.Is(err, ErrInvalidWebhookURL) {
		t.Fatalf("missing URL error = %v, want ErrInvalidWebhookURL", err)
	}
	if _, err := NewManager(db, ManagerConfig{GlobalURL: "ftp://example.com/hook"}); !errors.Is(err, ErrInvalidWebhookURL) {
		t.Fatalf("invalid URL error = %v, want ErrInvalidWebhookURL", err)
	}
	if _, err := NewManager(db, ManagerConfig{GlobalEnabled: true, GlobalURL: "https://example.com/hook"}); !errors.Is(err, ErrSigningSecretRequired) {
		t.Fatalf("missing signing secret error = %v, want ErrSigningSecretRequired", err)
	}
	if _, err := NewManager(db, ManagerConfig{SigningSecret: "short"}); !errors.Is(err, ErrSigningSecretTooShort) {
		t.Fatalf("short signing secret error = %v, want ErrSigningSecretTooShort", err)
	}
	if _, err := NewManager(db, ManagerConfig{Retention: -time.Second}); err == nil {
		t.Fatal("NewManager() accepted negative retention")
	}
	if manager := newTestManager(t, db, ManagerConfig{}); manager.retention != DefaultDeliveryRetention {
		t.Fatalf("default retention = %s, want %s", manager.retention, DefaultDeliveryRetention)
	}
	if err := newTestManager(t, db, ManagerConfig{}).RegisterJobHandlers(nil); err == nil {
		t.Fatal("RegisterJobHandlers() accepted a nil worker")
	}
}

func TestWebhookCannotBeEnabledWithoutASigningSecret(t *testing.T) {
	service := NewService(nil, nil, nil, nil, 0, false)
	_, err := service.Set(context.Background(), testRuntimeGrant(), "beplus", SetInput{
		URL: "https://example.com/hook",
	})
	if !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("Set() error = %v, want ErrInvalidInput", err)
	}
}

func TestDispatcherFailsClosedForAnExistingUnsignedWebhook(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://example.com/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	manager, err := NewManager(testDB.db, ManagerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	err = manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEventConnectionUpdated, nil)
	if !errors.Is(err, ErrSigningSecretRequired) {
		t.Fatalf("Dispatch() error = %v, want ErrSigningSecretRequired", err)
	}
	if got := testDB.count(t, "whatsapp_webhook_deliveries"); got != 0 {
		t.Fatalf("unsigned delivery rows = %d, want 0", got)
	}
}

func TestMissingDeliverySnapshotIsTerminalAndIdempotent(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	manager := newTestManager(t, testDB.db, ManagerConfig{})
	job, err := jobs.New(testRuntimeGrant(), WebhookQueueName, WebhookDeliveryJobName,
		deliveryJobPayload{DeliveryID: "019f0000-0000-7000-8000-000000000099"})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.handleDelivery(context.Background(), jobs.GrantFor(&job), &job); err != nil {
		t.Fatalf("missing snapshot returned retryable error: %v", err)
	}
}

func TestInstanceDeletionDuringDeliveryDoesNotCreateADeadJob(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://example.com/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if _, err := testDB.raw.Exec(`DELETE FROM whatsapp_instances WHERE tenant_id = ? AND id = ?`, "acme", 1); err != nil {
			return nil, err
		}
		return testResponse(request, http.StatusNoContent, ""), nil
	})
	manager := newTestManager(t, testDB.db, ManagerConfig{HTTPClient: &http.Client{Transport: transport}})
	if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEventConnectionUpdated, nil); err != nil {
		t.Fatal(err)
	}
	job := popDeliveryJob(t, manager)
	if err := manager.handleDelivery(context.Background(), jobs.GrantFor(job), job); err != nil {
		t.Fatalf("concurrent instance deletion returned retryable error: %v", err)
	}
	if got := testDB.count(t, "whatsapp_webhook_deliveries"); got != 0 {
		t.Fatalf("delivery rows after cascade = %d, want 0", got)
	}
}

func TestDeliveryFailureNeverPersistsASecretFromTheTargetURL(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://example.com/hook?token=secret", true, types.WebhookEvents{ConnectionUpdated: true})
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Post", URL: request.URL.String(), Err: errors.New("dial failed")}
	})
	manager := newTestManager(t, testDB.db, ManagerConfig{HTTPClient: &http.Client{Transport: transport}})
	if err := manager.Dispatch(context.Background(), testRuntimeGrant(), testInstance(),
		types.WebhookEventConnectionUpdated, nil); err != nil {
		t.Fatal(err)
	}
	job := popDeliveryJob(t, manager)
	err := manager.handleDelivery(context.Background(), jobs.GrantFor(job), job)
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("handler error leaked target URL secret: %v", err)
	}
	var lastError string
	if err := testDB.raw.QueryRow(`SELECT last_error FROM whatsapp_webhook_deliveries`).Scan(&lastError); err != nil {
		t.Fatal(err)
	}
	if lastError != "request_failed" || strings.Contains(lastError, "secret") {
		t.Fatalf("persisted failure = %q", lastError)
	}
}

func TestLoggedWebhookOriginDropsCredentialsPathQueryAndFragment(t *testing.T) {
	got := safeWebhookURL("https://user:password@example.com/token-secret/hook?token=query-secret#fragment")
	if got != "https://example.com" {
		t.Fatalf("safeWebhookURL() = %q", got)
	}
}

func TestDeliveryRetentionPrunesEveryExpiredSnapshotForItsTenant(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	now := time.Now().UTC()
	for _, item := range []struct {
		id      string
		status  deliveryStatus
		created time.Time
	}{
		{id: "old-delivered", status: deliveryStatusDelivered, created: now.Add(-2 * time.Hour)},
		{id: "old-failed", status: deliveryStatusFailed, created: now.Add(-2 * time.Hour)},
		{id: "old-pending", status: deliveryStatusPending, created: now.Add(-2 * time.Hour)},
		{id: "recent-delivered", status: deliveryStatusDelivered, created: now},
	} {
		if _, err := testDB.raw.Exec(`INSERT INTO whatsapp_webhook_deliveries (
			id, tenant_id, instance_id, event, target, url, body, headers, status,
			attempts, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.id, "acme", 1,
			string(types.WebhookEventConnectionUpdated), string(deliveryTargetInstance),
			"https://example.com/hook", `{}`, `{}`, string(item.status), 0, item.created, item.created); err != nil {
			t.Fatal(err)
		}
	}
	manager := newTestManager(t, testDB.db, ManagerConfig{Retention: time.Hour})
	manager.pruneExpired(context.Background(), testRuntimeGrant())
	var ids string
	rows, err := testDB.raw.Query(`SELECT id FROM whatsapp_webhook_deliveries ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids += id + ","
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if ids != "recent-delivered," {
		t.Fatalf("snapshots after prune = %q, want only recent delivery", ids)
	}
}

func TestDeliveryAttemptAuditDoesNotRegressAfterManualRedrive(t *testing.T) {
	testDB := newWebhookTestDB(t, true)
	testDB.insertInstance(t)
	now := time.Now().UTC()
	if _, err := testDB.raw.Exec(`INSERT INTO whatsapp_webhook_deliveries (
		id, tenant_id, instance_id, event, target, url, body, headers, status,
		attempts, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "redriven", "acme", 1,
		string(types.WebhookEventConnectionUpdated), string(deliveryTargetInstance),
		"https://example.com/hook", `{}`, `{}`, string(deliveryStatusFailed), 5, now, now); err != nil {
		t.Fatal(err)
	}
	repository := newSQLDeliveryRepository(testDB.db)
	if err := repository.MarkAttempt(context.Background(), testRuntimeGrant(), "redriven", 1); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := testDB.raw.QueryRow(`SELECT attempts FROM whatsapp_webhook_deliveries WHERE id = ?`, "redriven").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 5 {
		t.Fatalf("attempts regressed to %d, want 5", attempts)
	}
}

type webhookTestDB struct {
	raw *sql.DB
	db  *data.DB
}

func newWebhookTestDB(t *testing.T, withJobs bool) webhookTestDB {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:webhook-"+str.UUID()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range webhookTestSchema {
		if _, err := raw.Exec(statement); err != nil {
			t.Fatalf("create webhook test schema: %v", err)
		}
	}
	wrapped := data.Wrap(raw, data.DialectSQLite)
	if withJobs {
		connection := database.ForMigrations(database.NewConnection(raw, "", "", map[string]any{
			"driver": string(database.DialectSQLite),
			"name":   "default",
		}))
		for _, migration := range queue.NewDatabaseQueue(wrapped).Migrations() {
			if err := migration.Up(context.Background(), connection); err != nil {
				t.Fatalf("apply queue migration %s: %v", migration.GetName(), err)
			}
		}
	}
	return webhookTestDB{raw: raw, db: wrapped}
}

func (d webhookTestDB) insertInstance(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := d.raw.Exec(`INSERT INTO whatsapp_instances
		(id, tenant_id, name, status, external_attributes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, 1, "acme", "beplus", "ONLINE", `{}`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := d.raw.Exec(`INSERT INTO whatsapp_instance_connections
		(tenant_id, instance_id, connection_status, connection_attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "acme", 1, "CLOSED", 0, now, now); err != nil {
		t.Fatal(err)
	}
}

func (d webhookTestDB) insertWebhook(t *testing.T, webhookURL string, enabled bool, events types.WebhookEvents) {
	t.Helper()
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := d.raw.Exec(`INSERT INTO whatsapp_webhooks
		(id, tenant_id, instance_id, url, enabled, events, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, 10, "acme", 1, webhookURL, enabled, string(encoded), now, now); err != nil {
		t.Fatal(err)
	}
}

func (d webhookTestDB) count(t *testing.T, table string) int {
	t.Helper()
	var count int
	if err := d.raw.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func newTestManager(t *testing.T, db *data.DB, config ManagerConfig) *Manager {
	t.Helper()
	if config.SigningSecret == "" {
		config.SigningSecret = testWebhookSigningSecret
	}
	manager, err := NewManager(db, config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func popDeliveryJob(t *testing.T, manager *Manager) *jobs.Job {
	t.Helper()
	queued, err := manager.queue.Pop(context.Background(), WebhookQueueName, 1, time.Minute)
	if err != nil {
		t.Fatalf("pop delivery job: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("popped %d delivery jobs, want 1", len(queued))
	}
	return queued[0]
}

func testInstance() WebhookInstance {
	owner := "5531999999999@s.whatsapp.net"
	return WebhookInstance{
		ID:                 1,
		Name:               "beplus",
		ConnectionStatus:   "online",
		OwnerJID:           &owner,
		ExternalAttributes: map[string]any{},
	}
}

func testRuntimeGrant() security.Grant {
	return security.SystemGrant(authz.ActionRuntime, "acme")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func testResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}

var webhookTestSchema = []string{
	`CREATE TABLE whatsapp_instances (
    id BIGINT NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description VARCHAR(255),
    status VARCHAR(32) NOT NULL,
    owner_jid VARCHAR(100),
    profile_pic_url VARCHAR(500),
    external_attributes TEXT NOT NULL,
    connection_lock_owner VARCHAR(255),
    connection_lock_until TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
)`,
	`CREATE TABLE whatsapp_instance_connections (
    tenant_id BIGINT NOT NULL,
    instance_id BIGINT NOT NULL,
    connection_status VARCHAR(64) NOT NULL,
    whatsapp_device_jid VARCHAR(100),
    whatsapp_owner_jid VARCHAR(100),
    whatsapp_phone_number VARCHAR(32),
    profile_pic_id VARCHAR(255),
    last_connected_at TIMESTAMP,
    last_disconnected_at TIMESTAMP,
    last_connection_attempt_at TIMESTAMP,
    last_connection_error VARCHAR(255),
    last_connection_event VARCHAR(100),
    connection_attempts BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, instance_id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
	`CREATE TABLE whatsapp_webhooks (
    id BIGINT NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    instance_id BIGINT NOT NULL,
    url VARCHAR(500) NOT NULL,
    enabled BOOLEAN NOT NULL,
    events TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, instance_id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
	`CREATE TABLE whatsapp_webhook_deliveries (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    instance_id BIGINT NOT NULL,
    event VARCHAR(100) NOT NULL,
    target VARCHAR(32) NOT NULL,
    url VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    headers TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    attempts INTEGER NOT NULL,
    response_status INTEGER,
    response_body TEXT,
    last_error TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    delivered_at TIMESTAMP,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, instance_id)
        REFERENCES whatsapp_instances (tenant_id, id) ON DELETE CASCADE
)`,
}
