// Package repositories contains the Grant-first persistence adapters.
package repositories

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	enums "github.com/hyz-is/arandu-whatsapp/app/Enums"
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// InstanceRepository is the Grant-first door to instance persistence.
type InstanceRepository struct {
	inner internalrepo.InstanceRepository
}

// NewInstanceRepository returns an instance repository over the application database.
func NewInstanceRepository(db *data.DB) *InstanceRepository {
	return NewInstanceRepositoryFromInternal(internalrepo.NewInstanceRepository(internalrepo.NewBase(db)))
}

// NewInstanceRepositoryFromInternal adapts the internal persistence contract.
func NewInstanceRepositoryFromInternal(inner internalrepo.InstanceRepository) *InstanceRepository {
	return &InstanceRepository{inner: inner}
}

var _ data.Repository[models.Instance, int64] = (*InstanceRepository)(nil)

const (
	// DefaultInstancePageLimit is used when an instance query omits its limit.
	DefaultInstancePageLimit = models.DefaultInstancePageLimit
	// MaxInstancePageLimit is the largest accepted instance page.
	MaxInstancePageLimit = models.MaxInstancePageLimit
)

// Find returns one instance in the Grant tenant.
func (r *InstanceRepository) Find(ctx context.Context, grant security.Grant, id int64) (models.Instance, error) {
	if err := grant.Check(enums.ActionInstanceView); err != nil {
		return models.Instance{}, err
	}
	item, err := r.inner.FindByID(ctx, grant, id)
	if err != nil {
		return models.Instance{}, err
	}
	return instanceFromDatabase(item.Instance), nil
}

// FindByName returns one instance by its stable public name.
func (r *InstanceRepository) FindByName(ctx context.Context, grant security.Grant, name string) (models.Instance, error) {
	if err := grant.Check(enums.ActionInstanceView); err != nil {
		return models.Instance{}, err
	}
	item, err := r.inner.FindByName(ctx, grant, name)
	if err != nil {
		return models.Instance{}, err
	}
	return instanceFromDatabase(item.Instance), nil
}

// ResolveByName returns one instance after the supplied action Grant passes the
// repository's instance-lookup contract.
func (r *InstanceRepository) ResolveByName(ctx context.Context, grant security.Grant, name string) (models.Instance, error) {
	item, err := r.inner.FindByName(ctx, grant, name)
	if err != nil {
		return models.Instance{}, err
	}
	return instanceFromDatabase(item.Instance), nil
}

// List returns one bounded tenant-scoped page for the framework repository contract.
func (r *InstanceRepository) List(ctx context.Context, grant security.Grant, query data.Query) ([]models.Instance, error) {
	page, err := r.listPage(ctx, grant, models.InstanceListQuery{Limit: query.Limit, Cursor: query.Cursor}, query.Sort)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListPage returns a bounded tenant-scoped page and its next cursor.
func (r *InstanceRepository) ListPage(ctx context.Context, grant security.Grant, query models.InstanceListQuery) (models.InstancePage, error) {
	return r.listPage(ctx, grant, query, "")
}

func (r *InstanceRepository) listPage(ctx context.Context, grant security.Grant, query models.InstanceListQuery, sort string) (models.InstancePage, error) {
	if err := grant.Check(enums.ActionInstanceList); err != nil {
		return models.InstancePage{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = DefaultInstancePageLimit
	}
	if limit < 1 || limit > MaxInstancePageLimit {
		return models.InstancePage{}, internalrepo.ErrInvalidInput
	}
	var name *string
	if query.Name != nil {
		value := strings.TrimSpace(*query.Name)
		if value == "" || len(value) > 255 {
			return models.InstancePage{}, internalrepo.ErrInvalidInput
		}
		name = &value
	}
	page, err := r.inner.ListPage(ctx, grant, data.Query{Limit: limit, Cursor: query.Cursor, Sort: sort}, name)
	if err != nil {
		return models.InstancePage{}, err
	}
	out := make([]models.Instance, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, instanceFromDatabase(item.Instance))
	}
	return models.InstancePage{Items: out, NextCursor: page.Next, PerPage: limit}, nil
}

// Create stores a new instance in the Grant tenant.
func (r *InstanceRepository) Create(ctx context.Context, grant security.Grant, entity models.Instance) (models.Instance, error) {
	if err := grant.Check(enums.ActionInstanceCreate); err != nil {
		return models.Instance{}, err
	}
	status := dbtypes.InstanceStatus(entity.Status)
	var statusPointer *dbtypes.InstanceStatus
	if entity.Status != "" {
		statusPointer = &status
	}
	item, err := r.inner.Create(ctx, grant, dbtypes.CreateInstanceInput{Name: entity.Name, Description: entity.Description,
		Status: statusPointer, OwnerJid: entity.OwnerJID, ProfilePicUrl: entity.ProfilePictureURL,
		ExternalAttributes: entity.ExternalAttributes})
	if err != nil {
		return models.Instance{}, err
	}
	return instanceFromDatabase(item.Instance), nil
}

// Update changes the mutable instance metadata.
func (r *InstanceRepository) Update(ctx context.Context, grant security.Grant, entity models.Instance) (models.Instance, error) {
	if err := grant.Check(enums.ActionInstanceUpdate); err != nil {
		return models.Instance{}, err
	}
	name := entity.Name
	item, err := r.inner.Update(ctx, grant, entity.ID, dbtypes.UpdateInstanceInput{Name: &name,
		Description:        dbtypes.OptionalField[string]{Set: true, Value: entity.Description},
		ProfilePicUrl:      dbtypes.OptionalField[string]{Set: true, Value: entity.ProfilePictureURL},
		ExternalAttributes: dbtypes.OptionalField[json.RawMessage]{Set: true, Value: &entity.ExternalAttributes}})
	if err != nil {
		return models.Instance{}, err
	}
	return instanceFromDatabase(item.Instance), nil
}

// Delete removes one instance and its owned rows.
func (r *InstanceRepository) Delete(ctx context.Context, grant security.Grant, id int64) error {
	if err := grant.Check(enums.ActionInstanceDelete); err != nil {
		return err
	}
	return r.inner.Delete(ctx, grant, id, false)
}

func instanceFromDatabase(item dbtypes.Instance) models.Instance {
	return models.Instance{
		ID: item.ID, TenantID: item.TenantID, Name: item.Name,
		Description: item.Description, Status: string(item.Status),
		ConnectionStatus: string(item.ConnectionStatus), OwnerJID: item.OwnerJid,
		ProfilePictureURL: item.ProfilePicUrl, WhatsAppDeviceJID: item.WhatsAppDeviceJid,
		WhatsAppOwnerJID: item.WhatsAppOwnerJid, WhatsAppPhone: item.WhatsAppPhone,
		LastConnectedAt: item.LastConnectedAt, LastDisconnectedAt: item.LastDisconnectedAt,
		ConnectionAttempts: item.ConnectionAttempts, CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt, ExternalAttributes: item.ExternalAttributes,
	}
}
