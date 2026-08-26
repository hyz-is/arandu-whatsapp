package whatsapp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// InstanceRepository is the public, Grant-first door to instance persistence.
// Internal SQL adapters receive the same explicit Grant and repeat the action
// check before deriving the tenant or reaching the database.
type InstanceRepository struct {
	inner internalrepo.InstanceRepository
}

// NewInstanceRepository returns an instance repository over the application database.
func NewInstanceRepository(db *data.DB) *InstanceRepository {
	base := internalrepo.NewBase(db)
	return newInstanceRepository(internalrepo.NewInstanceRepository(base))
}

func newInstanceRepository(inner internalrepo.InstanceRepository) *InstanceRepository {
	return &InstanceRepository{inner: inner}
}

var _ data.Repository[Instance, int64] = (*InstanceRepository)(nil)

const (
	// DefaultInstancePageLimit is used when an instance query omits its limit.
	DefaultInstancePageLimit = internalrepo.DefaultInstancePageLimit
	// MaxInstancePageLimit is the largest accepted instance page.
	MaxInstancePageLimit = internalrepo.MaxInstancePageLimit
)

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

// Find returns one instance in the Grant tenant.
func (r *InstanceRepository) Find(ctx context.Context, grant security.Grant, id int64) (Instance, error) {
	if err := grant.Check(ActionInstanceView); err != nil {
		return Instance{}, err
	}
	item, err := r.inner.FindByID(ctx, grant, id)
	if err != nil {
		return Instance{}, err
	}
	return instanceFromInternal(item.Instance), nil
}

// FindByName returns one instance by its stable public name.
func (r *InstanceRepository) FindByName(ctx context.Context, grant security.Grant, name string) (Instance, error) {
	if err := grant.Check(ActionInstanceView); err != nil {
		return Instance{}, err
	}
	item, err := r.inner.FindByName(ctx, grant, name)
	if err != nil {
		return Instance{}, err
	}
	return instanceFromInternal(item.Instance), nil
}

// List returns one bounded tenant-scoped page for the legacy framework repository contract.
func (r *InstanceRepository) List(ctx context.Context, grant security.Grant, query data.Query) ([]Instance, error) {
	page, err := r.listPage(ctx, grant, InstanceListQuery{Limit: query.Limit, Cursor: query.Cursor}, query.Sort)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListPage returns a bounded tenant-scoped page and its next cursor.
func (r *InstanceRepository) ListPage(ctx context.Context, grant security.Grant, query InstanceListQuery) (InstancePage, error) {
	return r.listPage(ctx, grant, query, "")
}

func (r *InstanceRepository) listPage(ctx context.Context, grant security.Grant, query InstanceListQuery, sort string) (InstancePage, error) {
	if err := grant.Check(ActionInstanceList); err != nil {
		return InstancePage{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = DefaultInstancePageLimit
	}
	if limit < 1 || limit > MaxInstancePageLimit {
		return InstancePage{}, ErrInvalidInput
	}
	var name *string
	if query.Name != nil {
		value := strings.TrimSpace(*query.Name)
		if value == "" || len(value) > 255 {
			return InstancePage{}, ErrInvalidInput
		}
		name = &value
	}
	page, err := r.inner.ListPage(ctx, grant, data.Query{Limit: limit, Cursor: query.Cursor, Sort: sort}, name)
	if err != nil {
		return InstancePage{}, err
	}
	out := make([]Instance, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, instanceFromInternal(item.Instance))
	}
	return InstancePage{Items: out, NextCursor: page.Next, PerPage: limit}, nil
}

// Create stores a new instance in the Grant tenant.
func (r *InstanceRepository) Create(ctx context.Context, grant security.Grant, entity Instance) (Instance, error) {
	if err := grant.Check(ActionInstanceCreate); err != nil {
		return Instance{}, err
	}
	status := dbtypes.InstanceStatus(entity.Status)
	var statusPointer *dbtypes.InstanceStatus
	if entity.Status != "" {
		statusPointer = &status
	}
	item, err := r.inner.Create(ctx, grant, dbtypes.CreateInstanceInput{
		Name: entity.Name, Description: entity.Description, Status: statusPointer,
		OwnerJid: entity.OwnerJID, ProfilePicUrl: entity.ProfilePictureURL,
		ExternalAttributes: entity.ExternalAttributes,
	})
	if err != nil {
		return Instance{}, err
	}
	return instanceFromInternal(item.Instance), nil
}

// Update changes the mutable instance metadata.
func (r *InstanceRepository) Update(ctx context.Context, grant security.Grant, entity Instance) (Instance, error) {
	if err := grant.Check(ActionInstanceUpdate); err != nil {
		return Instance{}, err
	}
	name := entity.Name
	description := dbtypes.OptionalField[string]{Set: true, Value: entity.Description}
	profile := dbtypes.OptionalField[string]{Set: true, Value: entity.ProfilePictureURL}
	attributes := dbtypes.OptionalField[json.RawMessage]{Set: true, Value: &entity.ExternalAttributes}
	item, err := r.inner.Update(ctx, grant, entity.ID, dbtypes.UpdateInstanceInput{
		Name: &name, Description: description, ProfilePicUrl: profile, ExternalAttributes: attributes,
	})
	if err != nil {
		return Instance{}, err
	}
	return instanceFromInternal(item.Instance), nil
}

// Delete removes one instance and its owned rows.
func (r *InstanceRepository) Delete(ctx context.Context, grant security.Grant, id int64) error {
	if err := grant.Check(ActionInstanceDelete); err != nil {
		return err
	}
	return r.inner.Delete(ctx, grant, id, false)
}
