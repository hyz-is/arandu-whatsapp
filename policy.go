package whatsapp

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
)

// Instance actions identify permissions for instance lifecycle operations.
const (
	ActionInstanceCreate security.Action = authz.ActionInstanceCreate
	ActionInstanceList   security.Action = authz.ActionInstanceList
	ActionInstanceView   security.Action = authz.ActionInstanceView
	ActionInstanceUpdate security.Action = authz.ActionInstanceUpdate
	ActionInstanceDelete security.Action = authz.ActionInstanceDelete
)

// Connection actions identify permissions for pairing and session operations.
const (
	ActionConnectionPair   security.Action = authz.ActionConnectionPair
	ActionConnectionView   security.Action = authz.ActionConnectionView
	ActionConnectionLogout security.Action = authz.ActionConnectionLogout
)

// Webhook actions identify permissions for webhook configuration operations.
const (
	ActionWebhookSet  security.Action = authz.ActionWebhookSet
	ActionWebhookView security.Action = authz.ActionWebhookView
)

// Message actions identify permissions for message operations.
const (
	ActionMessageSend          security.Action = authz.ActionMessageSend
	ActionMessageList          security.Action = authz.ActionMessageList
	ActionMessageRead          security.Action = authz.ActionMessageRead
	ActionMessageDelete        security.Action = authz.ActionMessageDelete
	ActionMessageEdit          security.Action = authz.ActionMessageEdit
	ActionMessageMediaDownload security.Action = authz.ActionMessageMediaDownload
)

// Contact, chat, profile and call actions identify their respective permissions.
const (
	ActionContactCheck       security.Action = authz.ActionContactCheck
	ActionChatArchive        security.Action = authz.ActionChatArchive
	ActionProfilePictureView security.Action = authz.ActionProfilePictureView
	ActionCallReject         security.Action = authz.ActionCallReject
)

// Group actions identify permissions for group operations.
const (
	ActionGroupCreate            security.Action = authz.ActionGroupCreate
	ActionGroupPictureUpdate     security.Action = authz.ActionGroupPictureUpdate
	ActionGroupInviteView        security.Action = authz.ActionGroupInviteView
	ActionGroupInviteRevoke      security.Action = authz.ActionGroupInviteRevoke
	ActionGroupParticipantUpdate security.Action = authz.ActionGroupParticipantUpdate
	ActionGroupLeave             security.Action = authz.ActionGroupLeave
)

// ActionRuntime identifies the internal permission used by background work.
const ActionRuntime security.Action = authz.ActionRuntime

var publicActions = [...]security.Action{
	ActionInstanceCreate, ActionInstanceList, ActionInstanceView, ActionInstanceUpdate, ActionInstanceDelete,
	ActionConnectionPair, ActionConnectionView, ActionConnectionLogout,
	ActionWebhookSet, ActionWebhookView,
	ActionMessageSend, ActionMessageList, ActionMessageRead, ActionMessageDelete,
	ActionMessageEdit, ActionMessageMediaDownload, ActionContactCheck, ActionChatArchive,
	ActionProfilePictureView, ActionCallReject, ActionGroupCreate, ActionGroupPictureUpdate,
	ActionGroupInviteView, ActionGroupInviteRevoke, ActionGroupParticipantUpdate, ActionGroupLeave,
}

// Actions is a snapshot of the complete public permission vocabulary.
var Actions = append([]security.Action(nil), publicActions[:]...)

// InstancePolicy protects every operation on WhatsApp instances and their
// children. No role is enabled unless Config.Policy explicitly lists it.
type InstancePolicy struct{ roles map[security.Action][]string }

// NewInstancePolicy builds a default-deny policy from typed role mappings.
func NewInstancePolicy(cfg PolicyConfig) InstancePolicy {
	roles := make(map[security.Action][]string, len(cfg.Roles))
	for action, allowed := range cfg.Roles {
		roles[action] = append([]string(nil), allowed...)
	}
	return InstancePolicy{roles: roles}
}

var _ security.Policy[Instance] = InstancePolicy{}

// Can decides whether a subject may perform an action on an instance.
func (p InstancePolicy) Can(_ context.Context, subject security.Subject, action security.Action, instance Instance) error {
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

func isWhatsAppAction(action security.Action) bool {
	for _, candidate := range publicActions {
		if action == candidate {
			return true
		}
	}
	return false
}
