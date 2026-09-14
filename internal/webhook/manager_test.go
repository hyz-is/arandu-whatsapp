package webhook

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/queue/jobs"
	"github.com/arandu-io/hesape/str"
	hesapewebhook "github.com/arandu-io/hesape/webhook"
	_ "modernc.org/sqlite"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const testWebhookSigningSecret = "0123456789abcdef0123456789abcdef"

func TestDispatchPreservesSnapshotQueueAndFanoutContracts(t *testing.T) {
	db := newWebhookTestDB(t, true)
	db.insertInstance(t)
	db.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	m := newTestManager(t, db.db, ManagerConfig{GlobalEnabled: true, GlobalURL: "https://global.example/hook"})
	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-123"))
	if err := m.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated, map[string]string{"state": "open"}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.raw.Query(`SELECT id,event_id,endpoint_id,target,body,headers FROM whatsapp_webhook_deliveries ORDER BY endpoint_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct{ id, event, endpoint, target, body, headers string }
	var got []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.event, &item.endpoint, &item.target, &item.body, &item.headers); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if len(got) != 2 || got[0].event == "" || got[0].event != got[1].event || got[0].id == got[1].id || got[0].endpoint == got[1].endpoint {
		t.Fatalf("fanout identities=%#v", got)
	}
	if got[0].endpoint != "global:1" || got[1].endpoint != "instance:1" {
		t.Fatalf("endpoints=%q,%q", got[0].endpoint, got[1].endpoint)
	}
	for _, item := range got {
		var payload WebhookPayload
		if err := json.Unmarshal([]byte(item.body), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Event != types.WebhookEventConnectionUpdated || payload.Instance.Name != "beplus" {
			t.Fatalf("payload=%#v", payload)
		}
		var headers map[string]string
		if err := json.Unmarshal([]byte(item.headers), &headers); err != nil {
			t.Fatal(err)
		}
		if headers["x-request-id"] != "req-123" || headers["X-Arandu-Delivery-ID"] != item.id || headers["User-Agent"] != webhookUserAgent {
			t.Fatalf("headers=%#v", headers)
		}
	}
	if db.count(t, "jobs") != 2 {
		t.Fatalf("jobs=%d", db.count(t, "jobs"))
	}
	var queueName, jobName, action string
	if err := db.raw.QueryRow(`SELECT queue,name,action FROM jobs LIMIT 1`).Scan(&queueName, &jobName, &action); err != nil {
		t.Fatal(err)
	}
	if queueName != WebhookQueueName || jobName != WebhookDeliveryJobName || action != string(authz.ActionRuntime) {
		t.Fatalf("queue=%q job=%q action=%q", queueName, jobName, action)
	}
}

func TestDispatchRollsBackSnapshotWhenQueueInsertFails(t *testing.T) {
	db := newWebhookTestDB(t, false)
	db.insertInstance(t)
	db.insertWebhook(t, "https://example.com/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	m := newTestManager(t, db.db, ManagerConfig{})
	if err := m.Dispatch(context.Background(), testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated, nil); err == nil {
		t.Fatal("missing jobs table accepted")
	}
	if db.count(t, "whatsapp_webhook_deliveries") != 0 {
		t.Fatal("snapshot escaped rollback")
	}
}

func TestHandlerPreservesWireHMACAndRetriesSameDelivery(t *testing.T) {
	db := newWebhookTestDB(t, true)
	db.insertInstance(t)
	db.insertWebhook(t, "https://example.com/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	var mu sync.Mutex
	var ids []string
	var lastHeader http.Header
	var lastBody []byte
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		ids = append(ids, r.Header.Get("X-Arandu-Delivery-ID"))
		lastHeader = r.Header.Clone()
		lastBody = body
		n := len(ids)
		mu.Unlock()
		if n == 1 {
			return nil, errors.New("dial failed")
		}
		if n == 2 {
			return testResponse(r, 503, "retry"), nil
		}
		return testResponse(r, 204, "ok"), nil
	})
	m := newTestManager(t, db.db, ManagerConfig{HTTPClient: &http.Client{Transport: transport}})
	if err := m.Dispatch(context.Background(), testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated, map[string]string{"state": "open"}); err != nil {
		t.Fatal(err)
	}
	h := registeredDeliveryHandler(t, m)
	job := popDeliveryJob(t, db.db)
	grant := jobs.GrantFor(job)
	if job.Attributes.Tries != 5 {
		t.Fatalf("tries=%d", job.Attributes.Tries)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		err := h.Handle(context.Background(), grant, job)
		if attempt < 3 && err == nil {
			t.Fatalf("attempt %d succeeded", attempt)
		}
		if attempt == 3 && err != nil {
			t.Fatal(err)
		}
	}
	if len(ids) != 3 || ids[0] == "" || ids[0] != ids[1] || ids[1] != ids[2] {
		t.Fatalf("ids=%v", ids)
	}
	if lastHeader.Get("User-Agent") != webhookUserAgent {
		t.Fatalf("user agent=%q", lastHeader.Get("User-Agent"))
	}
	want := hesapewebhook.Sign([]byte(testWebhookSigningSecret), lastHeader.Get(timestampHeader), ids[0], lastBody)
	if !hmac.Equal([]byte(lastHeader.Get(signatureHeader)), []byte(want)) {
		t.Fatalf("signature=%q want=%q", lastHeader.Get(signatureHeader), want)
	}
	var status, url, body, headers string
	var attempts int
	if err := db.raw.QueryRow(`SELECT status,attempts,url,body,headers FROM whatsapp_webhook_deliveries`).Scan(&status, &attempts, &url, &body, &headers); err != nil {
		t.Fatal(err)
	}
	if status != string(hesapewebhook.StatusDelivered) || attempts != 3 || url != "" || body != "" || headers != "{}" {
		t.Fatalf("status=%q attempts=%d url=%q body=%q headers=%q", status, attempts, url, body, headers)
	}
	if err := h.Handle(context.Background(), grant, job); err != nil {
		t.Fatalf("idempotent handle=%v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("delivered row resent: %v", ids)
	}
}

func TestHandlerMarksOrdinary4xxPermanent(t *testing.T) {
	db := newWebhookTestDB(t, true)
	db.insertInstance(t)
	db.insertWebhook(t, "https://example.com/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	m := newTestManager(t, db.db, ManagerConfig{HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return testResponse(r, 400, "bad"), nil })}})
	if err := m.Dispatch(context.Background(), testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated, nil); err != nil {
		t.Fatal(err)
	}
	job := popDeliveryJob(t, db.db)
	if err := registeredDeliveryHandler(t, m).Handle(context.Background(), jobs.GrantFor(job), job); err != nil {
		t.Fatalf("permanent retry=%v", err)
	}
	var status, last string
	var permanent bool
	if err := db.raw.QueryRow(`SELECT status,permanent,last_error FROM whatsapp_webhook_deliveries`).Scan(&status, &permanent, &last); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || !permanent || last != "http_status_400" {
		t.Fatalf("status=%q permanent=%v last=%q", status, permanent, last)
	}
}

func TestStoreIdempotencyFencingAndConcurrentClaim(t *testing.T) {
	db := newWebhookTestDB(t, false)
	db.insertInstance(t)
	store := newSQLDeliveryStore(db.db)
	g := testRuntimeGrant()
	now := time.Now().UTC()
	base := hesapewebhook.Delivery{ID: "d1", EventID: "e1", EventName: "connection.update", EndpointID: "instance:1", URL: "https://example.com", Headers: map[string]string{}, Body: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
	created, err := store.Create(context.Background(), g, base)
	if err != nil || !created {
		t.Fatalf("create=%v %v", created, err)
	}
	dupe := base
	dupe.ID = "d2"
	created, err = store.Create(context.Background(), g, dupe)
	if err != nil || created {
		t.Fatalf("duplicate=%v %v", created, err)
	}
	claim1, claimed, err := store.Claim(context.Background(), g, "d1", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim1=%v %v", claimed, err)
	}
	if _, err := db.raw.Exec(`UPDATE whatsapp_webhook_deliveries SET lease_until=? WHERE id='d1'`, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	claim2, claimed, err := store.Claim(context.Background(), g, "d1", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim2=%v %v", claimed, err)
	}
	if err := store.Complete(context.Background(), g, claim1, hesapewebhook.Result{StatusCode: 200, FinishedAt: now}); !errors.Is(err, hesapewebhook.ErrStaleClaim) {
		t.Fatalf("stale=%v", err)
	}
	if err := store.Complete(context.Background(), g, claim2, hesapewebhook.Result{StatusCode: 204, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	removed := base
	removed.ID = "removed"
	removed.EventID = "event-removed"
	created, err = store.Create(context.Background(), g, removed)
	if err != nil || !created {
		t.Fatalf("removed Create() created=%v error=%v", created, err)
	}
	removedClaim, claimed, err := store.Claim(context.Background(), g, removed.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("removed Claim() claimed=%v error=%v", claimed, err)
	}
	if _, err := db.raw.Exec(`DELETE FROM whatsapp_webhook_deliveries WHERE id=?`, removed.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(context.Background(), g, removedClaim, hesapewebhook.Result{StatusCode: 204, FinishedAt: now}); err != nil {
		t.Fatalf("removed delivery settlement error=%v", err)
	}

	concurrent := base
	concurrent.ID = "dc"
	concurrent.EventID = "ec"
	created, err = store.Create(context.Background(), g, concurrent)
	if err != nil || !created {
		t.Fatal(err)
	}
	start := make(chan struct{})
	outcomes := make(chan bool, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, e := store.Claim(context.Background(), g, "dc", time.Minute)
			outcomes <- ok
			failures <- e
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)
	close(failures)
	count := 0
	for ok := range outcomes {
		if ok {
			count++
		}
	}
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	if count != 1 {
		t.Fatalf("claims=%d", count)
	}
}

func TestLegacyRowAndRuntimeJobRemainConsumable(t *testing.T) {
	db := newWebhookTestDB(t, true)
	db.insertInstance(t)
	now := time.Now().UTC()
	_, err := db.raw.Exec(`INSERT INTO whatsapp_webhook_deliveries(id,tenant_id,instance_id,event,target,url,body,headers,status,attempts,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, "legacy", "acme", 1, "connection.update", "instance", "https://example.com", `{}`, `{}`, "pending", 0, now, now)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, db.db, ManagerConfig{HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return testResponse(r, 204, ""), nil })}})
	job, err := jobs.New(testRuntimeGrant(), WebhookQueueName, WebhookDeliveryJobName, map[string]string{"deliveryId": "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := registeredDeliveryHandler(t, m).Handle(context.Background(), jobs.GrantFor(&job), &job); err != nil {
		t.Fatal(err)
	}
	var eventID, endpointID sql.NullString
	var status string
	if err := db.raw.QueryRow(`SELECT event_id,endpoint_id,status FROM whatsapp_webhook_deliveries WHERE id='legacy'`).Scan(&eventID, &endpointID, &status); err != nil {
		t.Fatal(err)
	}
	if eventID.Valid || endpointID.Valid || status != "delivered" {
		t.Fatalf("event=%v endpoint=%v status=%q", eventID, endpointID, status)
	}
}

func TestBoundariesFailClosed(t *testing.T) {
	db := data.Wrap(nil, data.DialectSQLite)
	if _, err := NewManager(nil, ManagerConfig{}); err == nil {
		t.Fatal("nil db accepted")
	}
	if _, err := NewManager(db, ManagerConfig{GlobalEnabled: true}); !errors.Is(err, ErrInvalidWebhookURL) {
		t.Fatalf("url=%v", err)
	}
	if _, err := NewManager(db, ManagerConfig{GlobalURL: "https://example.com/" + strings.Repeat("x", MaxURLLength)}); !errors.Is(err, ErrInvalidWebhookURL) {
		t.Fatalf("long url=%v", err)
	}
	if _, err := NewManager(db, ManagerConfig{GlobalEnabled: true, GlobalURL: "https://example.com"}); !errors.Is(err, ErrSigningSecretRequired) {
		t.Fatalf("secret=%v", err)
	}
	if _, err := NewManager(db, ManagerConfig{SigningSecret: "short"}); !errors.Is(err, ErrSigningSecretTooShort) {
		t.Fatalf("short=%v", err)
	}
	store := newSQLDeliveryStore(db)
	wrong := security.SystemGrant(authz.ActionWebhookView, "acme")
	if _, err := store.FindConfiguration(context.Background(), wrong, 1); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("grant=%v", err)
	}
	m := newTestManager(t, db, ManagerConfig{})
	if err := m.Dispatch(context.Background(), security.SystemGrant(security.Action("wrong"), "acme"), testInstance(), types.WebhookEventConnectionUpdated, nil); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("dispatch=%v", err)
	}
	if err := m.Dispatch(context.Background(), testRuntimeGrant(), testInstance(), types.WebhookEvent("bad"), nil); !errors.Is(err, ErrUnsupportedEvent) {
		t.Fatalf("event=%v", err)
	}
	if err := m.RegisterJobHandlers(nil); err == nil {
		t.Fatal("nil worker accepted")
	}
}

func TestUnsignedConfigurationFailsBeforePersistence(t *testing.T) {
	db := newWebhookTestDB(t, true)
	db.insertInstance(t)
	db.insertWebhook(t, "https://example.com", true, types.WebhookEvents{ConnectionUpdated: true})
	m, err := NewManager(db.db, ManagerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	err = m.Dispatch(context.Background(), testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated, nil)
	if !errors.Is(err, ErrSigningSecretRequired) {
		t.Fatalf("err=%v", err)
	}
	if db.count(t, "whatsapp_webhook_deliveries") != 0 {
		t.Fatal("unsigned row persisted")
	}
	service := NewService(nil, nil, nil, nil, 0, false)
	_, err = service.Set(context.Background(), testRuntimeGrant(), "beplus", SetInput{URL: "https://example.com"})
	if !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("service=%v", err)
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
	raw.SetMaxOpenConns(4)
	_, _ = raw.Exec(`PRAGMA foreign_keys=ON`)
	for _, stmt := range webhookTestSchema {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	wrapped := data.Wrap(raw, data.DialectSQLite)
	if withJobs {
		conn := database.ForMigrations(database.NewConnection(raw, "", "", map[string]any{"driver": "sqlite", "name": "default"}))
		for _, migration := range queue.NewDatabaseQueue(wrapped).Migrations() {
			if err := migration.Up(context.Background(), conn); err != nil {
				t.Fatal(err)
			}
		}
	}
	return webhookTestDB{raw, wrapped}
}
func (d webhookTestDB) insertInstance(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()
	_, err := d.raw.Exec(`INSERT INTO whatsapp_instances(id,tenant_id,name,status,external_attributes,created_at,updated_at)VALUES(?,?,?,?,?,?,?)`, 1, "acme", "beplus", "ONLINE", `{}`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.raw.Exec(`INSERT INTO whatsapp_instance_connections(tenant_id,instance_id,connection_status,connection_attempts,created_at,updated_at)VALUES(?,?,?,?,?,?)`, "acme", 1, "CLOSED", 0, now, now)
	if err != nil {
		t.Fatal(err)
	}
}
func (d webhookTestDB) insertWebhook(t *testing.T, url string, enabled bool, events types.WebhookEvents) {
	t.Helper()
	encoded, _ := json.Marshal(events)
	now := time.Now().UTC()
	_, err := d.raw.Exec(`INSERT INTO whatsapp_webhooks(id,tenant_id,instance_id,url,enabled,events,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?)`, 10, "acme", 1, url, enabled, string(encoded), now, now)
	if err != nil {
		t.Fatal(err)
	}
}
func (d webhookTestDB) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := d.raw.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func newTestManager(t *testing.T, db *data.DB, cfg ManagerConfig) *Manager {
	t.Helper()
	if cfg.SigningSecret == "" {
		cfg.SigningSecret = testWebhookSigningSecret
	}
	m, err := NewManager(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func registeredDeliveryHandler(t *testing.T, m *Manager) queue.Handler {
	t.Helper()
	w := queue.NewWorker(queue.NullQueue{}, queue.WorkerOptions{})
	if err := m.RegisterJobHandlers(w); err != nil {
		t.Fatal(err)
	}
	h, ok := w.Handler(WebhookDeliveryJobName)
	if !ok {
		t.Fatal("missing handler")
	}
	return h
}
func popDeliveryJob(t *testing.T, db *data.DB) *jobs.Job {
	t.Helper()
	found, err := queue.NewDatabaseQueue(db).Pop(context.Background(), WebhookQueueName, 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("jobs=%d", len(found))
	}
	return found[0]
}
func testInstance() WebhookInstance {
	owner := "5531999999999@s.whatsapp.net"
	return WebhookInstance{ID: 1, Name: "beplus", ConnectionStatus: "online", OwnerJID: &owner, ExternalAttributes: map[string]any{}}
}
func testRuntimeGrant() security.Grant { return security.SystemGrant(authz.ActionRuntime, "acme") }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}
}

var webhookTestSchema = []string{
	`CREATE TABLE whatsapp_instances(id BIGINT PRIMARY KEY,tenant_id VARCHAR(255) NOT NULL,name VARCHAR(255) NOT NULL,description VARCHAR(255),status VARCHAR(32) NOT NULL,owner_jid VARCHAR(100),profile_pic_url VARCHAR(500),external_attributes TEXT NOT NULL,connection_lock_owner VARCHAR(255),connection_lock_until TIMESTAMP,created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL,UNIQUE(tenant_id,id),UNIQUE(tenant_id,name))`,
	`CREATE TABLE whatsapp_instance_connections(tenant_id VARCHAR(255) NOT NULL,instance_id BIGINT NOT NULL,connection_status VARCHAR(64) NOT NULL,whatsapp_device_jid VARCHAR(100),whatsapp_owner_jid VARCHAR(100),whatsapp_phone_number VARCHAR(32),profile_pic_id VARCHAR(255),last_connected_at TIMESTAMP,last_disconnected_at TIMESTAMP,last_connection_attempt_at TIMESTAMP,last_connection_error VARCHAR(255),last_connection_event VARCHAR(100),connection_attempts BIGINT NOT NULL,created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL,PRIMARY KEY(tenant_id,instance_id),FOREIGN KEY(tenant_id,instance_id)REFERENCES whatsapp_instances(tenant_id,id)ON DELETE CASCADE)`,
	`CREATE TABLE whatsapp_webhooks(id BIGINT PRIMARY KEY,tenant_id VARCHAR(255) NOT NULL,instance_id BIGINT NOT NULL,url VARCHAR(500) NOT NULL,enabled BOOLEAN NOT NULL,events TEXT NOT NULL,created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL,UNIQUE(tenant_id,instance_id),FOREIGN KEY(tenant_id,instance_id)REFERENCES whatsapp_instances(tenant_id,id)ON DELETE CASCADE)`,
	`CREATE TABLE whatsapp_webhook_deliveries(id VARCHAR(36) PRIMARY KEY,tenant_id VARCHAR(255) NOT NULL,instance_id BIGINT NOT NULL,event VARCHAR(100) NOT NULL,target VARCHAR(32) NOT NULL,url VARCHAR(500) NOT NULL,body TEXT NOT NULL,headers TEXT NOT NULL,status VARCHAR(32) NOT NULL,attempts INTEGER NOT NULL,response_status INTEGER,response_body TEXT,last_error TEXT,created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL,delivered_at TIMESTAMP,event_id VARCHAR(255),endpoint_id VARCHAR(255),secret_ref VARCHAR(255) NOT NULL DEFAULT '',permanent BOOLEAN NOT NULL DEFAULT FALSE,claim_token VARCHAR(255),claim_version BIGINT NOT NULL DEFAULT 0,lease_until TIMESTAMP,UNIQUE(tenant_id,id),UNIQUE(tenant_id,event_id,endpoint_id),FOREIGN KEY(tenant_id,instance_id)REFERENCES whatsapp_instances(tenant_id,id)ON DELETE CASCADE)`,
}
