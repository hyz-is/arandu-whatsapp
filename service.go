package skeleton

import (
	"context"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
)

// SkeletonService holds the rules of this package.
//
// It receives its collaborators through the constructor. There is no container
// and no resolution by reflection: what this service is made of is written at
// the one place that builds it, and reading that place is how somebody learns
// what the package touches.
//
// Everything a handler is allowed to do goes through here, which is what keeps
// the repository out of reach of the request layer. A handler that could reach
// the repository would be a handler that could skip the policy.
type SkeletonService struct {
	repo   *SkeletonRepository
	policy SkeletonPolicy
}

// NewSkeletonService wires the service over a repository.
func NewSkeletonService(repo *SkeletonRepository) *SkeletonService {
	return &SkeletonService{repo: repo}
}

// CreateRequest is the input contract.
//
// The fields are explicit and there is no mass assignment, so a request body
// cannot write a column nobody meant to expose. There is no TenantID here and
// there must never be one: the tenant comes from the Grant, which comes from
// the session.
type CreateRequest struct {
	// Name is what the record will be called.
	Name string
}

// Validate reports the errors per field.
func (r CreateRequest) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "name", r.Name)
	validation.MaxLen(e, "name", r.Name, 120)
	return e
}

// Compile-time proof that the request honors the validation contract.
var _ validation.Validatable = CreateRequest{}

// Create is the whole path in one function: validate, authorize, then act with
// the Grant the authorization produced.
//
// The candidate is authorized before it is stored, and the candidate is what
// the policy sees -- so a rule about what may be created is a rule about the
// record being created, and not about the person alone.
func (s *SkeletonService) Create(ctx context.Context, actor security.Subject, in CreateRequest) (Skeleton, error) {
	if errs := in.Validate(); errs.Any() {
		return Skeleton{}, errs
	}

	candidate := Skeleton{TenantID: actor.Tenant, Name: in.Name}

	g, err := security.Authorize(ctx, s.policy, actor, SkeletonCreate, candidate)
	if err != nil {
		return Skeleton{}, err
	}
	return s.repo.Create(ctx, g, candidate)
}

// Find returns one record, and asks the policy twice.
//
// The first call is on the empty candidate, because there is no way to read the
// record without a Grant and no way to hold a Grant without a decision. What it
// decides is whether this subject may view records of this kind at all.
//
// The second call is on the record that came back, and it is the one a rule
// about the record itself depends on: the first call saw an empty value, so
// anything the policy says about who owns the row, or about a row that is not
// published yet, never ran. Without it a policy can be written that looks
// correct, reads correctly, and is never consulted about the thing it protects.
//
// The read itself is already scoped by data.Tenant, so the second call is not
// what keeps customers apart. It is what keeps the policy honest.
func (s *SkeletonService) Find(ctx context.Context, actor security.Subject, id string) (Skeleton, error) {
	g, err := security.Authorize(ctx, s.policy, actor, SkeletonView, Skeleton{})
	if err != nil {
		return Skeleton{}, err
	}

	record, err := s.repo.Find(ctx, g, id)
	if err != nil {
		return Skeleton{}, err
	}

	if _, err := security.Authorize(ctx, s.policy, actor, SkeletonView, record); err != nil {
		return Skeleton{}, err
	}
	return record, nil
}

// List returns a page of records.
//
// It authorizes once, on the empty candidate, and the tenant filter in the
// statement is what bounds the rows. A policy call per row would be one call
// per record on a page and would still not narrow the query -- a listing that
// has to read a customer's rows in order to decide it may not read them has
// already read them.
//
// A rule that hides individual records from a listing belongs in the statement,
// as a predicate, and the action here is what decides whether the listing may
// run at all.
func (s *SkeletonService) List(ctx context.Context, actor security.Subject, q data.Query) ([]Skeleton, error) {
	g, err := security.Authorize(ctx, s.policy, actor, SkeletonList, Skeleton{})
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, g, q)
}
