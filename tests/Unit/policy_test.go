package unit_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	skeleton "github.com/arandu-io/package-skeleton"
)

// The four properties this package exists to keep are checked here, and they
// are checked against the code rather than described in a document:
//
//  1. the policy denies every action, and has no branch that allows one;
//  2. the repository refuses a Grant that was never issued, and one issued for
//     another action;
//  3. the tenant comes from the Grant;
//  4. nothing reaches the database without passing through the first two.
//
// The fourth is checked by the handle these tests pass in. It wraps a nil
// *sql.DB, so any statement that were issued would panic and fail the test
// loudly -- which makes "the refusal happened before the query" a fact the
// suite proves rather than a comment.

// everyAction is the whole set the policy answers about. A test that listed
// four of five would pass while the fifth was open.
var everyAction = []security.Action{
	skeleton.SkeletonView,
	skeleton.SkeletonList,
	skeleton.SkeletonCreate,
	skeleton.SkeletonUpdate,
	skeleton.SkeletonDelete,
}

// administrator is the most privileged subject an application can produce. It
// is the one to test the default with: a policy that refuses an administrator
// refuses everyone.
func administrator() security.Subject {
	return security.Subject{ID: "user-1", Tenant: "acme", Roles: []string{"admin"}, Verified: true}
}

func TestThePolicyDeniesEveryActionByDefault(t *testing.T) {
	t.Parallel()

	for _, action := range everyAction {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()

			_, err := security.Authorize(context.Background(), skeleton.SkeletonPolicy{},
				administrator(), action, skeleton.Skeleton{})
			if !errors.Is(err, security.ErrForbidden) {
				t.Fatalf("an unopened policy allowed %s: got %v, want ErrForbidden", action, err)
			}
		})
	}
}

func TestThePolicyDeniesARecordOfAnotherTenant(t *testing.T) {
	t.Parallel()

	other := skeleton.Skeleton{ID: "record-1", TenantID: "globex", Name: "theirs"}

	err := skeleton.SkeletonPolicy{}.Can(context.Background(),
		administrator(), skeleton.SkeletonView, other)
	if err == nil {
		t.Fatal("the policy allowed a record belonging to another tenant")
	}
	// The message is asserted because the tenant check is the one refusal that
	// has to survive somebody opening the actions below it.
	if !strings.Contains(err.Error(), "another tenant") {
		t.Fatalf("the refusal did not name the tenant: %v", err)
	}
}

func TestThePolicyDeniesAGuest(t *testing.T) {
	t.Parallel()

	for _, action := range everyAction {
		_, err := security.Authorize(context.Background(), skeleton.SkeletonPolicy{},
			security.Guest("acme"), action, skeleton.Skeleton{})
		if !errors.Is(err, security.ErrForbidden) {
			t.Fatalf("a guest was allowed %s: got %v, want ErrForbidden", action, err)
		}
	}
}

func TestAuthorizeRefusesASubjectThatIsNobody(t *testing.T) {
	t.Parallel()

	// The zero Subject is a session that failed to load, not an anonymous
	// reader, and it is refused before the policy is consulted. A package that
	// answered it as a guest would answer a broken session as a visitor.
	_, err := security.Authorize(context.Background(), skeleton.SkeletonPolicy{},
		security.Subject{}, skeleton.SkeletonView, skeleton.Skeleton{})
	if !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("an empty subject was authorized: got %v, want ErrForbidden", err)
	}
}

// nilHandle is a handle over no database.
//
// Any statement issued through it panics, which is what makes these tests
// prove that the refusal came first: a repository that checked the Grant after
// running the query would crash here rather than pass.
func nilHandle() *data.DB { return data.Wrap(nil, data.DialectSQLite) }

func TestTheRepositoryRefusesAGrantThatWasNeverIssued(t *testing.T) {
	t.Parallel()

	repo := skeleton.NewSkeletonRepository(nilHandle())
	ctx := context.Background()
	var none security.Grant

	if _, err := repo.Find(ctx, none, "record-1"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Find accepted the zero Grant: %v", err)
	}
	if _, err := repo.List(ctx, none, data.Query{}); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("List accepted the zero Grant: %v", err)
	}
	if _, err := repo.Create(ctx, none, skeleton.Skeleton{Name: "one"}); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Create accepted the zero Grant: %v", err)
	}
	if _, err := repo.Update(ctx, none, skeleton.Skeleton{ID: "record-1"}); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Update accepted the zero Grant: %v", err)
	}
	if err := repo.Delete(ctx, none, "record-1"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Delete accepted the zero Grant: %v", err)
	}
}

func TestTheRepositoryRefusesAGrantIssuedForAnotherAction(t *testing.T) {
	t.Parallel()

	repo := skeleton.NewSkeletonRepository(nilHandle())

	// A valid Grant, for the wrong thing. This is the copy-paste between two
	// repository methods, and it is caught by the Check rather than by review.
	toDelete := security.SystemGrant(skeleton.SkeletonDelete, "acme")

	if _, err := repo.Find(context.Background(), toDelete, "record-1"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Find accepted a Grant issued for %s: %v", skeleton.SkeletonDelete, err)
	}
}

func TestASystemGrantWithoutATenantReachesNothing(t *testing.T) {
	t.Parallel()

	// A system grant with no tenant is not a grant over every customer: it is
	// the zero Grant, and it passes no check. The tenant of work that runs
	// outside a request comes from the job, the task or the row that caused it.
	repo := skeleton.NewSkeletonRepository(nilHandle())

	if _, err := repo.Find(context.Background(), security.SystemGrant(skeleton.SkeletonView, ""), "record-1"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("a system grant with no tenant reached the table: %v", err)
	}
}

func TestTheTenantComesFromTheGrant(t *testing.T) {
	t.Parallel()

	g := security.SystemGrant(skeleton.SkeletonView, "acme")
	if got := data.Tenant(g); got != "acme" {
		t.Fatalf("data.Tenant(g) = %q, want %q", got, "acme")
	}

	// And a Grant nobody issued carries no tenant at all, so a statement that
	// took its tenant from anywhere else would be reading rows this Grant does
	// not name.
	if got := data.Tenant(security.Grant{}); got != "" {
		t.Fatalf("the zero Grant carries the tenant %q, want none", got)
	}
}

func TestTheRequestValidatesItsInput(t *testing.T) {
	t.Parallel()

	if errs := (skeleton.CreateRequest{}).Validate(); !errs.Any() {
		t.Fatal("an empty request validated")
	}
	if errs := (skeleton.CreateRequest{Name: strings.Repeat("a", 121)}).Validate(); !errs.Any() {
		t.Fatal("a name past the maximum validated")
	}
	if errs := (skeleton.CreateRequest{Name: "one"}).Validate(); errs.Any() {
		t.Fatalf("a valid request was rejected: %v", errs)
	}
}

func TestTheConfigurationRefusesWhatCannotWork(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]skeleton.Config{
		"no tenant":        {},
		"tenant with a /":  {Tenant: "acme/reports"},
		"tenant uppercase": {Tenant: "Acme"},
		"relative prefix":  {Tenant: "acme", Prefix: "skeleton"},
		"page size too big": {Tenant: "acme",
			PageSize: skeleton.MaxPageSize + 1},
		"negative page size": {Tenant: "acme", PageSize: -1},
	} {
		if err := cfg.Validate(); err == nil {
			t.Errorf("the configuration with %s was accepted", name)
		}
	}

	if err := (skeleton.Config{Tenant: "acme"}).Validate(); err != nil {
		t.Fatalf("a valid configuration was refused: %v", err)
	}
}
