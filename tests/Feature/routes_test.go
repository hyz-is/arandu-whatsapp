package feature_test

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/migrations"

	skeleton "github.com/arandu-io/package-skeleton"
)

// These tests drive the module the way an application does: build it, register
// its routes on a router, and make a request.
//
// The database handle wraps nothing, and that is the assertion. A request that
// reached a statement would panic, so every answer below is proof that the
// refusal happened in the policy and not after a read.

// appKey is the key a session store is built over. Any thirty-two bytes will
// do here; a real application reads its own from the environment.
const appKey = "0123456789abcdef0123456789abcdef"

// mount builds the module and returns a router with its routes registered.
func mount(t *testing.T, cfg skeleton.Config) *fhttp.Router {
	t.Helper()

	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())

	module, err := skeleton.New(cfg, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatalf("building the module: %v", err)
	}

	router := fhttp.NewRouter()
	module.Routes(router.ForModule(module.Name()))
	return router
}

// answer makes one request against the router and returns the recorder.
func answer(t *testing.T, router *fhttp.Router, method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAVisitorWithNoSessionReachesNothing(t *testing.T) {
	t.Parallel()

	router := mount(t, skeleton.Config{Tenant: "acme"})

	for _, request := range []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, skeleton.DefaultPrefix, ""},
		{http.MethodGet, skeleton.DefaultPrefix + "/record-1", ""},
		{http.MethodPost, skeleton.DefaultPrefix, "name=one"},
	} {
		rec := answer(t, router, request.method, request.target, request.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s answered %d, want %d", request.method, request.target, rec.Code, http.StatusForbidden)
		}
	}
}

func TestARejectedInputIsAnsweredBeforeTheDatabase(t *testing.T) {
	t.Parallel()

	router := mount(t, skeleton.Config{Tenant: "acme"})

	// The input is validated before anything is authorized, so this is the one
	// refusal that arrives as 422 rather than 403 -- and it still never reaches
	// a statement.
	rec := answer(t, router, http.MethodPost, skeleton.DefaultPrefix, "name=")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an empty name answered %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestTheModuleRegistersItsRoutesUnderItsPrefix(t *testing.T) {
	t.Parallel()

	router := mount(t, skeleton.Config{Tenant: "acme", Prefix: "/widgets"})

	got := make([]string, 0, 3)
	for _, route := range router.Routes() {
		if route.Module != "skeleton" {
			t.Errorf("the route %s %s is not tagged with the module name: %q", route.Method, route.Pattern, route.Module)
		}
		got = append(got, route.Method+" "+route.Pattern)
	}
	sort.Strings(got)

	want := []string{"GET /widgets", "GET /widgets/{id}", "POST /widgets"}
	if len(got) != len(want) {
		t.Fatalf("registered %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("registered %v, want %v", got, want)
		}
	}
}

func TestNewRefusesAWiringThatCannotWork(t *testing.T) {
	t.Parallel()

	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	handle := data.Wrap(nil, data.DialectSQLite)
	valid := skeleton.Config{Tenant: "acme"}

	if _, err := skeleton.New(skeleton.Config{}, handle, sessions); err == nil {
		t.Error("a configuration with no tenant was accepted")
	}
	if _, err := skeleton.New(valid, nil, sessions); err == nil {
		t.Error("a nil database handle was accepted")
	}
	if _, err := skeleton.New(valid, handle, nil); err == nil {
		t.Error("a nil session store was accepted")
	}
	if _, err := skeleton.New(valid, handle, sessions); err != nil {
		t.Fatalf("a valid wiring was refused: %v", err)
	}
}

func TestTheModuleDeclaresItsSchema(t *testing.T) {
	t.Parallel()

	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	module, err := skeleton.New(skeleton.Config{Tenant: "acme"}, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatalf("building the module: %v", err)
	}

	declared := module.Migrations()
	if len(declared) == 0 {
		t.Fatal("the module declares migrations = true and returns none")
	}

	names := make([]string, 0, len(declared))
	for _, migration := range declared {
		name := migration.GetName()
		if name == "" {
			t.Fatal("a migration has no name, and the name is what carries the order")
		}
		names = append(names, name)

		// A migration that cannot be rolled back is a deploy that cannot be
		// undone. The migrator finds Down by type assertion, so a Down with the
		// wrong signature is a rollback that silently does nothing.
		if _, ok := migration.(migrations.ReversibleMigration); !ok {
			t.Errorf("the migration %s has no Down", name)
		}
	}

	if !sort.StringsAreSorted(names) {
		t.Fatalf("the migrations are not returned in the order their names sort in: %v", names)
	}
}
