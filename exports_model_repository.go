package whatsapp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	requests "github.com/hyz-is/arandu-whatsapp/app/Http/Requests"
	resources "github.com/hyz-is/arandu-whatsapp/app/Http/Resources"
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	repositories "github.com/hyz-is/arandu-whatsapp/app/Repositories"
	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
)

// Instance is the public WhatsApp instance entity protected by InstancePolicy.
type Instance struct {
	// ID is the instance identifier.
	ID int64
	// TenantID is the tenant that owns the instance.
	TenantID string
	// Name is the stable public instance name.
	Name string
	// Description is optional human-readable metadata.
	Description *string
	// Status is the configured instance status.
	Status string
	// ConnectionStatus is the latest persisted connection state.
	ConnectionStatus string
	// OwnerJID is the legacy owner address when one is stored.
	OwnerJID *string
	// ProfilePictureURL is the latest known profile-picture URL.
	ProfilePictureURL *string
	// WhatsAppDeviceJID is the paired device address.
	WhatsAppDeviceJID *string
	// WhatsAppOwnerJID is the paired account address without device information.
	WhatsAppOwnerJID *string
	// WhatsAppPhone is the paired account phone number.
	WhatsAppPhone *string
	// LastConnectedAt is when the instance most recently connected.
	LastConnectedAt *time.Time
	// LastDisconnectedAt is when the instance most recently disconnected.
	LastDisconnectedAt *time.Time
	// ConnectionAttempts is the number of consecutive connection attempts.
	ConnectionAttempts int32
	// CreatedAt is when the instance was created.
	CreatedAt time.Time
	// UpdatedAt is when the instance was last updated.
	UpdatedAt time.Time
	// ExternalAttributes holds caller-defined JSON object metadata.
	ExternalAttributes json.RawMessage
}

// CreateInstanceInput is the explicit instance creation contract.
type CreateInstanceInput struct {
	// Name is the stable public name. A generated name is used when it is absent.
	Name *string `json:"name,omitempty"`
	// Description is optional human-readable instance metadata.
	Description *string `json:"description,omitempty"`
	// ExternalAttributes holds caller-defined JSON object metadata.
	ExternalAttributes json.RawMessage `json:"externalAttributes,omitempty"`
}

// PhonePairingInput is the JSON-safe input for phone-code pairing.
type PhonePairingInput struct {
	// PhoneNumber is the international phone number to pair.
	PhoneNumber string `json:"phoneNumber"`
}

// InstanceListQuery selects one forward-only page of tenant instances.
type InstanceListQuery struct {
	// Name limits results to instance names containing this value.
	Name *string
	// Limit is the page size. Zero uses DefaultInstancePageLimit.
	Limit int
	// Cursor is the opaque NextCursor returned by the preceding page.
	Cursor string
}

// InstancePage contains tenant instances and the cursor for the next page.
type InstancePage struct {
	// Items are the instances in this page.
	Items []Instance
	// NextCursor is empty when this is the final page.
	NextCursor string
	// PerPage is the normalized page size.
	PerPage int
}

// InstanceResource declares the fields that may leave the module. Tenant and
// the WhatsMeow device-store key are intentionally absent.
type InstanceResource struct{ instance Instance }

// NewInstanceResource wraps one instance for an Arandu JSON response.
func NewInstanceResource(instance Instance) InstanceResource {
	return InstanceResource{instance: instance}
}

// ToArray returns the public representation of an instance.
func (r InstanceResource) ToArray() map[string]any {
	return resources.NewInstanceResource(instanceToModel(r.instance)).ToArray()
}

// With returns no top-level metadata.
func (r InstanceResource) With() map[string]any {
	return resources.NewInstanceResource(instanceToModel(r.instance)).With()
}

// InstanceCollection is a tenant-scoped instance page.
type InstanceCollection struct{ page InstancePage }

// NewInstanceCollection wraps an instance page for an Arandu JSON response.
func NewInstanceCollection(page InstancePage) InstanceCollection {
	return InstanceCollection{page: page}
}

// ToArray returns the page items and forward-pagination metadata.
func (c InstanceCollection) ToArray() map[string]any {
	return resources.NewInstanceCollection(instancePageToModel(c.page)).ToArray()
}

// With returns no top-level metadata.
func (c InstanceCollection) With() map[string]any {
	return resources.NewInstanceCollection(instancePageToModel(c.page)).With()
}

// InstanceRepository is the public, Grant-first door to instance persistence.
// The native application repository receives the same explicit Grant and
// derives the tenant before reaching the database.
type InstanceRepository struct {
	inner *repositories.InstanceRepository
}

// NewInstanceRepository returns an instance repository over the application database.
func NewInstanceRepository(db *data.DB) *InstanceRepository {
	return newInstanceRepository(internalrepo.NewInstanceRepository(internalrepo.NewBase(db)))
}

func newInstanceRepository(inner internalrepo.InstanceRepository) *InstanceRepository {
	return &InstanceRepository{inner: repositories.NewInstanceRepositoryFromInternal(inner)}
}

var _ data.Repository[Instance, int64] = (*InstanceRepository)(nil)

const (
	// DefaultInstancePageLimit is used when an instance query omits its limit.
	DefaultInstancePageLimit = repositories.DefaultInstancePageLimit
	// MaxInstancePageLimit is the largest accepted instance page.
	MaxInstancePageLimit = repositories.MaxInstancePageLimit
)

// Find returns one instance in the Grant tenant.
func (r *InstanceRepository) Find(ctx context.Context, grant security.Grant, id int64) (Instance, error) {
	instance, err := r.inner.Find(ctx, grant, id)
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(instance), nil
}

// FindByName returns one instance by its stable public name.
func (r *InstanceRepository) FindByName(ctx context.Context, grant security.Grant, name string) (Instance, error) {
	instance, err := r.inner.FindByName(ctx, grant, name)
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(instance), nil
}

// List returns one bounded tenant-scoped page for the legacy framework repository contract.
func (r *InstanceRepository) List(ctx context.Context, grant security.Grant, query data.Query) ([]Instance, error) {
	items, err := r.inner.List(ctx, grant, query)
	if err != nil {
		return nil, err
	}
	return instancesFromModels(items), nil
}

// ListPage returns a bounded tenant-scoped page and its next cursor.
func (r *InstanceRepository) ListPage(ctx context.Context, grant security.Grant, query InstanceListQuery) (InstancePage, error) {
	page, err := r.inner.ListPage(ctx, grant, instanceListQueryToModel(query))
	if err != nil {
		return InstancePage{}, err
	}
	return instancePageFromModel(page), nil
}

// Create stores a new instance in the Grant tenant.
func (r *InstanceRepository) Create(ctx context.Context, grant security.Grant, entity Instance) (Instance, error) {
	instance, err := r.inner.Create(ctx, grant, instanceToModel(entity))
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(instance), nil
}

// Update changes the mutable instance metadata.
func (r *InstanceRepository) Update(ctx context.Context, grant security.Grant, entity Instance) (Instance, error) {
	instance, err := r.inner.Update(ctx, grant, instanceToModel(entity))
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(instance), nil
}

// Delete removes one instance and its owned rows.
func (r *InstanceRepository) Delete(ctx context.Context, grant security.Grant, id int64) error {
	return r.inner.Delete(ctx, grant, id)
}

func instanceFromModel(instance models.Instance) Instance {
	return Instance{
		ID: instance.ID, TenantID: instance.TenantID, Name: instance.Name,
		Description: instance.Description, Status: instance.Status,
		ConnectionStatus: instance.ConnectionStatus, OwnerJID: instance.OwnerJID,
		ProfilePictureURL: instance.ProfilePictureURL, WhatsAppDeviceJID: instance.WhatsAppDeviceJID,
		WhatsAppOwnerJID: instance.WhatsAppOwnerJID, WhatsAppPhone: instance.WhatsAppPhone,
		LastConnectedAt: instance.LastConnectedAt, LastDisconnectedAt: instance.LastDisconnectedAt,
		ConnectionAttempts: instance.ConnectionAttempts, CreatedAt: instance.CreatedAt,
		UpdatedAt: instance.UpdatedAt, ExternalAttributes: instance.ExternalAttributes,
	}
}

func instanceToModel(instance Instance) models.Instance {
	return models.Instance{
		ID: instance.ID, TenantID: instance.TenantID, Name: instance.Name,
		Description: instance.Description, Status: instance.Status,
		ConnectionStatus: instance.ConnectionStatus, OwnerJID: instance.OwnerJID,
		ProfilePictureURL: instance.ProfilePictureURL, WhatsAppDeviceJID: instance.WhatsAppDeviceJID,
		WhatsAppOwnerJID: instance.WhatsAppOwnerJID, WhatsAppPhone: instance.WhatsAppPhone,
		LastConnectedAt: instance.LastConnectedAt, LastDisconnectedAt: instance.LastDisconnectedAt,
		ConnectionAttempts: instance.ConnectionAttempts, CreatedAt: instance.CreatedAt,
		UpdatedAt: instance.UpdatedAt, ExternalAttributes: instance.ExternalAttributes,
	}
}

func instancesFromModels(items []models.Instance) []Instance {
	if items == nil {
		return nil
	}
	instances := make([]Instance, len(items))
	for index, item := range items {
		instances[index] = instanceFromModel(item)
	}
	return instances
}

func instancesToModels(items []Instance) []models.Instance {
	if items == nil {
		return nil
	}
	instances := make([]models.Instance, len(items))
	for index, item := range items {
		instances[index] = instanceToModel(item)
	}
	return instances
}

func instanceListQueryToModel(query InstanceListQuery) models.InstanceListQuery {
	return models.InstanceListQuery{Name: query.Name, Limit: query.Limit, Cursor: query.Cursor}
}

func instancePageFromModel(page models.InstancePage) InstancePage {
	return InstancePage{
		Items: instancesFromModels(page.Items), NextCursor: page.NextCursor, PerPage: page.PerPage,
	}
}

func instancePageToModel(page InstancePage) models.InstancePage {
	return models.InstancePage{
		Items: instancesToModels(page.Items), NextCursor: page.NextCursor, PerPage: page.PerPage,
	}
}

func createInstanceInputToRequest(input CreateInstanceInput) requests.CreateInstance {
	return requests.CreateInstance{
		Name: input.Name, Description: input.Description, ExternalAttributes: input.ExternalAttributes,
	}
}

func phonePairingInputToRequest(input PhonePairingInput) requests.PairPhone {
	return requests.PairPhone{PhoneNumber: input.PhoneNumber}
}
