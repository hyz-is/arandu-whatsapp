// Package domain holds contracts shared by the module's application and
// infrastructure responsibilities.
package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInstanceNotFound means the requested instance does not exist.
	ErrInstanceNotFound = errors.New("instance not found")
	// ErrInstanceNameAlreadyExists means an instance name is already in use.
	ErrInstanceNameAlreadyExists = errors.New("instance name already exists")
	// ErrInstanceHasDependencies means owned records prevent instance deletion.
	ErrInstanceHasDependencies = errors.New("instance has dependencies")
	// ErrWhatsAppDeviceAlreadyLinked means a device belongs to another instance.
	ErrWhatsAppDeviceAlreadyLinked = errors.New("whatsapp device already linked")
	// ErrWebhookAlreadyExists means an instance already owns a webhook.
	ErrWebhookAlreadyExists = errors.New("webhook already exists")
	// ErrMessageNotFound means the requested persisted message does not exist.
	ErrMessageNotFound = errors.New("message not found")
	// ErrInvalidWebhookEvent means a webhook event is unsupported.
	ErrInvalidWebhookEvent = errors.New("invalid webhook event")
	// ErrInvalidWebhookURL means a webhook destination is invalid.
	ErrInvalidWebhookURL = errors.New("invalid webhook url")
	// ErrInvalidJSON means a JSON value is malformed or has the wrong shape.
	ErrInvalidJSON = errors.New("invalid json")
	// ErrInvalidEnum means an enum value is unsupported.
	ErrInvalidEnum = errors.New("invalid enum")
	// ErrInvalidInput means an operation received invalid input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidCursor means a page cursor is invalid.
	ErrInvalidCursor = errors.New("invalid cursor")
	// ErrWebhookNotFound means the requested webhook does not exist.
	ErrWebhookNotFound = errors.New("webhook not found")
)

const (
	// DefaultInstancePageLimit is used when an instance query omits its limit.
	DefaultInstancePageLimit = 200
	// MaxInstancePageLimit is the largest accepted instance page.
	MaxInstancePageLimit = 200
)

// InstanceDependenciesError describes owned records preventing instance deletion.
type InstanceDependenciesError struct {
	InstanceID int64
	Messages   int64
	Chats      int64
	Contacts   int64
	Webhooks   int64
}

// Error returns the dependency counts associated with the instance.
func (e *InstanceDependenciesError) Error() string {
	return fmt.Sprintf("%v: instance %d has %d messages, %d chats, %d contacts and %d webhooks",
		ErrInstanceHasDependencies, e.InstanceID, e.Messages, e.Chats, e.Contacts, e.Webhooks)
}

// Unwrap exposes ErrInstanceHasDependencies for errors.Is checks.
func (e *InstanceDependenciesError) Unwrap() error { return ErrInstanceHasDependencies }
