// Package policies contains the module's authorization decisions.
package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/app/Models"
	"github.com/hyz-is/arandu-whatsapp/config"
)

// InstancePolicy protects every operation on WhatsApp instances and their
// children. No role is enabled unless config.PolicyConfig explicitly lists it.
type InstancePolicy struct{ roles map[security.Action][]string }

// NewInstancePolicy builds a default-deny policy from typed role mappings.
func NewInstancePolicy(cfg config.PolicyConfig) InstancePolicy {
	roles := make(map[security.Action][]string, len(cfg.Roles))
	for action, allowed := range cfg.Roles {
		roles[action] = append([]string(nil), allowed...)
	}
	return InstancePolicy{roles: roles}
}

var _ security.Policy[models.Instance] = InstancePolicy{}

// Can decides whether a subject may perform an action on an instance.
func (p InstancePolicy) Can(_ context.Context, subject security.Subject, action security.Action, instance models.Instance) error {
	if instance.TenantID != "" && instance.TenantID != subject.Tenant {
		return fmt.Errorf("whatsapp instance belongs to another tenant")
	}
	for _, role := range p.roles[action] {
		if subject.HasRole(role) {
			return nil
		}
	}
	return fmt.Errorf("no configured role allows %s", action)
}
