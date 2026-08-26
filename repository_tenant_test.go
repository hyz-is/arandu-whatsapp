package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database"
	"go.mau.fi/whatsmeow/store/sqlstore"

	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
)

func TestInstanceRepositoryIsolatesTenants(t *testing.T) {
	db := migratedSQLite(t, "tenant-isolation")
	handle := data.Wrap(db, data.DialectSQLite)
	inner := internalrepo.NewInstanceRepository(internalrepo.NewBase(handle))
	repository := newInstanceRepository(inner)
	ctx := context.Background()

	acme, err := repository.Create(ctx, security.SystemGrant(ActionInstanceCreate, "acme"), Instance{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	globex, err := repository.Create(ctx, security.SystemGrant(ActionInstanceCreate, "globex"), Instance{Name: "shared"})
	if err != nil {
		t.Fatalf("same name in another tenant: %v", err)
	}
	if acme.ID == globex.ID {
		t.Fatal("generated duplicate instance IDs")
	}

	if _, err := repository.Find(ctx, security.SystemGrant(ActionInstanceView, "acme"), globex.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("acme found globex instance: %v", err)
	}
	acmeRows, err := repository.List(ctx, security.SystemGrant(ActionInstanceList, "acme"), data.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	globexRows, err := repository.List(ctx, security.SystemGrant(ActionInstanceList, "globex"), data.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(acmeRows) != 1 || acmeRows[0].ID != acme.ID {
		t.Fatalf("acme rows = %#v", acmeRows)
	}
	if len(globexRows) != 1 || globexRows[0].ID != globex.ID {
		t.Fatalf("globex rows = %#v", globexRows)
	}

	if _, err := inner.FindByID(ctx, security.Grant{}, acme.ID); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("explicit zero Grant fell back to background Grant: %v", err)
	}
}

func TestInstanceRepositoryUsesTenantScopedKeysetPagination(t *testing.T) {
	db := migratedSQLite(t, "instance-keyset-pagination")
	repository := NewInstanceRepository(data.Wrap(db, data.DialectSQLite))
	ctx := context.Background()
	createGrant := security.SystemGrant(ActionInstanceCreate, "acme")
	want := make(map[int64]struct{}, 5)
	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		instance, err := repository.Create(ctx, createGrant, Instance{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		want[instance.ID] = struct{}{}
	}
	for _, name := range []string{"globex-one", "globex-two"} {
		if _, err := repository.Create(ctx, security.SystemGrant(ActionInstanceCreate, "globex"), Instance{Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	listGrant := security.SystemGrant(ActionInstanceList, "acme")
	query := InstanceListQuery{Limit: 2}
	seen := make(map[int64]struct{}, len(want))
	firstCursor := ""
	for {
		page, err := repository.ListPage(ctx, listGrant, query)
		if err != nil {
			t.Fatal(err)
		}
		if page.PerPage != 2 || len(page.Items) > 2 {
			t.Fatalf("page metadata = perPage %d items %d", page.PerPage, len(page.Items))
		}
		for _, item := range page.Items {
			if _, ok := want[item.ID]; !ok {
				t.Fatalf("page leaked instance %d from another tenant", item.ID)
			}
			if _, duplicate := seen[item.ID]; duplicate {
				t.Fatalf("instance %d appeared in two pages", item.ID)
			}
			seen[item.ID] = struct{}{}
		}
		if page.NextCursor == "" {
			break
		}
		if firstCursor == "" {
			firstCursor = page.NextCursor
		}
		query.Cursor = page.NextCursor
	}
	if len(seen) != len(want) {
		t.Fatalf("walk returned %d instances, want %d", len(seen), len(want))
	}

	if _, err := repository.ListPage(ctx, listGrant, InstanceListQuery{Limit: 2, Cursor: "not-a-cursor"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("malformed cursor returned %v, want ErrInvalidCursor", err)
	}
	if _, err := repository.ListPage(ctx, listGrant, InstanceListQuery{Limit: MaxInstancePageLimit + 1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized page returned %v, want ErrInvalidInput", err)
	}
	filter := "echo"
	if _, err := repository.ListPage(ctx, listGrant, InstanceListQuery{Limit: 2, Cursor: firstCursor, Name: &filter}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor outside active filter returned %v, want ErrInvalidCursor", err)
	}

	globexPage, err := repository.ListPage(ctx, security.SystemGrant(ActionInstanceList, "globex"), InstanceListQuery{Limit: 1})
	if err != nil || globexPage.NextCursor == "" {
		t.Fatalf("globex cursor page = %#v, %v", globexPage, err)
	}
	if _, err := repository.ListPage(ctx, listGrant, InstanceListQuery{Limit: 2, Cursor: globexPage.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-tenant cursor returned %v, want ErrInvalidCursor", err)
	}
}

func migratedSQLite(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	connection := database.ForMigrations(database.NewConnection(db, "", "", map[string]any{
		"driver": string(database.DialectSQLite),
		"name":   migrationConnectionName,
	}))
	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	for _, migration := range whatsappMigrations(container) {
		if err := migration.Up(context.Background(), connection); err != nil {
			db.Close()
			t.Fatalf("apply %s: %v", migration.GetName(), err)
		}
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
