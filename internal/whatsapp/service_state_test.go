package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	watypes "go.mau.fi/whatsmeow/types"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

func TestLogoutKeepsInstanceActiveForReconnect(t *testing.T) {
	repo := &fakeInstanceRepository{
		found: types.InstanceRecord{
			Instance: types.Instance{
				ID:               1,
				Name:             "beplus",
				Status:           types.InstanceStatusOnline,
				ConnectionStatus: types.InstanceConnectionStatusOnline,
			},
		},
	}
	svc := &Service{
		instances: repo,
		hub:       NewClientHub(),
		lock:      &fakeConnectionLock{},
		logger:    testSlog(),
	}

	grant := security.SystemGrant(authz.ActionConnectionLogout, "acme")
	if _, err := svc.Logout(context.Background(), grant, "beplus"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	instance, err := svc.authenticateInstance(context.Background(), pairingTestGrant(), "beplus")
	if err != nil {
		t.Fatalf("expected instance to remain authorized for reconnect, got %v", err)
	}
	if instance.Instance.Status != types.InstanceStatusOnline {
		t.Fatalf("expected base instance status ONLINE after logout, got %s", instance.Instance.Status)
	}
	if got := repo.lastStatus(); got != types.InstanceConnectionStatusLoggedOut {
		t.Fatalf("expected WhatsApp connection status logged_out, got %s", got)
	}
}

func TestAuthenticateInstanceRepairsLoggedOutInactiveInstance(t *testing.T) {
	repo := &fakeInstanceRepository{
		found: types.InstanceRecord{
			Instance: types.Instance{
				ID:               1,
				Name:             "beplus",
				Status:           types.InstanceStatusOffline,
				ConnectionStatus: types.InstanceConnectionStatusLoggedOut,
			},
		},
	}
	svc := &Service{
		instances: repo,
		logger:    testSlog(),
	}

	instance, err := svc.authenticateInstance(context.Background(), pairingTestGrant(), "beplus")
	if err != nil {
		t.Fatalf("authenticateInstance() error = %v", err)
	}
	if instance.Instance.Status != types.InstanceStatusOnline {
		t.Fatalf("expected repaired instance status ONLINE, got %s", instance.Instance.Status)
	}
	if repo.found.Instance.Status != types.InstanceStatusOnline {
		t.Fatalf("expected repository status ONLINE, got %s", repo.found.Instance.Status)
	}
}

func TestDeleteInstanceRepairsLoggedOutInactiveInstance(t *testing.T) {
	repo := &fakeInstanceRepository{
		found: types.InstanceRecord{
			Instance: types.Instance{
				ID:               1,
				Name:             "beplus",
				Status:           types.InstanceStatusOffline,
				ConnectionStatus: types.InstanceConnectionStatusLoggedOut,
			},
		},
	}
	svc := &Service{
		instances: repo,
		hub:       NewClientHub(),
		lock:      &fakeConnectionLock{},
		logger:    testSlog(),
	}

	grant := security.SystemGrant(authz.ActionInstanceDelete, "acme")
	result, err := svc.DeleteInstance(context.Background(), grant, "beplus", false)
	if err != nil {
		t.Fatalf("DeleteInstance() error = %v", err)
	}
	if !result.Deleted || result.InstanceName != "beplus" {
		t.Fatalf("unexpected delete result %#v", result)
	}
	if repo.found.Instance.Status != types.InstanceStatusOnline {
		t.Fatalf("expected repository status ONLINE, got %s", repo.found.Instance.Status)
	}
	if !repo.deleted {
		t.Fatal("expected repository delete to be called")
	}
}

func TestManagedConnectionStatusDistinguishesSessionPresence(t *testing.T) {
	if got := managedConnectionStatus(nil); got != types.InstanceConnectionStatusSessionMissing {
		t.Fatalf("nil managed status = %s", got)
	}

	managed := &ManagedWhatsAppClient{Client: &whatsmeow.Client{}}
	if got := managedConnectionStatus(managed); got != types.InstanceConnectionStatusSessionMissing {
		t.Fatalf("client without store ID status = %s", got)
	}

	jid := mustParseJID(t, "553171714339.0:1@s.whatsapp.net")
	managed.Client.Store = &store.Device{ID: &jid}
	if got := managedConnectionStatus(managed); got != types.InstanceConnectionStatusDisconnected {
		t.Fatalf("client with store ID but disconnected status = %s", got)
	}
}

func TestValidateManagedDeviceRejectsDeviceMismatch(t *testing.T) {
	jid := mustParseJID(t, "553171714339.0:1@s.whatsapp.net")
	expected := "553197853327.0:1@s.whatsapp.net"
	logger, records := hlog.Capture()
	managed := &ManagedWhatsAppClient{
		InstanceID:   "1",
		InstanceName: "test_001",
		Client:       &whatsmeow.Client{Store: &store.Device{ID: &jid}},
	}
	svc := &Service{logger: logger}

	ctx := hlog.Into(context.Background(), logger)
	err := svc.validateManagedDevice(ctx, types.Instance{
		ID:                1,
		Name:              "test_001",
		WhatsAppDeviceJid: &expected,
	}, managed)
	if !errors.Is(err, ErrDeviceMismatch) {
		t.Fatalf("expected ErrDeviceMismatch, got %v", err)
	}
	if records.Len() != 1 {
		t.Fatalf("captured %d records, want 1", records.Len())
	}
	for _, value := range records.All()[0].Attrs {
		if value == jid.String() || value == jid.ToNonAD().String() || value == expected {
			t.Fatalf("log exposed a complete JID: %#v", records.All()[0])
		}
	}
}

func TestValidateManagedDeviceAcceptsPersistedDeviceAndOwner(t *testing.T) {
	jid := mustParseJID(t, "553171714339.0:1@s.whatsapp.net")
	device := jid.String()
	owner := jid.ToNonAD().String()
	managed := &ManagedWhatsAppClient{
		InstanceID:   "1",
		InstanceName: "test_001",
		Client:       &whatsmeow.Client{Store: &store.Device{ID: &jid}},
	}
	svc := &Service{logger: testSlog()}

	err := svc.validateManagedDevice(context.Background(), types.Instance{
		ID:                1,
		Name:              "test_001",
		OwnerJid:          &owner,
		WhatsAppDeviceJid: &device,
		WhatsAppOwnerJid:  &owner,
	}, managed)
	if err != nil {
		t.Fatalf("validateManagedDevice() error = %v", err)
	}
	if managed.DeviceJID != device || managed.OwnerJID != owner {
		t.Fatalf("managed JIDs not synchronized: device=%q owner=%q", managed.DeviceJID, managed.OwnerJID)
	}
}

func mustParseJID(t *testing.T, raw string) watypes.JID {
	t.Helper()
	jid, err := watypes.ParseJID(raw)
	if err != nil {
		t.Fatalf("ParseJID(%q): %v", raw, err)
	}
	return jid
}
