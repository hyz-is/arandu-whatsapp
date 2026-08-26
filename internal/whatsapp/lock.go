package whatsapp

import (
	"context"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
)

type InstanceConnectionLock interface {
	TryAcquire(ctx context.Context, grant security.Grant, instanceID string) (bool, error)
	Release(ctx context.Context, grant security.Grant, instanceID string) error
}

type PostgresInstanceConnectionLock struct {
	instances repository.InstanceRepository
}

func NewPostgresInstanceConnectionLock(instances repository.InstanceRepository) PostgresInstanceConnectionLock {
	return PostgresInstanceConnectionLock{instances: instances}
}

func (l PostgresInstanceConnectionLock) TryAcquire(ctx context.Context, grant security.Grant, instanceID string) (bool, error) {
	return l.instances.TryAcquireConnectionLock(ctx, grant, instanceID)
}

func (l PostgresInstanceConnectionLock) Release(ctx context.Context, grant security.Grant, instanceID string) error {
	return l.instances.ReleaseConnectionLock(ctx, grant, instanceID)
}
