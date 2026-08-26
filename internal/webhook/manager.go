package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	httpclient "github.com/arandu-io/hesape/http/client"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/queue/jobs"
	"github.com/arandu-io/hesape/str"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const (
	defaultHTTPTimeout    = 15 * time.Second
	terminalWriteTimeout  = 3 * time.Second
	defaultMaxRedirects   = 5
	minimumSigningKeySize = 32
	deliveryPruneInterval = time.Hour
	webhookUserAgent      = "Arandu-WhatsApp/1.0"
	deliveryMaxTries      = 5
	signatureHeader       = "X-Arandu-Signature"
	timestampHeader       = "X-Arandu-Timestamp"

	// DefaultDeliveryRetention is the maximum age of a durable delivery
	// snapshot when the application does not choose a shorter window.
	DefaultDeliveryRetention = 30 * 24 * time.Hour
	// MaxURLLength is the storage and validation limit for webhook URLs.
	MaxURLLength = 500

	// WebhookQueueName is the queue an application's worker must drain.
	WebhookQueueName = "whatsapp-webhooks"
	// WebhookDeliveryJobName is the stable handler name registered with Hesape.
	WebhookDeliveryJobName = "whatsapp.webhook.deliver"
)

var (
	// ErrInvalidWebhookURL means a webhook target is absent or unusable.
	ErrInvalidWebhookURL = errors.New("invalid webhook URL")
	// ErrUnsupportedEvent means the dispatcher received no public event contract.
	ErrUnsupportedEvent = errors.New("unsupported webhook event")
	// ErrSigningSecretRequired means delivery was enabled without an HMAC key.
	ErrSigningSecretRequired = errors.New("webhook signing secret is required")
	// ErrSigningSecretTooShort means the configured HMAC key is below 32 bytes.
	ErrSigningSecretTooShort = errors.New("webhook signing secret must contain at least 32 bytes")
)

// WebhookManager persists webhook delivery snapshots and queues their ids.
type WebhookManager interface {
	Dispatch(ctx context.Context, grant security.Grant, instance WebhookInstance, event types.WebhookEvent, data any) error
}

// ManagerConfig configures the global target and the guarded HTTP test seam.
type ManagerConfig struct {
	// GlobalURL receives every supported event when GlobalEnabled is true.
	GlobalURL string
	// GlobalEnabled enables the module-wide webhook target.
	GlobalEnabled bool
	// SigningSecret is the HMAC-SHA256 key shared with every webhook consumer.
	SigningSecret string
	// Retention bounds durable snapshot lifetime. Zero uses the safe default.
	Retention time.Duration
	// HTTPClient is a test seam. Production leaves it nil so the Hesape client
	// factory owns the guarded transport.
	HTTPClient *http.Client
}

type deliveryJobPayload struct {
	DeliveryID string `json:"deliveryId"`
}

// Manager snapshots deliveries in SQL and dispatches their ids through the
// application's native database queue.
type Manager struct {
	db            *data.DB
	repository    deliveryRepository
	queue         *queue.DatabaseQueue
	globalURL     string
	globalEnabled bool
	signingSecret []byte
	retention     time.Duration
	client        *http.Client
	pruneMu       sync.Mutex
	lastPrune     map[string]time.Time
}

// NewManager returns a webhook manager over the host database.
func NewManager(db *data.DB, cfg ManagerConfig) (*Manager, error) {
	if db == nil {
		return nil, errors.New("webhook: NewManager needs a database handle")
	}
	globalURL := strings.TrimSpace(cfg.GlobalURL)
	if globalURL != "" {
		normalized, err := NormalizeURL(globalURL)
		if err != nil {
			return nil, fmt.Errorf("%w: global webhook URL", ErrInvalidWebhookURL)
		}
		globalURL = normalized
	}
	if cfg.GlobalEnabled && globalURL == "" {
		return nil, fmt.Errorf("%w: global webhook enabled without URL", ErrInvalidWebhookURL)
	}
	secret := []byte(strings.TrimSpace(cfg.SigningSecret))
	if len(secret) > 0 && len(secret) < minimumSigningKeySize {
		return nil, ErrSigningSecretTooShort
	}
	if cfg.GlobalEnabled && len(secret) == 0 {
		return nil, ErrSigningSecretRequired
	}
	if cfg.Retention < 0 {
		return nil, errors.New("webhook: delivery retention cannot be negative")
	}
	retention := cfg.Retention
	if retention == 0 {
		retention = DefaultDeliveryRetention
	}

	factory := httpclient.NewFactory(cfg.HTTPClient)
	client := factory.CreatePendingRequest().
		Timeout(defaultHTTPTimeout).
		MaxRedirects(defaultMaxRedirects).
		CreateClient(nil)

	return &Manager{
		db:            db,
		repository:    newSQLDeliveryRepository(db),
		queue:         queue.NewDatabaseQueue(db),
		globalURL:     globalURL,
		globalEnabled: cfg.GlobalEnabled,
		signingSecret: secret,
		retention:     retention,
		client:        client,
		lastPrune:     make(map[string]time.Time),
	}, nil
}

// RegisterJobHandlers registers the native queue handler owned by the manager.
func (m *Manager) RegisterJobHandlers(worker *queue.Worker) error {
	if worker == nil {
		return errors.New("webhook: RegisterJobHandlers needs a worker")
	}
	worker.HandleFunc(WebhookDeliveryJobName, m.handleDelivery)
	return nil
}

// Dispatch snapshots every enabled target and atomically queues its delivery
// id with the row it describes.
func (m *Manager) Dispatch(ctx context.Context, grant security.Grant, instance WebhookInstance, event types.WebhookEvent, payloadData any) error {
	if err := authz.CheckInstanceLookup(grant); err != nil {
		return err
	}
	if !event.IsSupported() {
		return fmt.Errorf("%w: %s", ErrUnsupportedEvent, event)
	}
	runtimeGrant, err := webhookRuntimeGrant(grant)
	if err != nil {
		return err
	}
	requestID := ""
	if collector := hlog.FromContext(ctx); collector != nil {
		requestID = strings.TrimSpace(collector.RequestID)
	}
	if requestID == "" {
		requestID = str.UUID()
	}
	payload := WebhookPayload{
		Event:     event,
		Instance:  instance,
		Data:      payloadData,
		Timestamp: time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serialize webhook payload: %w", err)
	}
	headers := webhookHeaders(requestID, instance, event)

	var result error
	configured, err := m.repository.FindConfiguration(ctx, runtimeGrant, instance.ID)
	switch {
	case err == nil && configured.Enabled:
		events, parseErr := types.ParseWebhookEvents(configured.Events)
		if parseErr != nil {
			result = errors.Join(result, fmt.Errorf("parse webhook configuration events: %w", parseErr))
		} else if events.IsEnabled(event) {
			url, normalizeErr := NormalizeURL(configured.URL)
			if normalizeErr != nil {
				result = errors.Join(result, fmt.Errorf("normalize configured webhook URL: %w", normalizeErr))
			} else if enqueueErr := m.enqueueDelivery(ctx, runtimeGrant, createDeliveryInput{
				InstanceID: instance.ID,
				Event:      event,
				Target:     deliveryTargetInstance,
				URL:        url,
				Body:       body,
				Headers:    headers,
			}); enqueueErr != nil {
				result = errors.Join(result, enqueueErr)
			}
		}
	case err != nil && !errors.Is(err, errWebhookConfigurationNotFound):
		result = errors.Join(result, err)
	}

	if m.globalEnabled {
		if err := m.enqueueDelivery(ctx, runtimeGrant, createDeliveryInput{
			InstanceID: instance.ID,
			Event:      event,
			Target:     deliveryTargetGlobal,
			URL:        m.globalURL,
			Body:       body,
			Headers:    headers,
		}); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (m *Manager) enqueueDelivery(ctx context.Context, grant security.Grant, input createDeliveryInput) error {
	if len(m.signingSecret) == 0 {
		return ErrSigningSecretRequired
	}
	deliveryID, err := data.NewID()
	if err != nil {
		return fmt.Errorf("create webhook delivery id: %w", err)
	}
	input.ID = deliveryID
	input.Headers = cloneHeaders(input.Headers)
	input.Headers["X-Arandu-Delivery-ID"] = deliveryID
	return data.Transaction(ctx, m.db, func(txCtx context.Context) error {
		if err := m.repository.CreateDelivery(txCtx, grant, input); err != nil {
			return err
		}
		job, err := jobs.New(grant, WebhookQueueName, WebhookDeliveryJobName, deliveryJobPayload{DeliveryID: deliveryID})
		if err != nil {
			return err
		}
		job.Attributes.Tries = deliveryMaxTries
		job.Attributes.Backoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
		job.Attributes.Timeout = defaultHTTPTimeout + 5*time.Second
		return m.queue.Push(txCtx, grant, job)
	})
}

func (m *Manager) handleDelivery(ctx context.Context, grant security.Grant, job *jobs.Job) error {
	if err := grant.Check(authz.ActionRuntime); err != nil {
		return err
	}
	if job == nil {
		return errors.New("webhook: delivery job is nil")
	}
	var input deliveryJobPayload
	if err := job.Decode(&input); err != nil {
		return err
	}
	if strings.TrimSpace(input.DeliveryID) == "" {
		return errors.New("webhook: delivery job has no delivery id")
	}
	m.pruneExpired(ctx, grant)
	item, err := m.repository.FindDelivery(ctx, grant, input.DeliveryID)
	if errors.Is(err, errWebhookDeliveryNotFound) {
		hlog.For(ctx).Debug("webhook delivery snapshot no longer exists", "delivery_id", input.DeliveryID)
		return nil
	}
	if err != nil {
		return err
	}
	if item.Status == deliveryStatusDelivered {
		hlog.For(ctx).Debug("webhook delivery already completed", "delivery_id", item.ID)
		return nil
	}
	attempts := job.Attempts
	if attempts < 1 {
		attempts = 1
	}
	if err := m.repository.MarkAttempt(ctx, grant, item.ID, attempts); err != nil {
		if errors.Is(err, errWebhookDeliveryNotFound) {
			return nil
		}
		return err
	}
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, item.URL, bytes.NewReader(item.Body))
	if err != nil {
		return m.failDelivery(ctx, grant, item, attempts, 0, started, err)
	}
	for key, value := range item.Headers {
		request.Header.Set(key, value)
	}
	request.Header.Set("X-Arandu-Delivery-ID", item.ID)
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	request.Header.Set(timestampHeader, timestamp)
	request.Header.Set(signatureHeader, signDelivery(m.signingSecret, timestamp, item.ID, item.Body))

	response, err := m.client.Do(request)
	if err != nil {
		return m.failDelivery(ctx, grant, item, attempts, 0, started, err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return m.failDelivery(ctx, grant, item, attempts, response.StatusCode, started, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		err := fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
		return m.failDelivery(ctx, grant, item, attempts, response.StatusCode, started, err)
	}
	if err := m.repository.MarkDelivered(ctx, grant, item.ID, attempts, response.StatusCode); err != nil {
		if errors.Is(err, errWebhookDeliveryNotFound) {
			return nil
		}
		return err
	}
	hlog.For(ctx).Info("webhook delivered",
		"delivery_id", item.ID,
		"event", item.Event,
		"instance_id", item.InstanceID,
		"target", item.Target,
		"status_code", response.StatusCode,
		"duration_ms", time.Since(started).Milliseconds(),
		"url", safeWebhookURL(item.URL),
	)
	return nil
}

func (m *Manager) failDelivery(ctx context.Context, grant security.Grant, item delivery, attempts, statusCode int, started time.Time, cause error) error {
	reason := deliveryFailureReason(cause, statusCode)
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalWriteTimeout)
	defer cancel()
	recordErr := m.repository.MarkFailed(recordCtx, grant, item.ID, attempts, statusCode, reason)
	if errors.Is(recordErr, errWebhookDeliveryNotFound) {
		hlog.For(ctx).Debug("webhook delivery snapshot disappeared during attempt", "delivery_id", item.ID)
		return nil
	}
	hlog.For(ctx).Error("webhook delivery failed",
		"error_code", reason,
		"delivery_id", item.ID,
		"event", item.Event,
		"instance_id", item.InstanceID,
		"target", item.Target,
		"status_code", statusCode,
		"attempt", attempts,
		"duration_ms", time.Since(started).Milliseconds(),
		"url", safeWebhookURL(item.URL),
	)
	return errors.Join(errors.New(reason), recordErr)
}

func (m *Manager) pruneExpired(ctx context.Context, grant security.Grant) {
	tenant := data.Tenant(grant)
	if tenant == "" || m.retention <= 0 {
		return
	}
	now := time.Now().UTC()
	m.pruneMu.Lock()
	last, ok := m.lastPrune[tenant]
	if ok && now.Sub(last) < deliveryPruneInterval {
		m.pruneMu.Unlock()
		return
	}
	m.lastPrune[tenant] = now
	m.pruneMu.Unlock()

	deleted, err := m.repository.PruneBefore(ctx, grant, now.Add(-m.retention))
	if err != nil {
		m.pruneMu.Lock()
		if m.lastPrune[tenant].Equal(now) {
			delete(m.lastPrune, tenant)
		}
		m.pruneMu.Unlock()
		hlog.For(ctx).Warn("webhook delivery retention failed", "error", err)
		return
	}
	if deleted > 0 {
		hlog.For(ctx).Info("expired webhook delivery snapshots pruned", "count", deleted)
	}
}

func webhookRuntimeGrant(grant security.Grant) (security.Grant, error) {
	tenant := data.Tenant(grant)
	if tenant == "" {
		return security.Grant{}, fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	//arandu:system-grant an authorized event crosses into module-owned webhook delivery
	return security.SystemGrant(authz.ActionRuntime, tenant), nil
}

// NormalizeURL returns a normalized HTTP or HTTPS webhook URL.
func NormalizeURL(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ErrInvalidWebhookURL
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", ErrInvalidWebhookURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidWebhookURL
	}
	return normalized, nil
}

func webhookHeaders(requestID string, instance WebhookInstance, event types.WebhookEvent) map[string]string {
	ownerJID := ""
	if instance.OwnerJID != nil {
		ownerJID = *instance.OwnerJID
	}
	return map[string]string{
		"Content-Type":    "application/json",
		"User-Agent":      webhookUserAgent,
		"x-request-id":    requestID,
		"x-owner-jid":     ownerJID,
		"x-instance-name": instance.Name,
		"x-instance-id":   strconv.FormatInt(instance.ID, 10),
		"x-webhook-event": string(event),
	}
}

func cloneHeaders(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source)+3)
	for name, value := range source {
		cloned[name] = value
	}
	return cloned
}

func signDelivery(secret []byte, timestamp, deliveryID string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write([]byte(deliveryID))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func deliveryFailureReason(cause error, statusCode int) string {
	switch {
	case errors.Is(cause, context.DeadlineExceeded):
		return "request_timeout"
	case errors.Is(cause, context.Canceled):
		return "request_canceled"
	case errors.Is(cause, httpclient.ErrInternalAddress):
		return "unsafe_destination"
	case errors.Is(cause, httpclient.ErrResponseTooLarge):
		return "response_too_large"
	}
	if statusCode > 0 {
		return fmt.Sprintf("http_status_%d", statusCode)
	}
	return "request_failed"
}

func safeWebhookURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return ""
	}
	parsed.User = nil
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
