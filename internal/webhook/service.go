package webhook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/cache"
	hlog "github.com/arandu-io/hesape/log"

	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

type Service interface {
	Set(ctx context.Context, grant security.Grant, instanceName string, input SetInput) (types.Webhook, error)
	Find(ctx context.Context, grant security.Grant, instanceName string) (types.Webhook, error)
}

type SetInput struct {
	URL       string
	Enabled   *bool
	Events    map[string]bool
	EventsSet bool
}

type WebhookService struct {
	db               *data.DB
	instances        repository.InstanceRepository
	webhooks         repository.WebhookRepository
	configCache      configurationCache
	signingAvailable bool
}

// NewService builds the webhook configuration service. The database handle is
// the one Set opens its transaction on, so that the row and the events it was
// asked for are stored together or not at all.
//
// The cache is the one the dispatcher reads configurations from. Passing the
// same repository here is what makes a configuration written through Set take
// effect on the next event instead of at the end of the cache window; passing
// nil is correct for a service that only reads.
func NewService(
	db *data.DB,
	instances repository.InstanceRepository,
	webhooks repository.WebhookRepository,
	configCache *cache.Repository,
	configCacheTTL time.Duration,
	signingAvailable bool,
) *WebhookService {
	if configCacheTTL == 0 {
		configCacheTTL = DefaultConfigurationCacheTTL
	}
	return &WebhookService{
		db:               db,
		instances:        instances,
		webhooks:         webhooks,
		configCache:      newConfigurationCache(configCache, configCacheTTL),
		signingAvailable: signingAvailable,
	}
}

func (s *WebhookService) Set(ctx context.Context, grant security.Grant, instanceName string, input SetInput) (types.Webhook, error) {
	name, err := normalizeInstanceName(instanceName)
	if err != nil {
		return types.Webhook{}, err
	}
	webhookURL, err := normalizeWebhookURL(input.URL)
	if err != nil {
		return types.Webhook{}, err
	}
	if err := validateEvents(input.Events); err != nil {
		return types.Webhook{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if enabled && !s.signingAvailable {
		return types.Webhook{}, fmt.Errorf("%w: webhook signing secret is not configured", repository.ErrInvalidInput)
	}

	// The row and its events are one configuration. Written apart, a failure
	// between them leaves a webhook enabled against the events it used to have,
	// which is a delivery the caller did not ask for. data.Transaction is
	// reentrant per handle, so the transactions the repositories open inside
	// join this one instead of standing alone.
	var output types.Webhook
	err = s.transaction(ctx, func(txCtx context.Context) error {
		instance, err := s.instances.FindByName(txCtx, grant, name)
		if err != nil {
			return err
		}

		current, err := s.webhooks.FindByInstanceName(txCtx, grant, name)
		if err != nil {
			if !errors.Is(err, repository.ErrWebhookNotFound) {
				return err
			}
			current, err = s.webhooks.Create(txCtx, grant, types.CreateWebhookInput{
				URL:        webhookURL,
				Enabled:    &enabled,
				InstanceID: instance.Instance.ID,
			})
		} else {
			current, err = s.webhooks.Update(txCtx, grant, current.ID, types.UpdateWebhookInput{
				URL:     &webhookURL,
				Enabled: &enabled,
			})
		}
		if err != nil {
			return err
		}

		if input.EventsSet {
			current, err = s.webhooks.UpsertEvents(txCtx, grant, current.ID, input.Events)
			if err != nil {
				return err
			}
		}

		output = current
		return nil
	})
	if err != nil {
		return types.Webhook{}, err
	}

	// The dispatcher may still be holding what this row said a moment ago.
	s.configCache.forget(ctx, grant, output.InstanceID)

	hlog.For(ctx).InfoContext(ctx, "webhook configured",
		"component", "webhook_service",
		"operation", "webhook.set",
		"instance_name", name,
		"webhook_id", output.ID,
	)

	return output, nil
}

// transaction runs fn inside a transaction on the service's handle. A service
// built without one -- which only a test does -- runs fn directly rather than
// pretending the writes were grouped.
func (s *WebhookService) transaction(ctx context.Context, fn func(context.Context) error) error {
	if s.db == nil {
		return fn(ctx)
	}
	return data.Transaction(ctx, s.db, fn)
}

func (s *WebhookService) Find(ctx context.Context, grant security.Grant, instanceName string) (types.Webhook, error) {
	name, err := normalizeInstanceName(instanceName)
	if err != nil {
		return types.Webhook{}, err
	}

	if _, err := s.instances.FindByName(ctx, grant, name); err != nil {
		return types.Webhook{}, err
	}
	output, err := s.webhooks.FindByInstanceName(ctx, grant, name)
	if err != nil {
		return types.Webhook{}, err
	}
	return output, nil
}

func normalizeInstanceName(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" || len(normalized) > 255 {
		return "", repository.ErrInvalidInput
	}
	return normalized, nil
}

func normalizeWebhookURL(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" || len(normalized) > MaxURLLength {
		return "", repository.ErrInvalidWebhookURL
	}
	normalized, err := NormalizeURL(normalized)
	if err != nil {
		return "", repository.ErrInvalidWebhookURL
	}
	return normalized, nil
}

func validateEvents(events map[string]bool) error {
	for event := range events {
		if !types.IsWebhookEventField(event) {
			return fmt.Errorf("%w: %s", repository.ErrInvalidWebhookEvent, event)
		}
	}
	return nil
}
