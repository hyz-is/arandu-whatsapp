// Package authz owns the authorization vocabulary shared by the public
// package and its internal persistence adapters.
package authz

import (
	"fmt"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
)

const (
	ActionInstanceCreate security.Action = "whatsapp.instance.create"
	ActionInstanceList   security.Action = "whatsapp.instance.list"
	ActionInstanceView   security.Action = "whatsapp.instance.view"
	ActionInstanceUpdate security.Action = "whatsapp.instance.update"
	ActionInstanceDelete security.Action = "whatsapp.instance.delete"

	ActionConnectionPair   security.Action = "whatsapp.connection.pair"
	ActionConnectionView   security.Action = "whatsapp.connection.view"
	ActionConnectionLogout security.Action = "whatsapp.connection.logout"

	ActionWebhookSet  security.Action = "whatsapp.webhook.set"
	ActionWebhookView security.Action = "whatsapp.webhook.view"

	ActionMessageSend          security.Action = "whatsapp.message.send"
	ActionMessageList          security.Action = "whatsapp.message.list"
	ActionMessageRead          security.Action = "whatsapp.message.read"
	ActionMessageDelete        security.Action = "whatsapp.message.delete"
	ActionMessageEdit          security.Action = "whatsapp.message.edit"
	ActionMessageMediaDownload security.Action = "whatsapp.message.media.download"

	ActionContactCheck       security.Action = "whatsapp.contact.check"
	ActionChatArchive        security.Action = "whatsapp.chat.archive"
	ActionProfilePictureView security.Action = "whatsapp.profile.picture.view"
	ActionCallReject         security.Action = "whatsapp.call.reject"

	ActionGroupCreate            security.Action = "whatsapp.group.create"
	ActionGroupPictureUpdate     security.Action = "whatsapp.group.picture.update"
	ActionGroupInviteView        security.Action = "whatsapp.group.invite.view"
	ActionGroupInviteRevoke      security.Action = "whatsapp.group.invite.revoke"
	ActionGroupParticipantUpdate security.Action = "whatsapp.group.participant.update"
	ActionGroupLeave             security.Action = "whatsapp.group.leave"

	ActionRuntime security.Action = "whatsapp.runtime"
)

// CheckInstanceLookup accepts only actions whose service flow must load an
// instance before applying the record-level policy decision.
func CheckInstanceLookup(grant security.Grant) error {
	switch grant.Action() {
	case ActionInstanceView:
		return grant.Check(ActionInstanceView)
	case ActionConnectionPair:
		return grant.Check(ActionConnectionPair)
	case ActionConnectionView:
		return grant.Check(ActionConnectionView)
	case ActionConnectionLogout:
		return grant.Check(ActionConnectionLogout)
	case ActionInstanceDelete:
		return grant.Check(ActionInstanceDelete)
	case ActionWebhookSet:
		return grant.Check(ActionWebhookSet)
	case ActionWebhookView:
		return grant.Check(ActionWebhookView)
	case ActionMessageSend:
		return grant.Check(ActionMessageSend)
	case ActionMessageList:
		return grant.Check(ActionMessageList)
	case ActionMessageRead:
		return grant.Check(ActionMessageRead)
	case ActionMessageDelete:
		return grant.Check(ActionMessageDelete)
	case ActionMessageEdit:
		return grant.Check(ActionMessageEdit)
	case ActionMessageMediaDownload:
		return grant.Check(ActionMessageMediaDownload)
	case ActionContactCheck:
		return grant.Check(ActionContactCheck)
	case ActionChatArchive:
		return grant.Check(ActionChatArchive)
	case ActionProfilePictureView:
		return grant.Check(ActionProfilePictureView)
	case ActionCallReject:
		return grant.Check(ActionCallReject)
	case ActionGroupCreate:
		return grant.Check(ActionGroupCreate)
	case ActionGroupPictureUpdate:
		return grant.Check(ActionGroupPictureUpdate)
	case ActionGroupInviteView:
		return grant.Check(ActionGroupInviteView)
	case ActionGroupInviteRevoke:
		return grant.Check(ActionGroupInviteRevoke)
	case ActionGroupParticipantUpdate:
		return grant.Check(ActionGroupParticipantUpdate)
	case ActionGroupLeave:
		return grant.Check(ActionGroupLeave)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// CheckInstanceStatus accepts the closed set of operations that may reactivate
// an instance or persist a lifecycle status transition.
func CheckInstanceStatus(grant security.Grant) error {
	switch grant.Action() {
	case ActionConnectionPair:
		return grant.Check(ActionConnectionPair)
	case ActionConnectionView:
		return grant.Check(ActionConnectionView)
	case ActionConnectionLogout:
		return grant.Check(ActionConnectionLogout)
	case ActionInstanceDelete:
		return grant.Check(ActionInstanceDelete)
	case ActionMessageSend:
		return grant.Check(ActionMessageSend)
	case ActionMessageList:
		return grant.Check(ActionMessageList)
	case ActionMessageRead:
		return grant.Check(ActionMessageRead)
	case ActionMessageDelete:
		return grant.Check(ActionMessageDelete)
	case ActionMessageEdit:
		return grant.Check(ActionMessageEdit)
	case ActionMessageMediaDownload:
		return grant.Check(ActionMessageMediaDownload)
	case ActionContactCheck:
		return grant.Check(ActionContactCheck)
	case ActionChatArchive:
		return grant.Check(ActionChatArchive)
	case ActionProfilePictureView:
		return grant.Check(ActionProfilePictureView)
	case ActionCallReject:
		return grant.Check(ActionCallReject)
	case ActionGroupCreate:
		return grant.Check(ActionGroupCreate)
	case ActionGroupPictureUpdate:
		return grant.Check(ActionGroupPictureUpdate)
	case ActionGroupInviteView:
		return grant.Check(ActionGroupInviteView)
	case ActionGroupInviteRevoke:
		return grant.Check(ActionGroupInviteRevoke)
	case ActionGroupParticipantUpdate:
		return grant.Check(ActionGroupParticipantUpdate)
	case ActionGroupLeave:
		return grant.Check(ActionGroupLeave)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// CheckConnectionState accepts request pairing/logout transitions and module
// runtime transitions.
func CheckConnectionState(grant security.Grant) error {
	switch grant.Action() {
	case ActionConnectionPair:
		return grant.Check(ActionConnectionPair)
	case ActionConnectionLogout:
		return grant.Check(ActionConnectionLogout)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// CheckConnectionLock accepts the operations that own the instance connection
// lifecycle lock.
func CheckConnectionLock(grant security.Grant) error {
	switch grant.Action() {
	case ActionConnectionPair:
		return grant.Check(ActionConnectionPair)
	case ActionConnectionLogout:
		return grant.Check(ActionConnectionLogout)
	case ActionInstanceDelete:
		return grant.Check(ActionInstanceDelete)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// CheckClearDevice accepts explicit logout and WhatsApp runtime logout events.
func CheckClearDevice(grant security.Grant) error {
	switch grant.Action() {
	case ActionConnectionLogout:
		return grant.Check(ActionConnectionLogout)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// CheckMessageCreate accepts only the permission that initiated a send. Async
// send jobs preserve that Grant rather than upgrading to a system Grant.
func CheckMessageCreate(grant security.Grant) error {
	return grant.Check(ActionMessageSend)
}

// CheckMessageByID accepts the three semantic message lookup operations.
func CheckMessageByID(grant security.Grant) error {
	switch grant.Action() {
	case ActionMessageSend:
		return grant.Check(ActionMessageSend)
	case ActionMessageDelete:
		return grant.Check(ActionMessageDelete)
	case ActionMessageMediaDownload:
		return grant.Check(ActionMessageMediaDownload)
	default:
		return forbidden(grant)
	}
}

// CheckMessageByKey accepts media lookups and inbound runtime event lookups.
func CheckMessageByKey(grant security.Grant) error {
	switch grant.Action() {
	case ActionMessageSend:
		return grant.Check(ActionMessageSend)
	case ActionMessageMediaDownload:
		return grant.Check(ActionMessageMediaDownload)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// CheckMessageContentUpdate accepts delete and edit metadata updates.
func CheckMessageContentUpdate(grant security.Grant) error {
	switch grant.Action() {
	case ActionMessageDelete:
		return grant.Check(ActionMessageDelete)
	case ActionMessageEdit:
		return grant.Check(ActionMessageEdit)
	default:
		return forbidden(grant)
	}
}

// CheckWebhookLookup accepts webhook configuration and view lookups.
func CheckWebhookLookup(grant security.Grant) error {
	switch grant.Action() {
	case ActionWebhookSet:
		return grant.Check(ActionWebhookSet)
	case ActionWebhookView:
		return grant.Check(ActionWebhookView)
	default:
		return forbidden(grant)
	}
}

// CheckAddressResolution accepts operations that resolve a persisted recipient
// alias as part of their own already-authorized work.
func CheckAddressResolution(grant security.Grant) error {
	switch grant.Action() {
	case ActionMessageSend:
		return grant.Check(ActionMessageSend)
	case ActionProfilePictureView:
		return grant.Check(ActionProfilePictureView)
	default:
		return forbidden(grant)
	}
}

// CheckDeviceStore accepts only operations that may open or create a WhatsApp
// device session after loading the owning instance through its tenant Grant.
func CheckDeviceStore(grant security.Grant) error {
	switch grant.Action() {
	case ActionConnectionPair:
		return grant.Check(ActionConnectionPair)
	case ActionConnectionLogout:
		return grant.Check(ActionConnectionLogout)
	case ActionInstanceDelete:
		return grant.Check(ActionInstanceDelete)
	case ActionRuntime:
		return grant.Check(ActionRuntime)
	default:
		return forbidden(grant)
	}
}

// RuntimeGrantFromPair creates the narrow system authority held by callbacks
// after an authorized pairing flow hands a client to the module runtime.
func RuntimeGrantFromPair(grant security.Grant) (security.Grant, error) {
	if err := grant.Check(ActionConnectionPair); err != nil {
		return security.Grant{}, err
	}
	tenant := data.Tenant(grant)
	if tenant == "" {
		return security.Grant{}, fmt.Errorf("%w: grant has no tenant", security.ErrForbidden)
	}
	//arandu:system-grant pairing callbacks require module-owned lifecycle authority
	return security.SystemGrant(ActionRuntime, tenant), nil
}

func forbidden(grant security.Grant) error {
	return fmt.Errorf("%w: action %q is not valid for this persistence operation", security.ErrForbidden, grant.Action())
}
