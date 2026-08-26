package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/queue/jobs"
	_ "modernc.org/sqlite"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

func TestMessageProcessingManagerDurablyQueuesOnlyTheSnapshotID(t *testing.T) {
	db := newMessageQueueTestDB(t, "durable", true)
	manager, err := NewMessageProcessingManager(db, &MessageService{}, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	largePayload := []byte(strings.Repeat("p", jobs.MaxPayload*2))
	grant := security.SystemGrant(authz.ActionMessageSend, "acme")
	processID, err := manager.enqueue(context.Background(), grant, preparedMentionAllJob{
		Instance:       dbtypes.Instance{ID: 41, Name: "sales"},
		RemoteJID:      "120363000000000000@g.us",
		MessageID:      "message-1",
		MessageType:    "extendedTextMessage",
		MessagePayload: largePayload,
		Content:        json.RawMessage(`{"text":"hello"}`),
		WebhookInstance: webhooksvc.WebhookInstance{
			ID: 41, Name: "sales", ExternalAttributes: map[string]any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if processID == "" {
		t.Fatal("enqueue returned an empty process id")
	}

	var storedPayload string
	if err := db.QueryRowContext(context.Background(),
		`SELECT message_payload FROM whatsapp_message_jobs WHERE process_id = ?`, processID).
		Scan(&storedPayload); err != nil {
		t.Fatal(err)
	}
	if len(storedPayload) <= jobs.MaxPayload {
		t.Fatalf("snapshot payload length = %d, want proof it can exceed the queue limit", len(storedPayload))
	}

	var queueName, jobName, tenant, action, rawPayload string
	if err := db.QueryRowContext(context.Background(),
		`SELECT queue, name, tenant_id, action, payload FROM jobs WHERE name = ?`, MessageProcessingJobName).
		Scan(&queueName, &jobName, &tenant, &action, &rawPayload); err != nil {
		t.Fatal(err)
	}
	if queueName != MessageQueueName || jobName != MessageProcessingJobName ||
		tenant != "acme" || action != string(authz.ActionMessageSend) {
		t.Fatalf("unexpected queued job queue=%q name=%q tenant=%q action=%q",
			queueName, jobName, tenant, action)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["processId"] != processID {
		t.Fatalf("job payload = %#v, want only processId %q", payload, processID)
	}
	var jobsCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM jobs`).Scan(&jobsCount); err != nil {
		t.Fatal(err)
	}
	if jobsCount != 2 {
		t.Fatalf("jobs count = %d, want message and cleanup jobs", jobsCount)
	}
}

func TestMessageProcessingManagerRollsBackSnapshotWhenQueuePushFails(t *testing.T) {
	db := newMessageQueueTestDB(t, "rollback", false)
	manager, err := NewMessageProcessingManager(db, &MessageService{}, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.enqueue(context.Background(), security.SystemGrant(authz.ActionMessageSend, "acme"), preparedMentionAllJob{
		Instance:       dbtypes.Instance{ID: 41, Name: "sales"},
		RemoteJID:      "120363000000000000@g.us",
		MessageID:      "message-1",
		MessageType:    "extendedTextMessage",
		MessagePayload: []byte("payload"),
		Content:        json.RawMessage(`{"text":"hello"}`),
		WebhookInstance: webhooksvc.WebhookInstance{
			ID: 41, Name: "sales", ExternalAttributes: map[string]any{},
		},
	})
	if err == nil {
		t.Fatal("enqueue succeeded without the native jobs table")
	}
	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM whatsapp_message_jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("snapshot count = %d after queue failure, want 0", count)
	}
}

func TestMessageProcessingManagerRegistersHandlerAndTreatsMissingSnapshotAsComplete(t *testing.T) {
	db := newMessageQueueTestDB(t, "registration", true)
	manager, err := NewMessageProcessingManager(db, &MessageService{}, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RegisterJobHandlers(nil); err == nil {
		t.Fatal("RegisterJobHandlers accepted nil")
	}
	worker := queue.NewWorker(queue.NullQueue{}, queue.WorkerOptions{})
	if err := manager.RegisterJobHandlers(worker); err != nil {
		t.Fatal(err)
	}
	if err := manager.RegisterJobHandlers(worker); err == nil {
		t.Fatal("RegisterJobHandlers accepted duplicate registration")
	}
	handler, ok := worker.Handler(MessageProcessingJobName)
	if !ok {
		t.Fatalf("handler %q was not registered", MessageProcessingJobName)
	}
	if _, ok := worker.Handler(MessageProcessingCleanupJobName); !ok {
		t.Fatalf("handler %q was not registered", MessageProcessingCleanupJobName)
	}
	grant := security.SystemGrant(authz.ActionMessageSend, "acme")
	job, err := jobs.New(grant, MessageQueueName, MessageProcessingJobName,
		messageProcessingJobPayload{ProcessID: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), grant, &job); err != nil {
		t.Fatalf("missing snapshot was not idempotent: %v", err)
	}
}

func TestMessageProcessingHandlerRejectsGrantBeforeDatabaseAccess(t *testing.T) {
	manager, err := NewMessageProcessingManager(data.Wrap(nil, data.DialectSQLite), &MessageService{}, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{Name: MessageProcessingJobName, Payload: []byte(`{"processId":"one"}`)}
	err = manager.handle(context.Background(), security.SystemGrant(authz.ActionRuntime, "acme"), job)
	if !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("wrong action reached the nil database: %v", err)
	}
}

func TestTerminalFailurePreservesSnapshotWhenWebhookDispatchFails(t *testing.T) {
	db := newMessageQueueTestDB(t, "terminal-cleanup", false)
	webhooks := &fakeMessageWebhooks{err: errors.New("dispatch unavailable")}
	manager, err := NewMessageProcessingManager(db, &MessageService{webhooks: webhooks}, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	grant := security.SystemGrant(authz.ActionMessageSend, "acme")
	snapshot := messageProcessingSnapshot{
		ProcessID:      "process-terminal",
		InstanceID:     41,
		InstanceName:   "sales",
		RemoteJID:      "120363000000000000@g.us",
		MessageID:      "message-terminal",
		MessageType:    "extendedTextMessage",
		MessagePayload: []byte("payload"),
		Content:        json.RawMessage(`{"text":"hello"}`),
		WebhookInstance: webhooksvc.WebhookInstance{
			ID: 41, Name: "sales", ExternalAttributes: map[string]any{},
		},
	}
	if err := manager.repository.Create(context.Background(), grant, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := manager.finalizeFailure(context.Background(), grant, snapshot, ErrSendFailed); err == nil {
		t.Fatal("finalizeFailure ignored webhook error")
	}
	if webhooks.calls != 1 {
		t.Fatalf("webhook calls = %d, want 1", webhooks.calls)
	}
	var count int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM whatsapp_message_jobs WHERE process_id = ?`, snapshot.ProcessID).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("snapshot count = %d after terminal webhook failure, want 1", count)
	}
}

func TestTerminalFailureWithDurableWebhookCompletesWithoutDeadLetter(t *testing.T) {
	db := newMessageQueueTestDB(t, "terminal-complete", true)
	webhooks := &fakeMessageWebhooks{}
	service := &MessageService{
		instances: stubInstanceRepository{item: dbtypes.InstanceRecord{Instance: dbtypes.Instance{
			ID: 42, Name: "sales", Status: dbtypes.InstanceStatusOnline,
		}}},
		webhooks: webhooks,
	}
	manager, err := NewMessageProcessingManager(db, service, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	service.SetProcessor(manager)
	grant := security.SystemGrant(authz.ActionMessageSend, "acme")
	if _, err := manager.enqueue(context.Background(), grant, preparedMentionAllJob{
		Instance: dbtypes.Instance{ID: 41, Name: "sales"}, RemoteJID: "120363000000000000@g.us",
		MessageID: "message-terminal-complete", MessageType: "extendedTextMessage",
		MessagePayload: []byte("payload"), Content: json.RawMessage(`{"text":"hello"}`),
		WebhookInstance: webhooksvc.WebhookInstance{ID: 41, Name: "sales"},
	}); err != nil {
		t.Fatal(err)
	}
	worker := queue.NewWorker(manager.queue, queue.WorkerOptions{Queue: MessageQueueName})
	if err := manager.RegisterJobHandlers(worker); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunNextJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if webhooks.calls != 1 {
		t.Fatalf("terminal webhook calls = %d, want 1", webhooks.calls)
	}
	var snapshots, jobsCount, failedJobs int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM whatsapp_message_jobs`).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM jobs`).Scan(&jobsCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM jobs WHERE failed_at IS NOT NULL`).Scan(&failedJobs); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 || jobsCount != 0 || failedJobs != 0 {
		t.Fatalf("terminal completion left snapshots=%d jobs=%d failed_jobs=%d", snapshots, jobsCount, failedJobs)
	}
}

func TestMessageProcessingCleanupDeletesMainJobAndSnapshot(t *testing.T) {
	db := newMessageQueueTestDB(t, "retention-cleanup", true)
	manager, err := NewMessageProcessingManager(db, &MessageService{}, ProcessingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	grant := security.SystemGrant(authz.ActionMessageSend, "acme")
	processID, err := manager.enqueue(context.Background(), grant, preparedMentionAllJob{
		Instance: dbtypes.Instance{ID: 41, Name: "sales"}, RemoteJID: "120363000000000000@g.us",
		MessageID: "message-cleanup", MessageType: "extendedTextMessage", MessagePayload: []byte("payload"),
		Content: json.RawMessage(`{"text":"hello"}`), WebhookInstance: webhooksvc.WebhookInstance{ID: 41, Name: "sales"},
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobs.New(grant, MessageQueueName, MessageProcessingCleanupJobName, messageProcessingJobPayload{ProcessID: processID})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.handleCleanup(context.Background(), grant, &job); err != nil {
		t.Fatal(err)
	}
	var snapshots, mainJobs int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM whatsapp_message_jobs`).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM jobs WHERE name = ?`, MessageProcessingJobName).Scan(&mainJobs); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 || mainJobs != 0 {
		t.Fatalf("cleanup left snapshots=%d main_jobs=%d", snapshots, mainJobs)
	}
}

func TestExpiredMessageProcessingJobIsRemovedBeforeWhatsAppAccess(t *testing.T) {
	db := newMessageQueueTestDB(t, "expired-before-processing", true)
	manager, err := NewMessageProcessingManager(db, &MessageService{}, ProcessingConfig{Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	grant := security.SystemGrant(authz.ActionMessageSend, "acme")
	processID, err := manager.enqueue(context.Background(), grant, preparedMentionAllJob{
		Instance: dbtypes.Instance{ID: 41, Name: "sales"}, RemoteJID: "120363000000000000@g.us",
		MessageID: "message-expired", MessageType: "extendedTextMessage", MessagePayload: []byte("payload"),
		Content: json.RawMessage(`{"text":"hello"}`), WebhookInstance: webhooksvc.WebhookInstance{ID: 41, Name: "sales"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(),
		`UPDATE whatsapp_message_jobs SET created_at = ? WHERE process_id = ?`,
		time.Now().UTC().Add(-2*time.Hour), processID); err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{Name: MessageProcessingJobName, Payload: []byte(`{"processId":"` + processID + `"}`)}
	if err := manager.handle(context.Background(), grant, job); err != nil {
		t.Fatal(err)
	}
	var snapshots, queued int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM whatsapp_message_jobs`).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM jobs`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 || queued != 0 {
		t.Fatalf("expired work left snapshots=%d jobs=%d", snapshots, queued)
	}
}

func TestMentionAllWorkerResolvesConnectedSessionAtExecution(t *testing.T) {
	resolved := false
	service := &MessageService{
		instances: stubInstanceRepository{InstanceRepository: nil, item: dbtypes.InstanceRecord{Instance: dbtypes.Instance{
			ID: 41, Name: "sales", Status: dbtypes.InstanceStatusOnline,
		}}},
		messages: stubMessageRepository{MessageRepository: nil, err: repository.ErrMessageNotFound},
		clients: connectedResolverFunc(func(context.Context, security.Grant, string) (*whatsapp.ManagedWhatsAppClient, error) {
			resolved = true
			return nil, nil
		}),
	}
	_, err := service.processMentionAllJob(context.Background(),
		security.SystemGrant(authz.ActionMessageSend, "acme"), messageProcessingSnapshot{
			ProcessID: "process-1", InstanceID: 41, InstanceName: "sales",
			RemoteJID: "120363000000000000@g.us", MessageID: "message-1",
		}, DefaultProcessingConfig())
	if !errors.Is(err, whatsapp.ErrClientNotConnected) {
		t.Fatalf("processMentionAllJob error = %v, want disconnected client", err)
	}
	if !resolved {
		t.Fatal("worker did not resolve the connected session at execution time")
	}
}

type stubInstanceRepository struct {
	repository.InstanceRepository
	item dbtypes.InstanceRecord
}

func (s stubInstanceRepository) FindByName(context.Context, security.Grant, string) (dbtypes.InstanceRecord, error) {
	return s.item, nil
}

type stubMessageRepository struct {
	repository.MessageRepository
	item dbtypes.Message
	err  error
}

func (s stubMessageRepository) FindByKeyIDForInstance(context.Context, security.Grant, int64, string) (dbtypes.Message, error) {
	return s.item, s.err
}

type connectedResolverFunc func(context.Context, security.Grant, string) (*whatsapp.ManagedWhatsAppClient, error)

func (f connectedResolverFunc) ResolveConnectedClient(ctx context.Context, grant security.Grant, name string) (*whatsapp.ManagedWhatsAppClient, error) {
	return f(ctx, grant, name)
}

func newMessageQueueTestDB(t *testing.T, name string, withJobs bool) *data.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:message-processing-"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	raw.SetMaxOpenConns(1)
	statements := []string{
		`CREATE TABLE whatsapp_message_jobs (
			process_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, message_job_id TEXT NOT NULL,
			cleanup_job_id TEXT NOT NULL, instance_id INTEGER NOT NULL,
			instance_name TEXT NOT NULL, remote_jid TEXT NOT NULL, message_id TEXT NOT NULL,
			message_type TEXT NOT NULL, message_payload TEXT NOT NULL, content TEXT NOT NULL,
			presence TEXT, delay_ms INTEGER NOT NULL, external_attributes TEXT NOT NULL,
			webhook_instance TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL)`,
	}
	if withJobs {
		statements = append(statements, `CREATE TABLE jobs (
			id TEXT PRIMARY KEY, queue TEXT NOT NULL, name TEXT NOT NULL, display_name TEXT,
			tenant_id TEXT NOT NULL, payload TEXT NOT NULL, authorized_by TEXT NOT NULL,
			action TEXT NOT NULL, run_at TIMESTAMP NOT NULL, reserved_until TIMESTAMP,
			attempts INTEGER NOT NULL DEFAULT 0, exceptions INTEGER NOT NULL DEFAULT 0,
			attributes TEXT, failed_at TIMESTAMP, last_error TEXT, created_at TIMESTAMP)`)
	}
	for _, statement := range statements {
		if _, err := raw.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return data.Wrap(raw, data.DialectSQLite)
}
