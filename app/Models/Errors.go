package models

import "github.com/hyz-is/arandu-whatsapp/internal/domain"

var (
	// ErrInstanceNameAlreadyExists means the tenant already owns the requested name.
	ErrInstanceNameAlreadyExists = domain.ErrInstanceNameAlreadyExists
	// ErrInstanceNotFound means the requested instance does not exist in the Grant tenant.
	ErrInstanceNotFound = domain.ErrInstanceNotFound
	// ErrInstanceHasDependencies means owned records prevent instance deletion.
	ErrInstanceHasDependencies = domain.ErrInstanceHasDependencies
	// ErrInvalidCursor means a page cursor is malformed or outside the scoped result set.
	ErrInvalidCursor = domain.ErrInvalidCursor
	// ErrInvalidInput means an operation received invalid input.
	ErrInvalidInput = domain.ErrInvalidInput
	// ErrInvalidJSON means a JSON field is syntactically invalid or has the wrong shape.
	ErrInvalidJSON = domain.ErrInvalidJSON
	// ErrInvalidEnum means an enum value is unsupported.
	ErrInvalidEnum = domain.ErrInvalidEnum
	// ErrInvalidWebhookEvent means a webhook event name is unsupported.
	ErrInvalidWebhookEvent = domain.ErrInvalidWebhookEvent
	// ErrInvalidWebhookURL means a webhook destination is not acceptable.
	ErrInvalidWebhookURL = domain.ErrInvalidWebhookURL
	// ErrMessageNotFound means the requested persisted message does not exist.
	ErrMessageNotFound = domain.ErrMessageNotFound
	// ErrWebhookAlreadyExists means the instance already owns a webhook configuration.
	ErrWebhookAlreadyExists = domain.ErrWebhookAlreadyExists
	// ErrWebhookNotFound means the requested webhook configuration does not exist.
	ErrWebhookNotFound = domain.ErrWebhookNotFound
	// ErrWhatsAppDeviceAlreadyLinked means the device belongs to another instance.
	ErrWhatsAppDeviceAlreadyLinked = domain.ErrWhatsAppDeviceAlreadyLinked
)

// InstanceDependenciesError describes owned records preventing instance deletion.
type InstanceDependenciesError = domain.InstanceDependenciesError

const (
	// DefaultInstancePageLimit is used when an instance query omits its limit.
	DefaultInstancePageLimit = domain.DefaultInstancePageLimit
	// MaxInstancePageLimit is the largest accepted instance page.
	MaxInstancePageLimit = domain.MaxInstancePageLimit
)
