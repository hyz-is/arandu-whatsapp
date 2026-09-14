package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/cache"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/str"
	hesapewebhook "github.com/arandu-io/hesape/webhook"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const (
	defaultHTTPTimeout     = 15 * time.Second
	minimumSigningKeySize  = 32
	webhookUserAgent       = "Arandu-WhatsApp/1.0"
	signatureHeader        = "X-Arandu-Signature"
	timestampHeader        = "X-Arandu-Timestamp"
	endpointTargetInstance = "instance"
	endpointTargetGlobal   = "global"

	// DefaultDeliveryRetention bounds durable snapshot lifetime by default.
	DefaultDeliveryRetention = 30 * 24 * time.Hour
	// MaxURLLength is the storage and validation limit for webhook URLs.
	MaxURLLength = 500
	// WebhookQueueName is the queue an application's worker must drain.
	WebhookQueueName = "whatsapp-webhooks"
	// WebhookDeliveryJobName is the stable handler registered with Hesape.
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

// WebhookManager is the module-facing event dispatcher contract.
type WebhookManager interface {
	Dispatch(ctx context.Context, grant security.Grant, instance WebhookInstance, event types.WebhookEvent, data any) error
}

// ManagerConfig configures global selection, retention and the HTTP test seam.
type ManagerConfig struct {
	// GlobalURL receives every supported event when GlobalEnabled is true.
	GlobalURL string
	// GlobalEnabled enables the module-wide webhook target.
	GlobalEnabled bool
	// SigningSecret is the HMAC-SHA256 key shared with webhook consumers.
	SigningSecret string
	// Retention bounds durable snapshot lifetime. Zero uses the safe default.
	Retention time.Duration
	// ConfigurationCache reuses per-instance configuration reads.
	ConfigurationCache *cache.Repository
	// ConfigurationCacheTTL bounds how long a cached configuration is reused.
	ConfigurationCacheTTL time.Duration
	// HTTPClient is a test seam; nil retains Hesape's guarded transport.
	HTTPClient *http.Client
}

// Manager retains WhatsApp endpoint selection and delegates delivery to Hesape.
type Manager struct {
	repository       configurationRepository
	delivery         *hesapewebhook.Manager
	globalURL        string
	globalEnabled    bool
	signingAvailable bool
	configCache      configurationCache
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
	if cfg.ConfigurationCacheTTL < 0 {
		return nil, errors.New("webhook: configuration cache TTL cannot be negative")
	}
	configTTL := cfg.ConfigurationCacheTTL
	if configTTL == 0 {
		configTTL = DefaultConfigurationCacheTTL
	}
	store := newSQLDeliveryStore(db)
	delivery, err := hesapewebhook.NewManagerWithStore(db, store, hesapewebhook.NewStaticSecret(secret), hesapewebhook.ManagerOptions{
		Timeout: defaultHTTPTimeout, Retention: retention, Action: authz.ActionRuntime,
		QueueName: WebhookQueueName, DeliveryJobName: WebhookDeliveryJobName,
		UserAgent: webhookUserAgent, HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("webhook: build delivery engine: %w", err)
	}
	return &Manager{repository: store, delivery: delivery, globalURL: globalURL,
		globalEnabled: cfg.GlobalEnabled, signingAvailable: len(secret) > 0,
		configCache: newConfigurationCache(cfg.ConfigurationCache, configTTL)}, nil
}

// RegisterJobHandlers registers the stable WhatsApp delivery handler.
func (m *Manager) RegisterJobHandlers(worker *queue.Worker) error {
	return m.delivery.RegisterJobHandlers(worker)
}

// Dispatch selects WhatsApp targets once, then dispatches one shared event.
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
	payload := WebhookPayload{Event: event, Instance: instance, Data: payloadData, Timestamp: time.Now().UTC()}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serialize webhook payload: %w", err)
	}
	headers := webhookHeaders(requestID, instance, event)
	endpoints := make([]hesapewebhook.Endpoint, 0, 2)
	var result error
	configured, err := m.configCache.lookup(ctx, runtimeGrant, instance.ID, func(inner context.Context) (types.Webhook, error) {
		return m.repository.FindConfiguration(inner, runtimeGrant, instance.ID)
	})
	switch {
	case err == nil && configured.Enabled:
		events, parseErr := types.ParseWebhookEvents(configured.Events)
		if parseErr != nil {
			result = errors.Join(result, fmt.Errorf("parse webhook configuration events: %w", parseErr))
		} else if events.IsEnabled(event) {
			url, normalizeErr := NormalizeURL(configured.URL)
			if normalizeErr != nil {
				result = errors.Join(result, fmt.Errorf("normalize configured webhook URL: %w", normalizeErr))
			} else {
				endpoints = append(endpoints, hesapewebhook.Endpoint{ID: deliveryEndpointID(endpointTargetInstance, instance.ID), URL: url, Headers: headers})
			}
		}
	case err != nil && !errors.Is(err, errWebhookConfigurationNotFound):
		result = errors.Join(result, err)
	}
	if m.globalEnabled {
		endpoints = append(endpoints, hesapewebhook.Endpoint{ID: deliveryEndpointID(endpointTargetGlobal, instance.ID), URL: m.globalURL, Headers: headers})
	}
	if len(endpoints) == 0 {
		return result
	}
	if !m.signingAvailable {
		return errors.Join(result, ErrSigningSecretRequired)
	}
	eventID, err := data.NewID()
	if err != nil {
		return errors.Join(result, fmt.Errorf("create webhook event id: %w", err))
	}
	err = m.delivery.Dispatch(ctx, runtimeGrant, hesapewebhook.Event{ID: eventID, Name: string(event),
		Aggregate: "whatsapp.instance", AggregateID: strconv.FormatInt(instance.ID, 10), Payload: body, OccurredAt: payload.Timestamp}, endpoints)
	return errors.Join(result, err)
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
	if len(normalized) > MaxURLLength {
		return "", ErrInvalidWebhookURL
	}
	parsed, err := hesapewebhook.NormalizeURL(normalized)
	if err != nil {
		return "", ErrInvalidWebhookURL
	}
	return parsed, nil
}

func webhookHeaders(requestID string, instance WebhookInstance, event types.WebhookEvent) map[string]string {
	ownerJID := ""
	if instance.OwnerJID != nil {
		ownerJID = *instance.OwnerJID
	}
	return map[string]string{"Content-Type": "application/json", "User-Agent": webhookUserAgent,
		"x-request-id": requestID, "x-owner-jid": ownerJID, "x-instance-name": instance.Name,
		"x-instance-id": strconv.FormatInt(instance.ID, 10), "x-webhook-event": string(event)}
}

func deliveryEndpointID(target string, instanceID int64) string {
	return target + ":" + strconv.FormatInt(instanceID, 10)
}
