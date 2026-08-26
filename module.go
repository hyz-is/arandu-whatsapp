// Package whatsapp provides a tenant-scoped WhatsApp API module for Arandu.
package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"sync"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/foundation"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/queue"

	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

// Module wires the WhatsApp domain into the Arandu lifecycle. The database and
// session store are borrowed from the host application and are never closed by
// this module.
type Module struct {
	cfg      Config
	db       *data.DB
	sessions *security.SessionStore
	logger   *slog.Logger

	service           *Service
	webhookManager    *webhooksvc.Manager
	messageProcessor  *message.MessageProcessingManager
	connections       *internalwhatsapp.Service
	whatsMeowUpgrader whatsMeowSchemaUpgrader

	mu          sync.RWMutex
	lifecycle   context.Context
	cancel      context.CancelFunc
	restoreDone chan struct{}
	restoreErr  error
	started     bool
	closed      bool
}

var (
	_ foundation.Module     = (*Module)(nil)
	_ foundation.Bootable   = (*Module)(nil)
	_ foundation.Background = (*Module)(nil)
	_ foundation.Health     = (*Module)(nil)
	_ foundation.Closable   = (*Module)(nil)
	_ foundation.Migratable = (*Module)(nil)
)

// New validates configuration and assembles all collaborators without doing
// database I/O, starting goroutines, connecting to WhatsApp or running schema
// changes. That keeps commands such as `aru migrate` and `aru route:list`
// side-effect free.
func New(cfg Config, db *data.DB, sessions *security.SessionStore) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("whatsapp: New needs a database handle")
	}
	if sessions == nil {
		return nil, errors.New("whatsapp: New needs a session store")
	}
	if db.Dialect() != data.DialectPostgres && db.Dialect() != data.DialectSQLite {
		return nil, fmt.Errorf("whatsapp: database dialect %q is unsupported; use PostgreSQL or SQLite", db.Dialect())
	}

	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logger := slog.Default()
	base := internalrepo.NewBase(db)
	instances := internalrepo.NewInstanceRepository(base)
	messages := internalrepo.NewMessageRepository(base)
	messageUpdates := internalrepo.NewMessageUpdateRepository(base)
	contacts := internalrepo.NewContactRepository(base)
	webhookRepository := internalrepo.NewWebhookRepository(base)
	addressRepository := internalrepo.NewAddressMappingRepository(base)

	webhookManager, err := webhooksvc.NewManager(db, webhooksvc.ManagerConfig{
		GlobalURL: cfg.Webhooks.GlobalURL, GlobalEnabled: cfg.Webhooks.GlobalEnabled,
		SigningSecret: cfg.Webhooks.SigningSecret, Retention: cfg.Webhooks.Retention,
	})
	if err != nil {
		return nil, fmt.Errorf("whatsapp: build webhook manager: %w", err)
	}

	waLogger := internalwhatsapp.NewWhatsmeowLogger(logger)
	clientFactory, err := internalwhatsapp.NewSQLStoreClientFactory(db, waLogger)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: build WhatsMeow store: %w", err)
	}
	hub := internalwhatsapp.NewClientHub()
	lock := internalwhatsapp.NewPostgresInstanceConnectionLock(instances)
	events := internalwhatsapp.NewEventPersistenceService(internalwhatsapp.EventPersistenceConfig{
		SaveDataNewMessage:    cfg.Persistence.Messages,
		SaveMessageUpdate:     cfg.Persistence.MessageUpdates,
		SaveDataContacts:      cfg.Persistence.Contacts,
		ProfilePictureTimeout: cfg.WhatsApp.ProfilePictureTimeout,
	}, messages, messageUpdates, contacts)
	events.SetWebhookDispatcher(instances, webhookManager)
	connections, err := internalwhatsapp.NewService(
		cfg.internalWhatsApp(), instances, clientFactory, hub, lock, events, webhookManager, logger,
	)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: build connection service: %w", err)
	}

	resolver := address.NewResolver(addressRepository, cfg.WhatsApp.AddressCacheTTL)
	messageService := message.NewService(instances, messages, connections, resolver, webhookManager)
	messageService.ConfigureMedia(message.AudioConfig{
		MaxInputBytes: cfg.Media.MaxInputBytes, MaxDurationSeconds: cfg.Media.MaxDurationSeconds,
		ProcessingTimeout: cfg.Media.ProcessingTimeout, FFmpegPath: cfg.Media.FFmpegPath,
		FFprobePath: cfg.Media.FFprobePath, TempDir: cfg.Media.TempDir,
	}, message.ThumbnailConfig{
		MaxInputBytes: cfg.Media.MaxInputBytes, Timeout: cfg.Media.ProcessingTimeout,
		FFmpegPath: cfg.Media.FFmpegPath, TempDir: cfg.Media.TempDir,
	})
	processor, err := message.NewMessageProcessingManager(db, messageService, message.ProcessingConfig{
		ProcessingTimeout: cfg.Processing.ProcessingTimeout,
		GroupInfoTimeout:  cfg.Processing.GroupInfoTimeout, SendTimeout: cfg.Processing.SendTimeout,
		Retention: cfg.Processing.Retention,
	})
	if err != nil {
		return nil, fmt.Errorf("whatsapp: build message processor: %w", err)
	}
	messageService.SetProcessor(processor)
	chatService := chat.NewService(instances, messages, connections, resolver)
	groupService := group.NewService(instances, connections)
	webhookService := webhooksvc.NewService(instances, webhookRepository, cfg.Webhooks.SigningSecret != "")
	publicRepository := newInstanceRepository(instances)
	policy := NewInstancePolicy(cfg.Policy)

	return &Module{
		cfg: cfg, db: db, sessions: sessions, logger: logger,
		service: newService(cfg.Tenant, publicRepository, instances, policy, connections,
			messageService, chatService, groupService, webhookService),
		webhookManager:   webhookManager,
		messageProcessor: processor, connections: connections,
		whatsMeowUpgrader: clientFactory.Store(),
	}, nil
}

// Name is the stable module identifier used by Arandu route tooling.
func (*Module) Name() string { return "whatsapp" }

// Service exposes the authorized domain API for explicit composition by the
// host application.
func (m *Module) Service() *Service { return m.service }

// RegisterJobHandlers registers every durable module job with the host
// application's native queue worker.
func (m *Module) RegisterJobHandlers(worker *queue.Worker) error {
	if worker == nil {
		return errors.New("whatsapp: RegisterJobHandlers needs a worker")
	}
	for _, name := range []string{
		webhooksvc.WebhookDeliveryJobName,
		message.MessageProcessingJobName,
		message.MessageProcessingCleanupJobName,
	} {
		if _, exists := worker.Handler(name); exists {
			return fmt.Errorf("whatsapp: job handler %q is already registered", name)
		}
	}
	if err := m.webhookManager.RegisterJobHandlers(worker); err != nil {
		return err
	}
	return m.messageProcessor.RegisterJobHandlers(worker)
}

// Boot prepares the process-global WhatsMeow device descriptor. It performs no
// database or network work.
func (m *Module) Boot(context.Context) error {
	return internalwhatsapp.ConfigureSessionDevice(internalwhatsapp.SessionDeviceConfig{
		Client: m.cfg.WhatsApp.SessionPhoneClient,
		Name:   m.cfg.WhatsApp.SessionPhoneName,
	}, m.logger)
}

// Start verifies that migrations were applied. Connection restoration runs
// asynchronously and its terminal error is reported by Health. Durable jobs
// are owned by the host queue worker.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("whatsapp: module is closed")
	}
	if m.started {
		return nil
	}
	if m.db.Unwrap() == nil {
		return errors.New("whatsapp: Start needs an initialized database handle")
	}
	if err := m.db.PingContext(ctx); err != nil {
		return fmt.Errorf("whatsapp: database health check: %w", err)
	}
	if err := m.verifyWhatsMeowSchema(ctx); err != nil {
		return err
	}

	lifecycle, cancel := context.WithCancel(context.WithoutCancel(ctx))
	//arandu:system-grant module startup owns tenant-scoped runtime work
	runtimeGrant := security.SystemGrant(ActionRuntime, m.cfg.Tenant)

	m.lifecycle = lifecycle
	m.cancel = cancel
	m.restoreDone = make(chan struct{})
	m.started = true
	go m.restore(lifecycle, runtimeGrant, m.restoreDone)
	return nil
}

func (m *Module) verifyWhatsMeowSchema(ctx context.Context) error {
	if m.db.Dialect() == data.DialectSQLite {
		var enabled int
		if err := m.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
			return fmt.Errorf("whatsapp: verify SQLite foreign keys: %w", err)
		}
		if enabled != 1 {
			return errors.New("whatsapp: SQLite foreign keys are disabled; enable them before starting the module")
		}
	}
	var version int
	if err := m.db.QueryRowContext(ctx,
		`SELECT version FROM whatsmeow_version LIMIT 1`).Scan(&version); err != nil {
		return fmt.Errorf("whatsapp: WhatsMeow schema is unavailable; run `aru migrate`: %w", err)
	}
	if version < 1 {
		return errors.New("whatsapp: WhatsMeow schema has no applied version; run `aru migrate`")
	}
	return nil
}

func (m *Module) restore(ctx context.Context, grant security.Grant, done chan struct{}) {
	defer close(done)
	err := m.connections.Restore(ctx, grant)
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	m.mu.Lock()
	m.restoreErr = fmt.Errorf("restore WhatsApp connections: %w", err)
	m.mu.Unlock()
}

// Health checks only module-owned runtime state and the borrowed database. It
// does not call WhatsApp or probe every connected device.
func (m *Module) Health(ctx context.Context) error {
	if m.db == nil || m.db.Unwrap() == nil {
		return errors.New("whatsapp: database handle is unavailable")
	}
	if err := m.db.PingContext(ctx); err != nil {
		return fmt.Errorf("whatsapp: database health check: %w", err)
	}
	m.mu.RLock()
	err := m.restoreErr
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("whatsapp: module is closed")
	}
	return err
}

// Close stops producers before consumers while leaving the shared database,
// SessionStore and WhatsMeow container ownership with the host application.
func (m *Module) Close(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	cancel := m.cancel
	done := m.restoreDone
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	var errs []error
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
		}
	}
	if err := m.connections.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("stop WhatsApp connections: %w", err))
	}
	return errors.Join(errs...)
}

// Migrations declares the module-owned domain schema and delegates the
// WhatsMeow store schema to its upstream container. They are never applied by
// New, Boot or Start.
func (m *Module) Migrations() []foundation.Migration {
	return whatsappMigrations(m.whatsMeowUpgrader)
}

// subject obtains identity exclusively from the Arandu SessionStore. A failed
// lookup becomes a guest in the module's configured tenant.
func (m *Module) subject(r *stdhttp.Request) security.Subject {
	subject, err := m.sessions.Load(r.Context(), r)
	if err != nil || subject.ID == "" {
		return security.Guest(m.cfg.Tenant)
	}
	return subject
}

// Routes and the HTTP handlers are kept in routes.go and handlers_*.go so the
// lifecycle remains auditable in one place.
func (m *Module) Routes(r *fhttp.Router) { m.registerRoutes(r) }
