package controllers

import (
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/http/resources"
	"github.com/arandu-io/hesape/str"

	requests "github.com/hyz-is/arandu-whatsapp/app/Http/Requests"
	resourceshttp "github.com/hyz-is/arandu-whatsapp/app/Http/Resources"
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
)

// CreateInstance validates and creates a tenant-scoped WhatsApp instance.
func (m *WhatsAppController) CreateInstance(ctx *fhttp.Context) error {
	var input requests.CreateInstance
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	instance, err := m.service.CreateInstance(ctx.Ctx(), m.Subject(ctx.Request), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusCreated, resourceshttp.NewInstanceResource(instance))
}

// ListInstances returns one tenant-scoped instance page.
func (m *WhatsAppController) ListInstances(ctx *fhttp.Context) error {
	var name *string
	if ctx.Request.URL.Query().Has("instanceName") {
		value := ctx.Query("instanceName")
		name = &value
	}
	limit := 0
	if ctx.Request.URL.Query().Has("limit") {
		value, parseErr := strconv.Atoi(strings.TrimSpace(ctx.Query("limit")))
		if parseErr != nil || value < 1 || value > models.MaxInstancePageLimit {
			return m.answer(ctx, models.ErrInvalidInput)
		}
		limit = value
	}
	cursor := ctx.Query("cursor")
	if ctx.Request.URL.Query().Has("cursor") && cursor == "" {
		return m.answer(ctx, models.ErrInvalidCursor)
	}
	page, err := m.service.ListInstances(ctx.Ctx(), m.Subject(ctx.Request), models.InstanceListQuery{
		Name: name, Limit: limit, Cursor: cursor,
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resourceshttp.NewInstanceCollection(page))
}

// FindInstance returns one instance by its public name.
func (m *WhatsAppController) FindInstance(ctx *fhttp.Context) error {
	instance, err := m.service.FindInstance(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resourceshttp.NewInstanceResource(instance))
}

// DeleteInstance removes one instance and optionally its owned records.
func (m *WhatsAppController) DeleteInstance(ctx *fhttp.Context) error {
	force, err := parseOptionalBool(ctx.Query("force"))
	if err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.DeleteInstance(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), force)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"instanceName": result.InstanceName,
		"deleted":      result.Deleted,
		"forced":       result.Forced,
		"message":      result.Message,
	}))
}

// ConnectQRCode starts QR-code pairing for an instance.
func (m *WhatsAppController) ConnectQRCode(ctx *fhttp.Context) error {
	result, err := m.service.ConnectQRCode(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"count": result.Count, "code": result.Code, "base64": result.Base64,
		"instanceName": result.InstanceName, "connectionStatus": result.ConnectionStatus,
		"alreadyConnected": result.AlreadyConnected, "alreadyConnecting": result.AlreadyConnecting,
		"ownerJid": result.OwnerJid,
	}))
}

// ConnectPhone starts phone-code pairing for an instance.
func (m *WhatsAppController) ConnectPhone(ctx *fhttp.Context) error {
	var input requests.PairPhone
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.ConnectPhone(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"code": result.Code}))
}

// PasskeyChallenge requests a Passkey pairing challenge.
func (m *WhatsAppController) PasskeyChallenge(ctx *fhttp.Context) error {
	result, err := m.service.PasskeyChallenge(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"requestId": result.RequestID, "state": result.State,
		"expiresAt": result.ExpiresAt.UTC(), "publicKey": result.PublicKey,
	}))
}

// PasskeyAssertion submits an assertion for an active Passkey challenge.
func (m *WhatsAppController) PasskeyAssertion(ctx *fhttp.Context) error {
	var input requests.SubmitPasskeyAssertion
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return m.answer(ctx, fmt.Errorf("%w: requestId is required", errMalformedRequest))
	}
	if !str.IsUUID(input.RequestID) {
		return m.answer(ctx, fmt.Errorf("%w: requestId must be a UUID", errMalformedRequest))
	}
	result, err := m.service.PasskeyAssertion(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusAccepted, resources.Make(map[string]any{
		"state": result.State, "message": result.Message,
	}))
}

// ConnectionState returns an instance's current WhatsApp connection state.
func (m *WhatsAppController) ConnectionState(ctx *fhttp.Context) error {
	result, err := m.service.ConnectionState(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"state": result.State, "statusReason": result.StatusReason,
		"instanceName": result.InstanceName, "connectionStatus": result.ConnectionStatus,
		"connected": result.Connected, "loggedIn": result.LoggedIn, "ownerJid": result.OwnerJid,
	}))
}

// Logout disconnects an instance and removes its active session.
func (m *WhatsAppController) Logout(ctx *fhttp.Context) error {
	result, err := m.service.Logout(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"instanceName": result.InstanceName, "state": result.State,
		"connectionStatus": result.ConnectionStatus, "message": result.Message,
	}))
}

// SetWebhook creates or updates an instance webhook.
func (m *WhatsAppController) SetWebhook(ctx *fhttp.Context) error {
	var input requests.WebhookPayload
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SetWebhook(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), requests.SetWebhook{
		URL: input.URL, Enabled: input.Enabled, Events: input.Events, EventsSet: input.EventsSet,
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

// FindWebhook returns an instance webhook configuration.
func (m *WhatsAppController) FindWebhook(ctx *fhttp.Context) error {
	result, err := m.service.FindWebhook(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}
