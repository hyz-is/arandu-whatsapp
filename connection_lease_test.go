package whatsapp

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
)

// TestConnectionLeaseIsSingleOwnerAcrossProcesses runs two repositories, which
// is what two containers on one database are, against the real table.
func TestConnectionLeaseIsSingleOwnerAcrossProcesses(t *testing.T) {
	db := migratedSQLite(t, "connection-lease")
	handle := data.Wrap(db, data.DialectSQLite)
	first := internalrepo.NewInstanceRepository(internalrepo.NewBase(handle))
	second := internalrepo.NewInstanceRepository(internalrepo.NewBase(handle))
	ctx := context.Background()

	created, err := NewInstanceRepository(handle).Create(ctx, security.SystemGrant(ActionInstanceCreate, "acme"), Instance{Name: "zen"})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(created.ID, 10)
	grant := security.SystemGrant(ActionRuntime, "acme")

	acquire := func(repo *internalrepo.SQLInstanceRepository) bool {
		t.Helper()
		held, err := repo.TryAcquireConnectionLock(ctx, grant, id)
		if err != nil {
			t.Fatal(err)
		}
		return held
	}
	expiry := func() time.Time {
		t.Helper()
		var until sql.NullTime
		if err := db.QueryRow(`SELECT connection_lock_until FROM whatsapp_instances WHERE id = ?`, created.ID).Scan(&until); err != nil {
			t.Fatal(err)
		}
		return until.Time
	}
	expire := func() {
		t.Helper()
		if _, err := db.Exec(`UPDATE whatsapp_instances SET connection_lock_until = ? WHERE id = ?`,
			time.Now().UTC().Add(-time.Second), created.ID); err != nil {
			t.Fatal(err)
		}
	}

	if !acquire(first) {
		t.Fatal("the first process could not take a free lease")
	}
	if acquire(second) {
		t.Fatal("the second process took a lease the first holds")
	}

	// The owner renews: same holder, a later expiry.
	if _, err := db.Exec(`UPDATE whatsapp_instances SET connection_lock_until = ? WHERE id = ?`,
		time.Now().UTC().Add(5*time.Second), created.ID); err != nil {
		t.Fatal(err)
	}
	before := expiry()
	if !acquire(first) {
		t.Fatal("the owner could not renew its lease")
	}
	if !expiry().After(before) {
		t.Fatalf("renewal did not extend the lease: %v then %v", before, expiry())
	}
	if acquire(second) {
		t.Fatal("the second process took a lease the first renewed")
	}

	// A release by someone who does not own it changes nothing.
	if err := second.ReleaseConnectionLock(ctx, grant, id); err != nil {
		t.Fatal(err)
	}
	if acquire(second) {
		t.Fatal("a release by a non-owner freed the lease")
	}

	// The owner closes: the other takes over at once.
	if err := first.ReleaseConnectionLock(ctx, grant, id); err != nil {
		t.Fatal(err)
	}
	if !acquire(second) {
		t.Fatal("the lease was not free after its owner released it")
	}

	// The new owner is killed: after the expiry the first takes over again.
	if acquire(first) {
		t.Fatal("the first process took a lease the second holds")
	}
	expire()
	if !acquire(first) {
		t.Fatal("the lease was not free after it expired")
	}
}
