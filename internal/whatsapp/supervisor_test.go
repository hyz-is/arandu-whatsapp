package whatsapp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	watypes "go.mau.fi/whatsmeow/types"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/config"
	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// leaseStore is the shared table of connection leases that every process of a
// deployment reads, with a clock the test moves.
type leaseStore struct {
	mu     sync.Mutex
	now    time.Time
	owner  map[string]string
	until  map[string]time.Time
	failed error
}

func newLeaseStore() *leaseStore {
	return &leaseStore{
		now:   time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
		owner: map[string]string{},
		until: map[string]time.Time{},
	}
}

func (s *leaseStore) advance(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = s.now.Add(d)
}

func (s *leaseStore) holder(instanceID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.owner[instanceID]
}

func (s *leaseStore) expiry(instanceID string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.until[instanceID]
}

// take gives the lease to owner whatever its state, as another process that
// won it would.
func (s *leaseStore) take(instanceID, owner string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.owner[instanceID] = owner
	s.until[instanceID] = s.now.Add(repository.ConnectionLockTTL)
}

func (s *leaseStore) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = err
}

// leaseLock is one process's view of the leases, with the semantics of the SQL
// repository: free, expired or already its own.
type leaseLock struct {
	store *leaseStore
	owner string
}

func (l *leaseLock) TryAcquire(_ context.Context, _ security.Grant, instanceID string) (bool, error) {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.store.failed != nil {
		return false, l.store.failed
	}
	held, until := l.store.owner[instanceID], l.store.until[instanceID]
	if held != "" && held != l.owner && until.After(l.store.now) {
		return false, nil
	}
	l.store.owner[instanceID] = l.owner
	l.store.until[instanceID] = l.store.now.Add(repository.ConnectionLockTTL)
	return true, nil
}

func (l *leaseLock) Release(_ context.Context, _ security.Grant, instanceID string) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.store.owner[instanceID] == l.owner {
		delete(l.store.owner, instanceID)
		delete(l.store.until, instanceID)
	}
	return nil
}

// restoreRepository answers the restore reads from a list the test edits.
type restoreRepository struct {
	*fakeInstanceRepository
	mu    sync.Mutex
	items []types.Instance
}

func (r *restoreRepository) setItems(items []types.Instance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = items
}

func (r *restoreRepository) FindAutoConnectTenants(context.Context, security.Grant) ([]string, error) {
	return []string{"acme"}, nil
}

func (r *restoreRepository) FindAutoConnectInstances(context.Context, security.Grant) ([]types.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]types.Instance(nil), r.items...), nil
}

// restoreFactory builds a client that holds the stored device and never
// reaches the network; err, when set, is what reading the device store fails
// with.
type restoreFactory struct{ err error }

func (restoreFactory) NewDeviceClient(context.Context, security.Grant) (*whatsmeow.Client, error) {
	return nil, errors.New("a restore never creates a device")
}

func (f restoreFactory) ClientForDevice(_ context.Context, _ security.Grant, deviceJID string) (*whatsmeow.Client, error) {
	if f.err != nil {
		return nil, f.err
	}
	jid, err := watypes.ParseJID(deviceJID)
	if err != nil {
		return nil, err
	}
	return whatsmeow.NewClient(&store.Device{ID: &jid}, nil), nil
}

func (restoreFactory) Store() *sqlstore.Container { return nil }

const restoreDevice = "553171714339.0:1@s.whatsapp.net"

func pairedInstance(id int64, name string, status types.InstanceConnectionStatus) types.Instance {
	jid, err := watypes.ParseJID(restoreDevice)
	if err != nil {
		panic(err)
	}
	device, owner := jid.String(), jid.ToNonAD().String()
	return types.Instance{
		ID: id, Name: name, Status: types.InstanceStatusOnline, ConnectionStatus: status,
		OwnerJid: &owner, WhatsAppDeviceJid: &device, WhatsAppOwnerJid: &owner,
	}
}

// process is one container: its own hub and connection service over the
// leases every process shares.
type process struct {
	svc      *Service
	hub      *WhatsAppClientHub
	repo     *restoreRepository
	connects atomic.Int32
}

func newProcess(t *testing.T, leases *leaseStore, owner string, items ...types.Instance) *process {
	t.Helper()
	p := &process{hub: NewClientHub(), repo: &restoreRepository{fakeInstanceRepository: &fakeInstanceRepository{}, items: items}}
	p.svc = &Service{
		config: config.WhatsAppConfig{
			AutoReconnect:               true,
			RestoreInterval:             5 * time.Millisecond,
			StartupReconnectConcurrency: 2,
			ConnectTimeout:              time.Second,
		},
		instances:          p.repo,
		factory:            restoreFactory{},
		hub:                p.hub,
		lock:               &leaseLock{store: leases, owner: owner},
		logger:             testSlog(),
		pairings:           newPairingManager(),
		contactSyncCancels: map[string]*contactSyncHandle{},
		skipped:            map[string]struct{}{},
	}
	p.svc.connect = func(client *whatsmeow.Client) error {
		p.connects.Add(1)
		for _, managed := range p.hub.List() {
			if managed.Client == client {
				managed.SignalConnected()
			}
		}
		return nil
	}
	t.Cleanup(func() {
		for _, managed := range p.hub.List() {
			if managed.Cancel != nil {
				managed.Cancel()
			}
		}
	})
	return p
}

func runtimeTestGrant() security.Grant { return security.SystemGrant(authz.ActionRuntime, "acme") }

func discoverTestGrant() security.Grant {
	return security.SystemGrant(authz.ActionRuntimeDiscover, "acme")
}

func (p *process) restore(t *testing.T, ctx context.Context, item types.Instance) {
	t.Helper()
	if err := p.svc.restoreOne(ctx, runtimeTestGrant(), item); err != nil {
		t.Fatalf("restoreOne: %v", err)
	}
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func messagesOf(records *hlog.Records) []string {
	var out []string
	for _, record := range records.All() {
		out = append(out, record.Message)
	}
	return out
}

func countMessages(records *hlog.Records, fragment string) int {
	n := 0
	for _, message := range messagesOf(records) {
		if strings.Contains(message, fragment) {
			n++
		}
	}
	return n
}

func TestRestoreSkippedByHeldLeaseConnectsWhenTheOwnerCloses(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	b := newProcess(t, leases, "b", item)
	logger, records := hlog.Capture()
	ctx := hlog.Into(context.Background(), logger)

	a.restore(t, ctx, item)
	if a.connects.Load() != 1 || leases.holder("1") != "a" {
		t.Fatalf("process A connects=%d holder=%q, want 1 and a", a.connects.Load(), leases.holder("1"))
	}

	b.restore(t, ctx, item)
	b.restore(t, ctx, item)
	if b.connects.Load() != 0 || b.hub.Exists("1") {
		t.Fatal("process B connected a session whose lease process A holds")
	}
	if got := countMessages(records, "restore skipped"); got != 1 {
		t.Fatalf("skipped restore logged %d times, want once: %v", got, messagesOf(records))
	}

	supervised, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.svc.Supervise(supervised, discoverTestGrant(), nil)
	}()
	t.Cleanup(func() { stop(); <-done })

	time.Sleep(60 * time.Millisecond)
	if b.connects.Load() != 0 {
		t.Fatal("the supervisor of process B connected a session process A still holds")
	}

	if err := a.svc.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown of process A: %v", err)
	}
	if leases.holder("1") == "a" {
		t.Fatal("process A still holds the lease after it shut down")
	}
	waitFor(t, "process B to take the session over", func() bool { return b.connects.Load() == 1 })
	if !b.hub.Exists("1") || leases.holder("1") != "b" {
		t.Fatalf("after the handoff B holds hub=%v lease=%q", b.hub.Exists("1"), leases.holder("1"))
	}
	waitFor(t, "the success line", func() bool { return countMessages(records, "restore succeeded") == 1 })
	if a.connects.Load() != 1 {
		t.Fatalf("process A connected %d times, want 1", a.connects.Load())
	}
}

func TestSupervisorConnectsAfterTheOwnerLeaseExpires(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	b := newProcess(t, leases, "b", item)
	ctx := context.Background()

	a.restore(t, ctx, item)
	// Process A is killed: it releases nothing and renews nothing.
	if err := b.svc.superviseOnce(ctx, discoverTestGrant()); err != nil {
		t.Fatal(err)
	}
	if b.connects.Load() != 0 {
		t.Fatal("B connected while A's lease was valid")
	}
	leases.advance(repository.ConnectionLockTTL + time.Second)
	if err := b.svc.superviseOnce(ctx, discoverTestGrant()); err != nil {
		t.Fatal(err)
	}
	if b.connects.Load() != 1 || !b.hub.Exists("1") || leases.holder("1") != "b" {
		t.Fatalf("B connects=%d hub=%v holder=%q after the lease expired", b.connects.Load(), b.hub.Exists("1"), leases.holder("1"))
	}
}

func TestSupervisorLeavesLiveClientsAndLoggedOutInstancesAlone(t *testing.T) {
	leases := newLeaseStore()
	live := pairedInstance(1, "live", types.InstanceConnectionStatusOnline)
	loggedOut := pairedInstance(2, "logged-out", types.InstanceConnectionStatusLoggedOut)
	missing := pairedInstance(3, "missing", types.InstanceConnectionStatusSessionMissing)
	broken := pairedInstance(4, "broken", types.InstanceConnectionStatusConnectionError)
	p := newProcess(t, leases, "a", live, loggedOut, missing, broken)
	ctx := context.Background()

	p.restore(t, ctx, live)
	if p.connects.Load() != 1 {
		t.Fatalf("connects = %d, want 1", p.connects.Load())
	}
	if err := p.svc.superviseOnce(ctx, discoverTestGrant()); err != nil {
		t.Fatal(err)
	}
	if got := p.connects.Load(); got != 2 {
		t.Fatalf("connects = %d, want 2: only the instance whose last connect failed is retried", got)
	}
	if p.hub.Exists("2") || p.hub.Exists("3") || !p.hub.Exists("4") {
		t.Fatalf("hub after a pass: logged_out=%v session_missing=%v connection_error=%v",
			p.hub.Exists("2"), p.hub.Exists("3"), p.hub.Exists("4"))
	}
	if leases.holder("2") != "" || leases.holder("3") != "" {
		t.Fatal("the supervisor took the lease of a logged out instance")
	}
}

func TestSuperviseDoesNothingWithoutAutoReconnect(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	p := newProcess(t, leases, "a", item)
	p.svc.config.AutoReconnect = false
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.svc.Supervise(context.Background(), discoverTestGrant(), nil)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Supervise kept running with AutoReconnect off")
	}
	if p.connects.Load() != 0 {
		t.Fatal("Supervise connected with AutoReconnect off")
	}
}

func TestSuperviseReportsEachPass(t *testing.T) {
	leases := newLeaseStore()
	p := newProcess(t, leases, "a")
	var passes atomic.Int32
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.svc.Supervise(ctx, discoverTestGrant(), func(err error) {
			if err == nil {
				passes.Add(1)
			}
		})
	}()
	waitFor(t, "two reported passes", func() bool { return passes.Load() >= 2 })
	stop()
	<-done
}

func TestHeldClientRenewsItsLeaseSoAnotherProcessCannotTakeIt(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	b := newProcess(t, leases, "b", item)
	ctx := context.Background()

	a.restore(t, ctx, item)
	first := leases.expiry("1")

	// Ninety seconds in, A renews; a minute later the original lease would be
	// over, and B still cannot connect.
	leases.advance(90 * time.Second)
	a.svc.renewLeases(ctx)
	if !leases.expiry("1").After(first) {
		t.Fatalf("lease expiry %v was not extended past %v", leases.expiry("1"), first)
	}
	leases.advance(60 * time.Second)
	if err := b.svc.superviseOnce(ctx, discoverTestGrant()); err != nil {
		t.Fatal(err)
	}
	if b.connects.Load() != 0 || b.hub.Exists("1") {
		t.Fatal("B connected a session A was renewing")
	}
	if !a.hub.Exists("1") {
		t.Fatal("A lost its client while holding the lease")
	}
}

func TestRenewalFailureOnTheDatabaseKeepsTheClient(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	a.restore(t, context.Background(), item)

	leases.setError(errors.New("database unreachable"))
	logger, records := hlog.Capture()
	a.svc.renewLeases(hlog.Into(context.Background(), logger))
	if !a.hub.Exists("1") {
		t.Fatal("a database error evicted a connected client")
	}
	if countMessages(records, "failed to renew") != 1 {
		t.Fatalf("renewal failure not logged: %v", messagesOf(records))
	}
}

func TestClientIsReleasedWhenAnotherProcessOwnsTheLease(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	a.restore(t, context.Background(), item)
	managed, _ := a.hub.GetByInstanceID("1")

	// A was paused past its lease and B took the session.
	leases.take("1", "b")
	logger, records := hlog.Capture()
	a.svc.renewLeases(hlog.Into(context.Background(), logger))

	if a.hub.Exists("1") {
		t.Fatal("A kept a client for a session another process owns")
	}
	if managed.Context != nil && managed.Context.Err() == nil {
		t.Fatal("the released client's context is still alive")
	}
	if leases.holder("1") != "b" {
		t.Fatalf("A touched B's lease: holder = %q", leases.holder("1"))
	}
	if countMessages(records, "owned by another process") != 1 {
		t.Fatalf("lost lease not logged: %v", messagesOf(records))
	}
}

func TestHoldLeasesRenewsUntilTheContextEnds(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	a.restore(t, context.Background(), item)
	first := leases.expiry("1")
	leases.advance(time.Minute)

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.svc.HoldLeases(ctx)
	}()
	waitFor(t, "a renewal", func() bool { return leases.expiry("1").After(first) })
	stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("HoldLeases did not return when its context ended")
	}
}

func TestShutdownReleasesLeasesEvenWhenTheContextIsDone(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	a.restore(t, context.Background(), item)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = a.svc.Shutdown(ctx)
	if leases.holder("1") != "" {
		t.Fatalf("lease still held by %q after Shutdown", leases.holder("1"))
	}
}

func TestRestoreOfAnInstanceThisProcessHoldsKeepsItsLease(t *testing.T) {
	leases := newLeaseStore()
	item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
	a := newProcess(t, leases, "a", item)
	ctx := context.Background()
	a.restore(t, ctx, item)
	a.restore(t, ctx, item)
	if a.connects.Load() != 1 || leases.holder("1") != "a" {
		t.Fatalf("connects=%d holder=%q, want one connect and the lease kept", a.connects.Load(), leases.holder("1"))
	}
}

func TestQRPairingSuccessKeepsTheLeaseAndFailureReleasesIt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		item       whatsmeow.QRChannelItem
		wantHolder string
	}{
		{name: "success", item: whatsmeow.QRChannelSuccess, wantHolder: "a"},
		{name: "timeout", item: whatsmeow.QRChannelTimeout, wantHolder: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leases := newLeaseStore()
			qr, err := NewQRGenerator("#ffffff", "#198754")
			if err != nil {
				t.Fatal(err)
			}
			svc := &Service{
				config:        config.WhatsAppConfig{QRCodeLimit: 5, QRCodeExpirationTime: time.Second, PairingTimeout: time.Minute},
				instances:     &fakeInstanceRepository{},
				hub:           NewClientHub(),
				lock:          &leaseLock{store: leases, owner: "a"},
				qr:            qr,
				logger:        testSlog(),
				passkeyClient: newWhatsmeowPasskeyClient,
				pairings:      newPairingManager(),
			}
			if ok, _ := svc.lock.TryAcquire(context.Background(), runtimeTestGrant(), "1"); !ok {
				t.Fatal("could not take the lease")
			}
			pairingCtx, pairingCancel := context.WithCancel(context.Background())
			instanceCtx, instanceCancel := context.WithCancel(context.Background())
			t.Cleanup(instanceCancel)
			session := &pairingSession{cancel: pairingCancel, ctx: pairingCtx, startedAt: time.Now()}
			managed := &ManagedWhatsAppClient{
				InstanceID: "1", InstanceName: "zen", RuntimeGrant: eventRuntimeGrant(),
				Context: instanceCtx, Cancel: instanceCancel, ConnectedSignal: make(chan struct{}),
			}
			svc.pairings.add(managed.InstanceID, session)
			if err := svc.hub.Register(managed); err != nil {
				t.Fatal(err)
			}
			qrChannel := make(chan whatsmeow.QRChannelItem, 1)
			qrChannel <- tc.item
			svc.consumeQRCodeChannel(pairingCtx, pairingCancel, session, managed, qrChannel, nil, nil)
			if got := leases.holder("1"); got != tc.wantHolder {
				t.Fatalf("lease holder after %s = %q, want %q", tc.name, got, tc.wantHolder)
			}
		})
	}
}

func TestRestoreMarksSessionMissingOnlyWhenTheStoreHasNoDevice(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want types.InstanceConnectionStatus
	}{
		{name: "device absent", err: ErrSessionMissing, want: types.InstanceConnectionStatusSessionMissing},
		{name: "store unreadable", err: errors.New("database unreachable"), want: types.InstanceConnectionStatusConnectionError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leases := newLeaseStore()
			item := pairedInstance(1, "zen", types.InstanceConnectionStatusOnline)
			p := newProcess(t, leases, "a", item)
			p.svc.factory = restoreFactory{err: tc.err}
			if err := p.svc.restoreOne(context.Background(), runtimeTestGrant(), item); !errors.Is(err, tc.err) {
				t.Fatalf("restoreOne error = %v, want %v", err, tc.err)
			}
			if got := p.repo.lastStatus(); got != tc.want {
				t.Fatalf("stored status = %s, want %s", got, tc.want)
			}
			if leases.holder("1") != "" || p.hub.Exists("1") {
				t.Fatal("a failed restore kept its lease or a hub entry")
			}
		})
	}
}
