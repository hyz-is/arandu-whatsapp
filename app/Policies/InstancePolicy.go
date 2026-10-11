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
// children. No role or action is enabled unless config.PolicyConfig explicitly
// lists it.
type InstancePolicy struct {
	roles   map[security.Action][]string
	actions map[security.Action]bool
}

// NewInstancePolicy builds a default-deny policy from typed role mappings and
// the actions decided by the subject's own actions.
func NewInstancePolicy(cfg config.PolicyConfig) InstancePolicy {
	roles := make(map[security.Action][]string, len(cfg.Roles))
	for action, allowed := range cfg.Roles {
		roles[action] = append([]string(nil), allowed...)
	}
	actions := make(map[security.Action]bool, len(cfg.Actions))
	for _, action := range cfg.Actions {
		actions[action] = true
	}
	return InstancePolicy{roles: roles, actions: actions}
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
	if p.actions[action] && !subject.IsGuest() && subject.Can(action) {
		return nil
	}
	return fmt.Errorf("no configured role or action allows %s", action)
}
