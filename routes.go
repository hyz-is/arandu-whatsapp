package whatsapp

import (
	stdhttp "net/http"

	fhttp "github.com/arandu-io/framework/http"
)

func (m *Module) registerRoutes(router *fhttp.Router) {
	prefix := m.cfg.Prefix + "/instances"
	register := func(method, path, name string, handler func(*fhttp.Context) error) {
		router.Action(method, path, handler).Name("whatsapp." + name)
	}

	register(stdhttp.MethodPost, prefix, "instances.store", m.createInstance)
	register(stdhttp.MethodGet, prefix, "instances.index", m.listInstances)
	register(stdhttp.MethodGet, prefix+"/{instance}", "instances.show", m.findInstance)
	register(stdhttp.MethodDelete, prefix+"/{instance}", "instances.destroy", m.deleteInstance)

	connection := prefix + "/{instance}/connection"
	register(stdhttp.MethodPost, connection+"/qr", "connection.qr", m.connectQRCode)
	register(stdhttp.MethodPost, connection+"/phone", "connection.phone", m.connectPhone)
	register(stdhttp.MethodPost, connection+"/passkey/challenge", "connection.passkey.challenge", m.passkeyChallenge)
	register(stdhttp.MethodPost, connection+"/passkey/assertion", "connection.passkey.assertion", m.passkeyAssertion)
	register(stdhttp.MethodGet, connection, "connection.show", m.connectionState)
	register(stdhttp.MethodDelete, connection, "connection.destroy", m.logout)

	webhook := prefix + "/{instance}/webhook"
	register(stdhttp.MethodPut, webhook, "webhook.update", m.setWebhook)
	register(stdhttp.MethodGet, webhook, "webhook.show", m.findWebhook)

	messages := prefix + "/{instance}/messages"
	register(stdhttp.MethodPost, messages+"/text", "messages.text", m.sendText)
	register(stdhttp.MethodPost, messages+"/link", "messages.link", m.sendLink)
	register(stdhttp.MethodPost, messages+"/media", "messages.media", m.sendMedia)
	register(stdhttp.MethodPost, messages+"/media/file", "messages.media.file", m.sendMediaFile)
	register(stdhttp.MethodPost, messages+"/audio", "messages.audio", m.sendAudio)
	register(stdhttp.MethodPost, messages+"/audio/file", "messages.audio.file", m.sendAudioFile)
	register(stdhttp.MethodPost, messages+"/contact", "messages.contact", m.sendContact)
	register(stdhttp.MethodPost, messages+"/location", "messages.location", m.sendLocation)
	register(stdhttp.MethodPost, messages+"/reaction", "messages.reaction", m.sendReaction)
	register(stdhttp.MethodPost, messages+"/search", "messages.index", m.findMessages)
	register(stdhttp.MethodPatch, messages+"/read", "messages.read", m.readMessages)
	register(stdhttp.MethodDelete, messages+"/{message}", "messages.destroy", m.deleteMessage)
	register(stdhttp.MethodPut, messages+"/{message}", "messages.update", m.editMessage)
	register(stdhttp.MethodPost, messages+"/media/download", "messages.media.download", m.downloadMedia)

	register(stdhttp.MethodPost, prefix+"/{instance}/contacts/check", "contacts.check", m.checkContacts)
	register(stdhttp.MethodPost, prefix+"/{instance}/contacts/profile-picture", "contacts.profile-picture", m.profilePicture)
	register(stdhttp.MethodPut, prefix+"/{instance}/chats/archive", "chats.archive", m.archiveChat)
	register(stdhttp.MethodPost, prefix+"/{instance}/calls/reject", "calls.reject", m.rejectCall)

	groups := prefix + "/{instance}/groups"
	register(stdhttp.MethodPost, groups, "groups.store", m.createGroup)
	register(stdhttp.MethodPut, groups+"/{group}/picture", "groups.picture.update", m.updateGroupPicture)
	register(stdhttp.MethodGet, groups+"/{group}/invite", "groups.invite.show", m.groupInvite)
	register(stdhttp.MethodDelete, groups+"/{group}/invite", "groups.invite.destroy", m.revokeGroupInvite)
	register(stdhttp.MethodPatch, groups+"/{group}/participants", "groups.participants.update", m.updateGroupParticipants)
	register(stdhttp.MethodDelete, groups+"/{group}", "groups.leave", m.leaveGroup)
}
