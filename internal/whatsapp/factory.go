package whatsapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/lib/pq"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	watypes "go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
)

type WhatsAppClientFactory interface {
	NewDeviceClient(ctx context.Context, grant security.Grant) (*whatsmeow.Client, error)
	ClientForDevice(ctx context.Context, grant security.Grant, deviceJID string) (*whatsmeow.Client, error)
	Store() *sqlstore.Container
}

type SQLStoreClientFactory struct {
	container *sqlstore.Container
	logger    waLog.Logger
}

func NewSQLStoreClientFactory(db *data.DB, logger waLog.Logger) (*SQLStoreClientFactory, error) {
	container, err := NewWhatsAppSessionContainer(db, logger)
	if err != nil {
		return nil, err
	}
	return &SQLStoreClientFactory{container: container, logger: logger}, nil
}

// NewWhatsAppSessionContainer wraps the database already owned by the Arandu
// application. It deliberately does not call Upgrade: schema changes are
// declared by Module.Migrations and run by `aru migrate`, never at boot.
//
// The returned container does not own the handle and must not be closed. The
// application closes the shared database after modules have stopped.
func NewWhatsAppSessionContainer(db *data.DB, logger waLog.Logger) (*sqlstore.Container, error) {
	if db == nil {
		return nil, errors.New("initialize whatsmeow sqlstore: database handle is required")
	}

	var dialect string
	switch db.Dialect() {
	case data.DialectPostgres:
		dialect = "postgres"
		sqlstore.PostgresArrayWrapper = pq.Array
	case data.DialectSQLite:
		dialect = "sqlite3"
	default:
		return nil, fmt.Errorf("initialize whatsmeow sqlstore: %s is not supported; use PostgreSQL or SQLite", db.Dialect())
	}

	return sqlstore.NewWithDB(db.Unwrap(), dialect, logger), nil
}

func (f *SQLStoreClientFactory) NewDeviceClient(ctx context.Context, grant security.Grant) (*whatsmeow.Client, error) {
	if err := checkDeviceStoreGrant(grant); err != nil {
		return nil, err
	}
	_ = ctx
	return f.clientForDevice(f.container.NewDevice()), nil
}

func (f *SQLStoreClientFactory) ClientForDevice(ctx context.Context, grant security.Grant, deviceJID string) (*whatsmeow.Client, error) {
	if err := checkDeviceStoreGrant(grant); err != nil {
		return nil, err
	}
	jid, err := watypes.ParseJID(deviceJID)
	if err != nil {
		return nil, fmt.Errorf("%w: parse device jid: %w", ErrSessionMissing, err)
	}
	device, err := f.container.GetDevice(ctx, jid)
	if err != nil {
		return nil, fmt.Errorf("get whatsmeow device: %w", err)
	}
	if device == nil || device.ID == nil {
		return nil, ErrSessionMissing
	}
	return f.clientForDevice(device), nil
}

func checkDeviceStoreGrant(grant security.Grant) error {
	if err := authz.CheckDeviceStore(grant); err != nil {
		return err
	}
	if data.Tenant(grant) == "" {
		return fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	return nil
}

func (f *SQLStoreClientFactory) Store() *sqlstore.Container {
	return f.container
}

func (f *SQLStoreClientFactory) clientForDevice(device *store.Device) *whatsmeow.Client {
	return whatsmeow.NewClient(device, f.logger.Sub("client"))
}
