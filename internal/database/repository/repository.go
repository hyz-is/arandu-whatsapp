package repository

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"

	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
	"github.com/hyz-is/arandu-whatsapp/internal/domain"
)

var (
	ErrInstanceNotFound            = domain.ErrInstanceNotFound
	ErrInstanceNameAlreadyExists   = domain.ErrInstanceNameAlreadyExists
	ErrInstanceHasDependencies     = domain.ErrInstanceHasDependencies
	ErrWhatsAppDeviceAlreadyLinked = domain.ErrWhatsAppDeviceAlreadyLinked
	ErrWebhookAlreadyExists        = domain.ErrWebhookAlreadyExists
	ErrMessageNotFound             = domain.ErrMessageNotFound
	ErrInvalidWebhookEvent         = domain.ErrInvalidWebhookEvent
	ErrInvalidWebhookURL           = domain.ErrInvalidWebhookURL
	ErrInvalidJSON                 = domain.ErrInvalidJSON
	ErrInvalidEnum                 = domain.ErrInvalidEnum
	ErrInvalidInput                = domain.ErrInvalidInput
	ErrInvalidCursor               = domain.ErrInvalidCursor
	ErrWebhookNotFound             = domain.ErrWebhookNotFound
)

const (
	// DefaultInstancePageLimit is the page size used when a caller omits one.
	DefaultInstancePageLimit = domain.DefaultInstancePageLimit
	// MaxInstancePageLimit caps every public instance query.
	MaxInstancePageLimit = domain.MaxInstancePageLimit
)

// Base contains the shared application database. Authorization is deliberately
// not stored here: every persistence call must present its own Grant.
type Base struct{ db *data.DB }

// NewBase returns the shared state for the repository adapters.
func NewBase(db *data.DB) *Base { return &Base{db: db} }

// tenantFromGrant may only be called after the repository method has checked
// its exact action. Keeping extraction separate makes that ordering visible at
// every SQL door.
func tenantFromGrant(grant security.Grant) (string, error) {
	tenant := data.Tenant(grant)
	if tenant == "" {
		return "", fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	return tenant, nil
}

func (b *Base) database() *data.DB { return b.db }

func newID() (int64, error) {
	return newIDFrom(cryptorand.Reader)
}

func newIDFrom(reader io.Reader) (int64, error) {
	var buffer [8]byte
	for {
		if _, err := io.ReadFull(reader, buffer[:]); err != nil {
			return 0, fmt.Errorf("generate database identifier: %w", err)
		}
		id := int64(binary.BigEndian.Uint64(buffer[:]) & uint64(1<<63-1))
		if id != 0 {
			return id, nil
		}
	}
}

func looksUnique(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

type InstanceRepository interface {
	Create(ctx context.Context, grant security.Grant, input types.CreateInstanceInput) (types.InstanceRecord, error)
	FindByID(ctx context.Context, grant security.Grant, id int64) (types.InstanceRecord, error)
	FindByName(ctx context.Context, grant security.Grant, name string) (types.InstanceRecord, error)
	ListPage(ctx context.Context, grant security.Grant, query data.Query, name *string) (database.Page[types.InstanceRecord], error)
	FetchDetailsByName(ctx context.Context, grant security.Grant, name string) (types.InstanceDetails, error)
	FindAutoConnectInstances(ctx context.Context, grant security.Grant) ([]types.Instance, error)
	Update(ctx context.Context, grant security.Grant, instanceID int64, input types.UpdateInstanceInput) (types.InstanceRecord, error)
	UpdateStatus(ctx context.Context, grant security.Grant, instanceID int64, status types.InstanceStatus) error
	UpdateConnectionState(ctx context.Context, grant security.Grant, input types.UpdateConnectionStateInput) error
	SaveWhatsAppDevice(ctx context.Context, grant security.Grant, input types.SaveWhatsAppDeviceInput) error
	ClearWhatsAppDevice(ctx context.Context, grant security.Grant, instanceID int64) error
	UpdateProfilePicture(ctx context.Context, grant security.Grant, instanceID int64, profilePicURL *string, profilePicID *string) error
	TryAcquireConnectionLock(ctx context.Context, grant security.Grant, instanceID string) (bool, error)
	ReleaseConnectionLock(ctx context.Context, grant security.Grant, instanceID string) error
	EnsureDeletable(ctx context.Context, grant security.Grant, instanceID int64) error
	Delete(ctx context.Context, grant security.Grant, instanceID int64, force bool) error
}

type MessageRepository interface {
	Create(ctx context.Context, grant security.Grant, input types.CreateMessageInput) (types.Message, error)
	CreateOrIgnore(ctx context.Context, grant security.Grant, input types.CreateMessageInput) error
	FindByIDForInstance(ctx context.Context, grant security.Grant, instanceID int64, id int64) (types.Message, error)
	FindByKeyIDForInstance(ctx context.Context, grant security.Grant, instanceID int64, keyID string) (types.Message, error)
	FindByIDsForInstance(ctx context.Context, grant security.Grant, instanceID int64, ids []int64) ([]types.Message, error)
	FindOutgoingByIDForInstance(ctx context.Context, grant security.Grant, instanceID int64, id int64) (types.Message, error)
	FindOutgoingByKeyIDForInstance(ctx context.Context, grant security.Grant, instanceID int64, keyID string) (types.Message, error)
	MarkReadForInstance(ctx context.Context, grant security.Grant, instanceID int64, ids []int64) error
	UpdateContentForInstance(ctx context.Context, grant security.Grant, instanceID int64, id int64, content json.RawMessage) (types.Message, error)
	Count(ctx context.Context, grant security.Grant, instanceID int64, filters types.MessageFilters) (int64, error)
	List(ctx context.Context, grant security.Grant, instanceID int64, input types.ListMessagesInput) (types.MessageListResult, error)
	ListPage(ctx context.Context, grant security.Grant, instanceID int64, input types.ListMessagesPageInput) (types.MessageListResult, error)
}

type MessageUpdateRepository interface {
	Create(ctx context.Context, grant security.Grant, input types.CreateMessageUpdateInput) (types.MessageUpdate, error)
	CreateOrIgnore(ctx context.Context, grant security.Grant, input types.CreateMessageUpdateInput) error
	ListByMessageID(ctx context.Context, grant security.Grant, messageID int64) ([]types.MessageUpdate, error)
}

type ContactRepository interface {
	Create(ctx context.Context, grant security.Grant, input types.CreateContactInput) (types.Contact, error)
	Upsert(ctx context.Context, grant security.Grant, input types.CreateContactInput) (types.Contact, error)
	List(ctx context.Context, grant security.Grant, instanceID int64, filters types.ContactFilters) ([]types.Contact, error)
}

type WebhookRepository interface {
	Create(ctx context.Context, grant security.Grant, input types.CreateWebhookInput) (types.Webhook, error)
	FindByInstanceName(ctx context.Context, grant security.Grant, instanceName string) (types.Webhook, error)
	ListEnabledWithInstance(ctx context.Context, grant security.Grant) ([]types.WebhookWithInstance, error)
	Update(ctx context.Context, grant security.Grant, webhookID int64, input types.UpdateWebhookInput) (types.Webhook, error)
	UpsertEvents(ctx context.Context, grant security.Grant, webhookID int64, events map[string]bool) (types.Webhook, error)
}

// InstanceDependenciesError reports the owned record counts that blocked deletion.
type InstanceDependenciesError struct {
	InstanceID int64
	Messages   int64
	Chats      int64
	Contacts   int64
	Webhooks   int64
}

// Error describes the instance and its owned records.
func (e *InstanceDependenciesError) Error() string {
	return fmt.Sprintf("%v: instance %d has %d messages, %d chats, %d contacts and %d webhooks",
		ErrInstanceHasDependencies, e.InstanceID, e.Messages, e.Chats, e.Contacts, e.Webhooks)
}

// Unwrap returns the shared domain sentinel.
func (e *InstanceDependenciesError) Unwrap() error { return domain.ErrInstanceHasDependencies }

// As preserves compatibility with the domain model used across application layers.
func (e *InstanceDependenciesError) As(target any) bool {
	destination, ok := target.(**domain.InstanceDependenciesError)
	if !ok {
		return false
	}
	*destination = &domain.InstanceDependenciesError{
		InstanceID: e.InstanceID,
		Messages:   e.Messages,
		Chats:      e.Chats,
		Contacts:   e.Contacts,
		Webhooks:   e.Webhooks,
	}
	return true
}
