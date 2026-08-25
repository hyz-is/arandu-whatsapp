package skeleton

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"
)

// The actions of Skeleton. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because an application registers many
// packages, and the name of an action shows up in logs and in audit trails
// where "view" on its own says nothing about what was viewed.
const (
	// SkeletonView is reading one record.
	SkeletonView security.Action = "skeleton.view"
	// SkeletonList is paging through the records.
	SkeletonList security.Action = "skeleton.list"
	// SkeletonCreate is adding one.
	SkeletonCreate security.Action = "skeleton.create"
	// SkeletonUpdate is changing one.
	SkeletonUpdate security.Action = "skeleton.update"
	// SkeletonDelete is removing one.
	SkeletonDelete security.Action = "skeleton.delete"
)

// SkeletonPolicy is the only authority over who does what with a Skeleton.
//
// IT DENIES EVERYTHING, and that is the state to start from rather than a
// placeholder to delete. A policy shipped with a branch that allows every
// action is a hole in every application that installs the package, and the hole
// looks like working code until somebody reads it.
//
// There is deliberately no such branch to remove. Open one action at a time,
// inside the custom block below, saying who may take it and on which record --
// what is not written there stays closed, including every action added later.
type SkeletonPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other. A
// policy that drifted onto another type would leave this one unguarded, and the
// repository would still compile.
var _ security.Policy[Skeleton] = SkeletonPolicy{}

// Can decides whether the subject may perform the action on the record.
//
// It is the only place that decides. Nothing reaches the repository without a
// Grant, and the only way to obtain a Grant is for this method to return nil.
func (SkeletonPolicy) Can(ctx context.Context, s security.Subject, a security.Action, record Skeleton) error {
	// Tenant isolation comes first and applies to every action. Without it every
	// check below would be pointless in a multi-tenant system: a rule that
	// allows an owner to read their own record would allow it across customers
	// as soon as two of them have a record with the same identifier.
	//
	// The empty id is the candidate that has not been stored yet, which belongs
	// to nobody until it is written with the tenant off the Grant.
	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("skeleton belongs to another tenant")
	}

	// arandu:begin custom
	// The rules of this package go here, one action at a time. A rule that
	// depends on the record and not only on the role is written the same way:
	//
	//	if a == SkeletonView && (s.ID == record.ID || s.HasRole("admin")) {
	//		return nil
	//	}
	//
	// A guest is a reader the caller declared anonymous on purpose, and is the
	// only subject that arrives without an id. Answer it explicitly or it falls
	// through to the refusal below, which is the safe direction:
	//
	//	if a == SkeletonView && s.IsGuest() && record.Published {
	//		return nil
	//	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on skeleton", a)
}
