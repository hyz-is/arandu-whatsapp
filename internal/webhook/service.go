package webhook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arandu-io/framework/security"
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
	instances        repository.InstanceRepository
	webhooks         repository.WebhookRepository
	signingAvailable bool
}

func NewService(
	instances repository.InstanceRepository,
	webhooks repository.WebhookRepository,
	signingAvailable bool,
) *WebhookService {
	return &WebhookService{
		instances:        instances,
		webhooks:         webhooks,
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

	instance, err := s.instances.FindByName(ctx, grant, name)
	if err != nil {
		return types.Webhook{}, err
	}

	output, err := s.webhooks.FindByInstanceName(ctx, grant, name)
	if err != nil {
		if !errors.Is(err, repository.ErrWebhookNotFound) {
			return types.Webhook{}, err
		}
		output, err = s.webhooks.Create(ctx, grant, types.CreateWebhookInput{
			URL:        webhookURL,
			Enabled:    &enabled,
			InstanceID: instance.Instance.ID,
		})
	} else {
		output, err = s.webhooks.Update(ctx, grant, output.ID, types.UpdateWebhookInput{
			URL:     &webhookURL,
			Enabled: &enabled,
		})
	}
	if err != nil {
		return types.Webhook{}, err
	}
	if input.EventsSet {
		output, err = s.webhooks.UpsertEvents(ctx, grant, output.ID, input.Events)
		if err != nil {
			return types.Webhook{}, err
		}
	}
	hlog.For(ctx).InfoContext(ctx, "webhook configured",
		"component", "webhook_service",
		"operation", "webhook.set",
		"instance_name", name,
		"webhook_id", output.ID,
	)

	return output, nil
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
