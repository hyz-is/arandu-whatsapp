package whatsapp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// The module hosts every tenant of the application. At boot it reconnects the
// paired instances of each one, and discovering which tenants those are is the
// only read that crosses tenants: it answers identifiers and nothing else.
func TestBootDiscoveryListsEveryTenantWithAPairedOnlineInstance(t *testing.T) {
	db := migratedSQLite(t, "multi-tenant-discovery")
	handle := data.Wrap(db, data.DialectSQLite)
	inner := internalrepo.NewInstanceRepository(internalrepo.NewBase(handle))
	repository := newInstanceRepository(inner)
	ctx := context.Background()

	paired := func(tenant, name, jid string) {
		t.Helper()
		created, err := repository.Create(ctx, security.SystemGrant(ActionInstanceCreate, tenant), Instance{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if err := inner.UpdateStatus(ctx, security.SystemGrant(ActionConnectionPair, tenant), created.ID, dbtypes.InstanceStatusOnline); err != nil {
			t.Fatal(err)
		}
		if jid == "" {
			return
		}
		if err := inner.SaveWhatsAppDevice(ctx, security.SystemGrant(ActionRuntime, tenant), dbtypes.SaveWhatsAppDeviceInput{
			InstanceID: created.ID, DeviceJID: jid, OwnerJID: jid, PhoneNumber: "5511900000000",
		}); err != nil {
			t.Fatal(err)
		}
	}
	paired("acme", "main", "5511900000001:1@s.whatsapp.net")
	paired("acme", "second", "5511900000002:1@s.whatsapp.net")
	paired("globex", "main", "5511900000003:1@s.whatsapp.net")
	paired("initech", "unpaired", "")

	tenants, err := inner.FindAutoConnectTenants(ctx, security.SystemGrant(ActionRuntimeDiscover, "realm"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"acme", "globex"}; !reflect.DeepEqual(tenants, want) {
		t.Fatalf("discovery found %v, want %v", tenants, want)
	}
	if _, err := inner.FindAutoConnectTenants(ctx, security.SystemGrant(ActionRuntime, "acme")); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("a tenant's runtime Grant listed every tenant: %v", err)
	}

	// Each tenant's restore reads only its own instances.
	for tenant, want := range map[string]int{"acme": 2, "globex": 1} {
		found, err := inner.FindAutoConnectInstances(ctx, security.SystemGrant(ActionRuntime, tenant))
		if err != nil || len(found) != want {
			t.Fatalf("%s restores %d instances (%v), want %d", tenant, len(found), err, want)
		}
	}
}

// A subject reaches the instances of its own tenant, whichever tenant that
// is: the policy compares the instance's tenant with the subject's, and the
// module no longer pins one.
func TestEachTenantReachesItsOwnInstancesThroughTheService(t *testing.T) {
	db := migratedSQLite(t, "multi-tenant-service")
	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), 0, false, security.NewMemoryBackend())
	module, err := New(Config{
		Tenant: "realm",
		Policy: PolicyConfig{Roles: map[security.Action][]string{
			ActionInstanceCreate: {"admin"}, ActionInstanceList: {"admin"}, ActionInstanceView: {"admin"},
		}},
	}, data.Wrap(db, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin := func(tenant string) security.Subject {
		return security.Subject{ID: "admin-" + tenant, Tenant: tenant, Roles: []string{"admin"}, Verified: true}
	}
	name := "main"
	for _, tenant := range []string{"acme", "globex"} {
		if _, err := module.Service().CreateInstance(ctx, admin(tenant), CreateInstanceInput{Name: &name}); err != nil {
			t.Fatalf("%s could not create its instance: %v", tenant, err)
		}
	}
	for _, tenant := range []string{"acme", "globex"} {
		page, err := module.Service().ListInstances(ctx, admin(tenant), InstanceListQuery{})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("%s lists %d instances, want only its own", tenant, len(page.Items))
		}
	}
}
