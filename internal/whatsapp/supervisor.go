package whatsapp

import (
	"context"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/config"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// leaseReleaseTimeout bounds how long Shutdown spends handing the connection
// leases back, whatever state the context it was given is in.
const leaseReleaseTimeout = 5 * time.Second

// connectClient opens the WhatsApp connection of a restored client.
func (s *Service) connectClient(client *whatsmeow.Client) error {
	if s.connect != nil {
		return s.connect(client)
	}
	return client.Connect()
}

// restoreInterval is the period of the lease renewal and of the restore
// passes.
func (s *Service) restoreInterval() time.Duration {
	if s.config.RestoreInterval > 0 {
		return s.config.RestoreInterval
	}
	return config.DefaultRestoreInterval
}

// jittered spreads d by up to 20% either way, so containers started together
// do not keep trying in step.
func jittered(d time.Duration) time.Duration {
	spread := int64(d / 5)
	if spread <= 0 {
		return d
	}
	return d - time.Duration(spread) + time.Duration(rand.Int64N(2*spread+1))
}

// sleep waits for d and answers false when ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// HoldLeases keeps the connection lease of every client this process holds,
// until ctx ends. The lease is what stops a second process from connecting a
// session this one has connected; it lasts a few minutes, so it is renewed
// every RestoreInterval.
//
// A lease that cannot be renewed because another process now owns it means
// that process is connecting the same session. This one lets go of its client
// rather than fight for the socket.
func (s *Service) HoldLeases(ctx context.Context) {
	for sleep(ctx, jittered(s.restoreInterval())) {
		s.renewLeases(ctx)
	}
}

func (s *Service) renewLeases(ctx context.Context) {
	for _, managed := range s.hub.List() {
		if ctx.Err() != nil {
			return
		}
		held, err := s.lock.TryAcquire(ctx, managed.RuntimeGrant, managed.InstanceID)
		if err != nil {
			hlog.For(ctx).WarnContext(ctx, "failed to renew the WhatsApp connection lease",
				"component", "whatsapp_service", "error", err,
				"instance_id", managed.InstanceID, "instance_name", managed.InstanceName)
			continue
		}
		if held {
			continue
		}
		hlog.For(ctx).ErrorContext(ctx, "WhatsApp connection lease is owned by another process; disconnecting this process's client",
			"component", "whatsapp_service",
			"instance_id", managed.InstanceID, "instance_name", managed.InstanceName)
		s.releaseClient(managed)
	}
}

// releaseClient stops a client without logging the device out and without
// touching the lease, which belongs to someone else.
func (s *Service) releaseClient(managed *ManagedWhatsAppClient) {
	if managed.Cancel != nil {
		managed.Cancel()
	}
	if managed.Client != nil {
		managed.Client.Disconnect()
	}
	s.cancelContactSync(managed.InstanceID)
	s.hub.Remove(managed.InstanceID)
}

// Supervise keeps trying to connect the paired sessions this process holds no
// connection for, until ctx ends. Boot restore runs once; a lease held by
// another container then, or a connect that failed, would otherwise leave the
// session disconnected until the next boot.
//
// Each pass reads the tenants with instances to reconnect and restores, under
// each tenant's runtime Grant, the instances that have no client in this
// process and whose stored status is neither logged_out nor session_missing.
// It never pairs and never asks for a QR code: it only reconnects a device
// WhatsApp already knows. The connection lease stays the single-owner guard,
// so a session another process still holds is left alone and tried again at
// the next pass.
//
// report, when it is not nil, receives the result of every pass that was not
// cut short by ctx, so a host can tell a restore that recovered from one that
// keeps failing.
func (s *Service) Supervise(ctx context.Context, discover security.Grant, report func(error)) {
	if !s.config.AutoReconnect {
		return
	}
	for sleep(ctx, jittered(s.restoreInterval())) {
		err := s.superviseOnce(ctx, discover)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			hlog.For(ctx).ErrorContext(ctx, "WhatsApp restore pass failed",
				"component", "whatsapp_service", "error", err)
		}
		if report != nil {
			report(err)
		}
	}
}

func (s *Service) superviseOnce(ctx context.Context, discover security.Grant) error {
	tenants, err := s.instances.FindAutoConnectTenants(ctx, discover)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		//arandu:system-grant the restore pass of one tenant's paired instances, named by the discovery read
		grant := security.SystemGrant(authz.ActionRuntime, tenant)
		items, err := s.instances.FindAutoConnectInstances(ctx, grant)
		if err != nil {
			hlog.For(ctx).ErrorContext(ctx, "failed to list WhatsApp instances to restore",
				"component", "whatsapp_service", "error", err)
			continue
		}
		if err := s.restoreInstances(ctx, grant, s.pendingRestore(ctx, items)); err != nil {
			return err
		}
	}
	return nil
}

// pendingRestore keeps the instances a restore pass should try: paired,
// without a client in this process and not logged out.
func (s *Service) pendingRestore(ctx context.Context, items []types.Instance) []types.Instance {
	pending := make([]types.Instance, 0, len(items))
	for _, item := range items {
		if s.hub.Exists(strconv.FormatInt(item.ID, 10)) {
			s.noteRestored(ctx, item)
			continue
		}
		switch item.ConnectionStatus {
		case types.InstanceConnectionStatusLoggedOut, types.InstanceConnectionStatusSessionMissing:
			continue
		}
		pending = append(pending, item)
	}
	return pending
}

// noteRestoreSkipped logs a restore that found the connection lease held by
// another process. The restore supervisor tries again, so it is logged the
// first time only until the instance connects.
func (s *Service) noteRestoreSkipped(ctx context.Context, item types.Instance) {
	id := strconv.FormatInt(item.ID, 10)
	s.skippedMu.Lock()
	_, already := s.skipped[id]
	if s.skipped == nil {
		s.skipped = make(map[string]struct{})
	}
	s.skipped[id] = struct{}{}
	s.skippedMu.Unlock()
	if already {
		return
	}
	hlog.For(ctx).InfoContext(ctx, "WhatsApp restore skipped: the connection lease is held by another process; it will be retried",
		"component", "whatsapp_service", "instance_id", item.ID, "instance_name", item.Name)
}

// noteRestored logs that an instance whose restore was skipped is now held by
// this process.
func (s *Service) noteRestored(ctx context.Context, item types.Instance) {
	id := strconv.FormatInt(item.ID, 10)
	s.skippedMu.Lock()
	_, was := s.skipped[id]
	delete(s.skipped, id)
	s.skippedMu.Unlock()
	if !was {
		return
	}
	hlog.For(ctx).InfoContext(ctx, "WhatsApp restore succeeded after the connection lease was released",
		"component", "whatsapp_service", "instance_id", item.ID, "instance_name", item.Name)
}
