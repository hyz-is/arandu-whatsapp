// Package skeleton is an Arandu module: one entity, one policy that decides
// about it, one repository that is the only door to its table, and the routes
// that reach them.
//
// The files are laid out by role rather than by layer, so the whole package
// reads top to bottom:
//
//	module.go      -> registration, routes, handlers and migrations
//	config.go      -> what the application passes in
//	model.go       -> the entity, and what it may answer with
//	policy.go      -> who may do what
//	repository.go  -> data access, which requires a Grant
//	service.go     -> the rules, and the only path a handler may take
//
// An application registers it explicitly. There is no service provider, no
// container and no discovery: the wiring is three lines somebody wrote, and
// reading them is how they learn what the application is made of.
package skeleton

import (
	"context"
	"errors"
	stdhttp "net/http"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/foundation"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/hesape/database/migrations"
)

// Module is what the application registers.
//
// It implements foundation.Module, which is Name and Routes and nothing else --
// that pair is the whole public contract between a package and the framework.
//
// It also implements foundation.Migratable, because it owns a table. The other
// optional interfaces are declared beside Module in the framework and are opted
// into the same way, by implementing them: Bootable to prepare state at boot,
// Background to run a loop of its own, Schedulable to declare work for the
// scheduler, Health to report on the storage it depends on, Closable to give
// resources back at shutdown.
type Module struct {
	cfg      Config
	svc      *SkeletonService
	sessions *security.SessionStore
}

// Compile-time proof that the module honors the contracts it claims.
var (
	_ foundation.Module     = (*Module)(nil)
	_ foundation.Migratable = (*Module)(nil)
)

// New returns the module, or the reason it cannot be built.
//
// The collaborators are parameters and not fields somebody fills in afterwards:
// a module that could be registered half-wired is a module whose first request
// is the thing that reports the missing half.
//
// It returns an error rather than panicking or carrying on, because everything
// it refuses is a wiring mistake, and a wiring mistake found at boot costs one
// restart. The same mistake found later is a request that reached a nil handle.
func New(cfg Config, db *data.DB, sessions *security.SessionStore) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("skeleton: New needs a database handle: this package owns a table, and there is no in-memory mode that would let it start without one")
	}
	if sessions == nil {
		return nil, errors.New("skeleton: New needs a session store: it is where the subject comes from, and a request with no subject cannot be authorized")
	}
	cfg = cfg.withDefaults()
	return &Module{
		cfg:      cfg,
		svc:      NewSkeletonService(NewSkeletonRepository(db)),
		sessions: sessions,
	}, nil
}

// Name is the module identifier: a lowercase slug, stable, no spaces.
//
// It is what `aru route:list` groups by and what the route names are prefixed with,
// so changing it changes addresses that other code has already written down.
func (m *Module) Name() string { return "skeleton" }

// Routes registers the module's routes under the configured prefix.
//
// They are named, so a URL is built from a name rather than written out a
// second time somewhere else -- two spellings of one address disagree, and the
// failure when they do is a link to a 404.
func (m *Module) Routes(r *fhttp.Router) {
	r.Action(stdhttp.MethodGet, m.cfg.Prefix, m.index).Name("skeleton.index")
	r.Action(stdhttp.MethodGet, m.cfg.Prefix+"/{id}", m.show).Name("skeleton.show")
	r.Action(stdhttp.MethodPost, m.cfg.Prefix, m.store).Name("skeleton.store")
}

// Handlers are thin on purpose: read the input, ask the service, answer. No
// rule and no statement lives here. A handler that reached the repository would
// be a handler that skipped the policy, and the layout is what makes that
// visible rather than the type system.

// index answers a page of records.
func (m *Module) index(ctx *fhttp.Context) error {
	query := data.Query{
		Sort:   ctx.Query("sort"),
		Cursor: ctx.Query("cursor"),
		Limit:  m.cfg.PageSize,
	}

	records, err := m.svc.List(ctx.Ctx(), m.subject(ctx.Request), query)
	if err != nil {
		return m.answer(ctx, err)
	}

	// A full page is the only one that can have a successor. A short page is
	// the last one, and offering a cursor for it would be offering a next page
	// that comes back empty.
	cursor := ""
	if len(records) == m.cfg.PageSize {
		cursor = records[len(records)-1].ID
	}
	return ctx.JSON(stdhttp.StatusOK, NewCollection(records, cursor))
}

// show answers one record.
func (m *Module) show(ctx *fhttp.Context) error {
	record, err := m.svc.Find(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("id"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, NewResource(record))
}

// store creates one record.
func (m *Module) store(ctx *fhttp.Context) error {
	in := CreateRequest{Name: ctx.Input("name")}

	record, err := m.svc.Create(ctx.Ctx(), m.subject(ctx.Request), in)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusCreated, NewResource(record))
}

// subject reads who is acting from the session, and from nowhere else.
//
// A request with no readable session is a declared guest and not an empty
// subject. The difference matters: an empty subject is refused before the
// policy is consulted, because it is almost always a session that failed to
// load, and a policy asked about nobody answers about nobody. A guest reaches
// the policy and is refused there, by a rule somebody wrote -- or allowed,
// where the package means to serve a reader who never signed in.
//
// The tenant of that guest is the application's, from configuration. It is the
// one place a tenant does not come from a Grant, and it is because there is no
// Grant yet: everywhere downstream, data.Tenant is what the statements take.
func (m *Module) subject(r *stdhttp.Request) security.Subject {
	sub, err := m.sessions.Load(r.Context(), r)
	if err != nil || sub.ID == "" {
		return security.Guest(m.cfg.Tenant)
	}
	return sub
}

// answer turns what the service refused into something the client can act on.
//
// Three refusals have an answer, and everything else does not. An error this
// package did not expect is returned rather than swallowed: the framework turns
// it into the error page in development and a 500 in production, which is the
// honest outcome. Answering 200 with an empty body is the failure nobody
// debugs.
//
// A refusal is answered with a status and no detail. Telling the client why a
// policy said no is telling them what exists and what does not, one request at
// a time; the reason is in the log, where the person operating the system reads
// it and the person probing it does not.
func (m *Module) answer(ctx *fhttp.Context, err error) error {
	switch {
	case errors.Is(err, security.ErrForbidden):
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusForbidden, "forbidden")
		return nil
	case errors.Is(err, ErrNotFound):
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusNotFound, "not found")
		return nil
	}

	// A rejected input is the answer rather than a failure, and the fields that
	// were rejected are the client's own, so naming them gives nothing away.
	var rejected validation.Errors
	if errors.As(err, &rejected) {
		fhttp.Refuse(ctx.Response, ctx.Request, stdhttp.StatusUnprocessableEntity, rejected.Error())
		return nil
	}
	return err
}

// Migrations declares the schema this module owns.
//
// They are returned in the order their names sort in, which is the order they
// apply in: the name carries the order, and nothing else decides it.
func (m *Module) Migrations() []foundation.Migration {
	return []foundation.Migration{createSkeletons{}}
}

// The migration is reversible, and the assertion is here rather than discovered
// at rollback: the migrator tests for Down with a type assertion, so a Down
// with the wrong signature would leave a rollback that silently does nothing.
var _ migrations.ReversibleMigration = createSkeletons{}

// createSkeletons is the table this module owns, and the index its listing
// reads by.
type createSkeletons struct{ migrations.BaseMigration }

// GetName is the migration's identity, and it carries the order. It is fixed
// once the package is published: changing what an applied name means leaves the
// change missing everywhere it already ran, and nothing says so.
func (createSkeletons) GetName() string { return "20260823_0001_create_skeletons" }

// Up creates the table and the index the keyset pagination scans.
//
// Every type here spells the same in SQLite, PostgreSQL and MySQL, which is
// what lets one application develop on a file and deploy on Postgres without a
// second schema. The identifier columns are VARCHAR rather than TEXT because
// they take part in a key, and MySQL refuses TEXT in one without a prefix
// length. The timestamp has no database default: the value comes from Go.
func (createSkeletons) Up(ctx context.Context, conn migrations.Connection) error {
	if _, err := conn.Statement(ctx, `CREATE TABLE skeletons (
    id          VARCHAR(255) PRIMARY KEY,
    tenant_id   VARCHAR(255) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    created_at  TIMESTAMP NOT NULL
)`, nil); err != nil {
		return err
	}

	// The index matches the ORDER BY of the listing, tenant first. Without it
	// every page is a scan of every customer's rows.
	_, err := conn.Statement(ctx,
		`CREATE INDEX skeletons_tenant_created_idx ON skeletons (tenant_id, created_at, id)`, nil)
	return err
}

// Down drops the table, which takes its index with it.
func (createSkeletons) Down(ctx context.Context, conn migrations.Connection) error {
	_, err := conn.Statement(ctx, `DROP TABLE skeletons`, nil)
	return err
}
