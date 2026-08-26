package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/queue/jobs"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

const (
	defaultMessageProcessingTimeout = 60 * time.Second
	defaultMessageGroupInfoTimeout  = 30 * time.Second
	defaultMessageSendTimeout       = 30 * time.Second
	messageProcessingMaxTries       = 5
	// DefaultProcessingRetention bounds durable mention-all snapshot lifetime.
	DefaultProcessingRetention = 30 * 24 * time.Hour

	// MessageQueueName is the durable queue an application's worker must drain.
	MessageQueueName = "whatsapp-messages"
	// MessageProcessingJobName is the stable native queue handler name.
	MessageProcessingJobName = "whatsapp.message.mention-all"
	// MessageProcessingCleanupJobName is the durable snapshot cleanup handler name.
	MessageProcessingCleanupJobName = "whatsapp.message.mention-all.cleanup"
)

var errInvalidMessageProcessingSnapshot = errors.New("invalid message processing snapshot")

// ProcessingConfig bounds one durable mention-all execution.
type ProcessingConfig struct {
	// ProcessingTimeout bounds the complete native queue handler.
	ProcessingTimeout time.Duration
	// GroupInfoTimeout bounds participant discovery.
	GroupInfoTimeout time.Duration
	// SendTimeout bounds presence, delay and the WhatsApp send call.
	SendTimeout time.Duration
	// Retention bounds the lifetime of a durable mention-all snapshot.
	Retention time.Duration
}

// DefaultProcessingConfig returns the durable mention-all timeout defaults.
func DefaultProcessingConfig() ProcessingConfig {
	return ProcessingConfig{
		ProcessingTimeout: defaultMessageProcessingTimeout,
		GroupInfoTimeout:  defaultMessageGroupInfoTimeout,
		SendTimeout:       defaultMessageSendTimeout,
		Retention:         DefaultProcessingRetention,
	}
}

type messageProcessingJobPayload struct {
	ProcessID string `json:"processId"`
}

type preparedMentionAllJob struct {
	Instance           dbtypes.Instance
	RemoteJID          string
	MessageID          string
	MessageType        string
	MessagePayload     []byte
	Content            json.RawMessage
	Presence           *string
	Delay              time.Duration
	ExternalAttributes map[string]any
	WebhookInstance    webhooksvc.WebhookInstance
}

// MessageProcessingManager owns durable mention-all snapshots and their
// native Hesape queue handler. Worker lifecycle belongs to the host application.
type MessageProcessingManager struct {
	db         *data.DB
	service    *MessageService
	config     ProcessingConfig
	repository messageProcessingRepository
	queue      *queue.DatabaseQueue
}

// NewMessageProcessingManager builds the durable mention-all queue adapter
// without starting a process-local worker.
func NewMessageProcessingManager(db *data.DB, service *MessageService, cfg ProcessingConfig) (*MessageProcessingManager, error) {
	if db == nil {
		return nil, errors.New("message processing: database handle is required")
	}
	if service == nil {
		return nil, errors.New("message processing: message service is required")
	}
	if cfg.ProcessingTimeout <= 0 {
		cfg.ProcessingTimeout = defaultMessageProcessingTimeout
	}
	if cfg.GroupInfoTimeout <= 0 {
		cfg.GroupInfoTimeout = defaultMessageGroupInfoTimeout
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = defaultMessageSendTimeout
	}
	if cfg.Retention <= 0 {
		cfg.Retention = DefaultProcessingRetention
	}
	return &MessageProcessingManager{
		db:         db,
		service:    service,
		config:     cfg,
		repository: newSQLMessageProcessingRepository(db),
		queue:      queue.NewDatabaseQueue(db),
	}, nil
}

// RegisterJobHandlers registers the durable mention-all handler explicitly.
func (m *MessageProcessingManager) RegisterJobHandlers(worker *queue.Worker) error {
	if worker == nil {
		return errors.New("message processing: RegisterJobHandlers needs a worker")
	}
	for _, name := range []string{MessageProcessingJobName, MessageProcessingCleanupJobName} {
		if _, exists := worker.Handler(name); exists {
			return fmt.Errorf("message processing: handler %q is already registered", name)
		}
	}
	worker.HandleFunc(MessageProcessingJobName, m.handle)
	worker.HandleFunc(MessageProcessingCleanupJobName, m.handleCleanup)
	return nil
}

func (m *MessageProcessingManager) enqueue(ctx context.Context, grant security.Grant, input preparedMentionAllJob) (string, error) {
	if _, err := messageProcessingTenant(grant); err != nil {
		return "", err
	}
	if input.Instance.ID <= 0 || strings.TrimSpace(input.Instance.Name) == "" ||
		strings.TrimSpace(input.RemoteJID) == "" || strings.TrimSpace(input.MessageID) == "" ||
		strings.TrimSpace(input.MessageType) == "" || len(input.MessagePayload) == 0 || !json.Valid(input.Content) {
		return "", fmt.Errorf("%w: required snapshot field is missing", errInvalidMessageProcessingSnapshot)
	}
	processID, err := data.NewID()
	if err != nil {
		return "", fmt.Errorf("create message processing id: %w", err)
	}
	messageJob, err := jobs.New(grant, MessageQueueName, MessageProcessingJobName, messageProcessingJobPayload{ProcessID: processID})
	if err != nil {
		return "", err
	}
	cleanupJob, err := jobs.New(grant, MessageQueueName, MessageProcessingCleanupJobName, messageProcessingJobPayload{ProcessID: processID})
	if err != nil {
		return "", err
	}
	snapshot := messageProcessingSnapshot{
		ProcessID:          processID,
		MessageJobID:       messageJob.UUID,
		CleanupJobID:       cleanupJob.UUID,
		InstanceID:         input.Instance.ID,
		InstanceName:       input.Instance.Name,
		RemoteJID:          input.RemoteJID,
		MessageID:          input.MessageID,
		MessageType:        input.MessageType,
		MessagePayload:     append([]byte(nil), input.MessagePayload...),
		Content:            append(json.RawMessage(nil), input.Content...),
		Presence:           cloneStringPointer(input.Presence),
		Delay:              input.Delay,
		ExternalAttributes: cloneMap(input.ExternalAttributes),
		WebhookInstance:    input.WebhookInstance,
	}
	messageJob.Attributes.Tries = messageProcessingMaxTries
	messageJob.Attributes.Backoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	messageJob.Attributes.Timeout = m.config.ProcessingTimeout
	cleanupJob.Attributes.Tries = -1
	if err := data.Transaction(ctx, m.db, func(txCtx context.Context) error {
		if err := m.repository.Create(txCtx, grant, snapshot); err != nil {
			return err
		}
		if err := m.queue.Push(txCtx, grant, messageJob); err != nil {
			return err
		}
		return m.queue.Later(txCtx, grant, m.config.Retention, cleanupJob)
	}); err != nil {
		return "", err
	}
	hlog.For(ctx).InfoContext(ctx, "mention-all message queued",
		"component", "message_processor",
		"process_id", processID,
		"instance_name", input.Instance.Name,
		"remote_jid", address.MaskAddress(input.RemoteJID),
		"mention_all", true,
	)
	return processID, nil
}

func (m *MessageProcessingManager) handle(ctx context.Context, grant security.Grant, job *jobs.Job) error {
	if _, err := messageProcessingTenant(grant); err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("%w: job is nil", errInvalidMessageProcessingSnapshot)
	}
	var payload messageProcessingJobPayload
	if err := job.Decode(&payload); err != nil {
		return err
	}
	payload.ProcessID = strings.TrimSpace(payload.ProcessID)
	if payload.ProcessID == "" {
		return fmt.Errorf("%w: process id is missing", errInvalidMessageProcessingSnapshot)
	}
	snapshot, err := m.repository.Find(ctx, grant, payload.ProcessID)
	if errors.Is(err, errMessageProcessingSnapshotNotFound) {
		hlog.For(ctx).DebugContext(ctx, "mention-all snapshot already completed or removed",
			"component", "message_processor", "process_id", payload.ProcessID)
		return nil
	}
	if err != nil {
		if errors.Is(err, errInvalidMessageProcessingSnapshot) {
			messageJobID, cleanupJobID, idsErr := m.repository.FindJobIDs(ctx, grant, payload.ProcessID)
			if idsErr != nil {
				return errors.Join(err, idsErr)
			}
			return m.removeSnapshotAndJobs(ctx, grant, payload.ProcessID, messageJobID, cleanupJobID)
		}
		return err
	}
	if !time.Now().UTC().Before(snapshot.CreatedAt.Add(m.config.Retention)) {
		hlog.For(ctx).InfoContext(ctx, "expired mention-all snapshot removed before processing",
			"component", "message_processor", "process_id", snapshot.ProcessID)
		return m.removeSnapshotAndJobs(ctx, grant, snapshot.ProcessID,
			snapshot.MessageJobID, snapshot.CleanupJobID)
	}
	jobCtx := hlog.With(ctx,
		"process_id", snapshot.ProcessID,
		"instance_name", snapshot.InstanceName,
		"remote_jid", address.MaskAddress(snapshot.RemoteJID),
		"mention_all", true,
	)
	startedAt := time.Now()
	execution, err := m.service.processMentionAllJob(jobCtx, grant, snapshot, m.config)
	if err != nil {
		code := errorCodeForProcessing(err)
		hlog.For(jobCtx).ErrorContext(jobCtx, "mention-all message job failed",
			"error_code", code, "duration", time.Since(startedAt), "attempt", normalizedJobAttempts(job))
		safeErr := fmt.Errorf("message processing failed: %s", code)
		if errors.Is(err, errInvalidMessageProcessingSnapshot) || normalizedJobAttempts(job) >= messageProcessingMaxTries {
			if finalizeErr := m.finalizeFailure(jobCtx, grant, snapshot, err); finalizeErr != nil {
				return errors.Join(safeErr, finalizeErr)
			}
			return nil
		}
		return safeErr
	}
	if err := m.finalizeSuccess(jobCtx, grant, snapshot, execution); err != nil {
		return err
	}
	hlog.For(jobCtx).InfoContext(jobCtx, "mention-all message job completed",
		"duration", time.Since(startedAt), "status", "sent")
	return nil
}

func (m *MessageProcessingManager) finalizeSuccess(ctx context.Context, grant security.Grant, snapshot messageProcessingSnapshot, execution mentionAllExecution) error {
	return data.Transaction(ctx, m.db, func(txCtx context.Context) error {
		persisted := execution.Existing
		if persisted == nil {
			item, err := m.service.persistSentMessage(txCtx, grant, execution.Instance,
				execution.RemoteJID, snapshot.MessageID, execution.SentAt, snapshot.MessageType,
				execution.Content, mentionAllOptions(snapshot.ExternalAttributes))
			if err != nil {
				return err
			}
			persisted = &item
		}
		persisted.ExternalAttributes = cloneMap(snapshot.ExternalAttributes)
		if err := m.service.dispatchMentionAllSuccess(txCtx, grant,
			webhooksvc.NewWebhookInstance(execution.Instance), snapshot.ProcessID,
			*persisted, snapshot.ExternalAttributes); err != nil {
			return err
		}
		if err := m.queue.DeleteJob(txCtx, &jobs.Job{UUID: snapshot.CleanupJobID}); err != nil {
			return err
		}
		return m.repository.Delete(txCtx, grant, snapshot.ProcessID)
	})
}

func (m *MessageProcessingManager) finalizeFailure(ctx context.Context, grant security.Grant, snapshot messageProcessingSnapshot, cause error) error {
	var dispatchErr error
	err := data.Transaction(ctx, m.db, func(txCtx context.Context) error {
		dispatchErr = m.service.dispatchMentionAllFailure(txCtx, grant, snapshot.WebhookInstance,
			snapshot.ProcessID, errorCodeForProcessing(cause), safeProcessingError(cause),
			snapshot.ExternalAttributes)
		if dispatchErr != nil {
			return dispatchErr
		}
		if err := m.queue.DeleteJob(txCtx, &jobs.Job{UUID: snapshot.CleanupJobID}); err != nil {
			return err
		}
		return m.repository.Delete(txCtx, grant, snapshot.ProcessID)
	})
	if dispatchErr != nil {
		hlog.For(ctx).WarnContext(ctx, "mention-all failure webhook dispatch not queued",
			"component", "message_processor", "process_id", snapshot.ProcessID,
			"error_code", "WEBHOOK_DISPATCH_FAILED")
	}
	return err
}

func (m *MessageProcessingManager) handleCleanup(ctx context.Context, grant security.Grant, job *jobs.Job) error {
	if _, err := messageProcessingTenant(grant); err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("%w: cleanup job is nil", errInvalidMessageProcessingSnapshot)
	}
	var payload messageProcessingJobPayload
	if err := job.Decode(&payload); err != nil {
		return err
	}
	payload.ProcessID = strings.TrimSpace(payload.ProcessID)
	if payload.ProcessID == "" {
		return fmt.Errorf("%w: process id is missing", errInvalidMessageProcessingSnapshot)
	}
	messageJobID, _, err := m.repository.FindJobIDs(ctx, grant, payload.ProcessID)
	if errors.Is(err, errMessageProcessingSnapshotNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return data.Transaction(ctx, m.db, func(txCtx context.Context) error {
		if err := m.queue.DeleteJob(txCtx, &jobs.Job{UUID: messageJobID}); err != nil {
			return err
		}
		return m.repository.Delete(txCtx, grant, payload.ProcessID)
	})
}

func (m *MessageProcessingManager) removeSnapshotAndJobs(ctx context.Context, grant security.Grant, processID, messageJobID, cleanupJobID string) error {
	return data.Transaction(ctx, m.db, func(txCtx context.Context) error {
		for _, id := range []string{messageJobID, cleanupJobID} {
			if strings.TrimSpace(id) == "" {
				continue
			}
			if err := m.queue.DeleteJob(txCtx, &jobs.Job{UUID: id}); err != nil {
				return err
			}
		}
		return m.repository.Delete(txCtx, grant, processID)
	})
}

func mentionAllOptions(external map[string]any) *MessageOptions {
	enabled := true
	return &MessageOptions{MentionAll: &enabled, ExternalAttributes: cloneMap(external)}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func normalizedJobAttempts(job *jobs.Job) int {
	if job == nil || job.Attempts < 1 {
		return 1
	}
	return job.Attempts
}

func errorCodeForProcessing(err error) string {
	switch {
	case errors.Is(err, ErrGroupInfoFetchFailed):
		return "GROUP_INFO_FETCH_FAILED"
	case errors.Is(err, ErrGroupHasNoParticipants):
		return "GROUP_HAS_NO_PARTICIPANTS"
	case errors.Is(err, whatsapp.ErrClientNotConnected):
		return "INSTANCE_NOT_CONNECTED"
	case errors.Is(err, ErrSendFailed):
		return "MESSAGE_SEND_FAILED"
	default:
		return "GROUP_MENTION_PROCESSING_FAILED"
	}
}

func safeProcessingError(err error) string {
	switch {
	case errors.Is(err, ErrGroupInfoFetchFailed):
		return "The group participants could not be loaded."
	case errors.Is(err, ErrGroupHasNoParticipants):
		return "The group has no valid participants to mention."
	case errors.Is(err, whatsapp.ErrClientNotConnected):
		return "The instance is not connected."
	case errors.Is(err, ErrSendFailed):
		return "WhatsApp did not accept the message."
	default:
		return "The group message could not be completed."
	}
}
