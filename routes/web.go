// Package routes declares the canonical named HTTP surface of the module.
package routes

import (
	stdhttp "net/http"

	fhttp "github.com/arandu-io/framework/http"
	swagger "github.com/hyz-is/arandu-swagger"

	controllers "github.com/hyz-is/arandu-whatsapp/app/Http/Controllers"
	documentation "github.com/hyz-is/arandu-whatsapp/app/Http/Documentation"
)

// Deps contains the explicit dependencies of the WhatsApp HTTP routes.
type Deps struct {
	// Prefix is the validated module route prefix.
	Prefix string
	// Controller handles every route registered by this package.
	Controller *controllers.WhatsAppController
}

// Web registers the canonical WhatsApp HTTP surface.
func Web(router *fhttp.Router, deps Deps) {
	web(router, deps, nil)
}

// WebDocumented registers the canonical WhatsApp HTTP surface and documents
// each route through the supplied Arandu Swagger contract.
func WebDocumented(router *fhttp.Router, deps Deps, docs swagger.Documenter) {
	web(router, deps, docs)
}

func web(router *fhttp.Router, deps Deps, docs swagger.Documenter) {
	prefix := deps.Prefix + "/instances"
	register := func(method, path, name string, handler func(*fhttp.Context) error) {
		route := router.Action(method, path, handler).Name("whatsapp." + name)
		if docs != nil {
			documentation.Route(route.GetName(), docs.Route(route))
		}
	}

	register(stdhttp.MethodPost, prefix, "instances.store", deps.Controller.CreateInstance)
	register(stdhttp.MethodGet, prefix, "instances.index", deps.Controller.ListInstances)
	register(stdhttp.MethodGet, prefix+"/{instance}", "instances.show", deps.Controller.FindInstance)
	register(stdhttp.MethodDelete, prefix+"/{instance}", "instances.destroy", deps.Controller.DeleteInstance)

	connection := prefix + "/{instance}/connection"
	register(stdhttp.MethodPost, connection+"/qr", "connection.qr", deps.Controller.ConnectQRCode)
	register(stdhttp.MethodPost, connection+"/phone", "connection.phone", deps.Controller.ConnectPhone)
	register(stdhttp.MethodPost, connection+"/passkey/challenge", "connection.passkey.challenge", deps.Controller.PasskeyChallenge)
	register(stdhttp.MethodPost, connection+"/passkey/assertion", "connection.passkey.assertion", deps.Controller.PasskeyAssertion)
	register(stdhttp.MethodGet, connection, "connection.show", deps.Controller.ConnectionState)
	register(stdhttp.MethodDelete, connection, "connection.destroy", deps.Controller.Logout)

	webhook := prefix + "/{instance}/webhook"
	register(stdhttp.MethodPut, webhook, "webhook.update", deps.Controller.SetWebhook)
	register(stdhttp.MethodGet, webhook, "webhook.show", deps.Controller.FindWebhook)

	messages := prefix + "/{instance}/messages"
	register(stdhttp.MethodPost, messages+"/text", "messages.text", deps.Controller.SendText)
	register(stdhttp.MethodPost, messages+"/link", "messages.link", deps.Controller.SendLink)
	register(stdhttp.MethodPost, messages+"/media", "messages.media", deps.Controller.SendMedia)
	register(stdhttp.MethodPost, messages+"/media/file", "messages.media.file", deps.Controller.SendMediaFile)
	register(stdhttp.MethodPost, messages+"/audio", "messages.audio", deps.Controller.SendAudio)
	register(stdhttp.MethodPost, messages+"/audio/file", "messages.audio.file", deps.Controller.SendAudioFile)
	register(stdhttp.MethodPost, messages+"/contact", "messages.contact", deps.Controller.SendContact)
	register(stdhttp.MethodPost, messages+"/location", "messages.location", deps.Controller.SendLocation)
	register(stdhttp.MethodPost, messages+"/reaction", "messages.reaction", deps.Controller.SendReaction)
	register(stdhttp.MethodPost, messages+"/search", "messages.index", deps.Controller.FindMessages)
	register(stdhttp.MethodPatch, messages+"/read", "messages.read", deps.Controller.ReadMessages)
	register(stdhttp.MethodDelete, messages+"/{message}", "messages.destroy", deps.Controller.DeleteMessage)
	register(stdhttp.MethodPut, messages+"/{message}", "messages.update", deps.Controller.EditMessage)
	register(stdhttp.MethodPost, messages+"/media/download", "messages.media.download", deps.Controller.DownloadMedia)

	register(stdhttp.MethodPost, prefix+"/{instance}/contacts/check", "contacts.check", deps.Controller.CheckContacts)
	register(stdhttp.MethodPost, prefix+"/{instance}/contacts/profile-picture", "contacts.profile-picture", deps.Controller.ProfilePicture)
	register(stdhttp.MethodPut, prefix+"/{instance}/chats/archive", "chats.archive", deps.Controller.ArchiveChat)
	register(stdhttp.MethodPost, prefix+"/{instance}/calls/reject", "calls.reject", deps.Controller.RejectCall)

	groups := prefix + "/{instance}/groups"
	register(stdhttp.MethodPost, groups, "groups.store", deps.Controller.CreateGroup)
	register(stdhttp.MethodPut, groups+"/{group}/picture", "groups.picture.update", deps.Controller.UpdateGroupPicture)
	register(stdhttp.MethodGet, groups+"/{group}/invite", "groups.invite.show", deps.Controller.GroupInvite)
	register(stdhttp.MethodDelete, groups+"/{group}/invite", "groups.invite.destroy", deps.Controller.RevokeGroupInvite)
	register(stdhttp.MethodPatch, groups+"/{group}/participants", "groups.participants.update", deps.Controller.UpdateGroupParticipants)
	register(stdhttp.MethodDelete, groups+"/{group}", "groups.leave", deps.Controller.LeaveGroup)
}
