package whatsapp

import (
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/http/resources"
	"github.com/arandu-io/hesape/str"

	httprequest "github.com/hyz-is/arandu-whatsapp/internal/http/request"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

func (m *Module) createInstance(ctx *fhttp.Context) error {
	var input CreateInstanceInput
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	instance, err := m.service.CreateInstance(ctx.Ctx(), m.subject(ctx.Request), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusCreated, NewInstanceResource(instance))
}

func (m *Module) listInstances(ctx *fhttp.Context) error {
	var name *string
	if ctx.Request.URL.Query().Has("instanceName") {
		value := ctx.Query("instanceName")
		name = &value
	}
	limit := 0
	if ctx.Request.URL.Query().Has("limit") {
		value, parseErr := strconv.Atoi(strings.TrimSpace(ctx.Query("limit")))
		if parseErr != nil || value < 1 || value > MaxInstancePageLimit {
			return m.answer(ctx, ErrInvalidInput)
		}
		limit = value
	}
	cursor := ctx.Query("cursor")
	if ctx.Request.URL.Query().Has("cursor") && cursor == "" {
		return m.answer(ctx, ErrInvalidCursor)
	}
	page, err := m.service.ListInstances(ctx.Ctx(), m.subject(ctx.Request), InstanceListQuery{
		Name: name, Limit: limit, Cursor: cursor,
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, NewInstanceCollection(page))
}

func (m *Module) findInstance(ctx *fhttp.Context) error {
	instance, err := m.service.FindInstance(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, NewInstanceResource(instance))
}

func (m *Module) deleteInstance(ctx *fhttp.Context) error {
	force, err := parseOptionalBool(ctx.Query("force"))
	if err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.DeleteInstance(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), force)
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

func (m *Module) connectQRCode(ctx *fhttp.Context) error {
	result, err := m.service.ConnectQRCode(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"))
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

func (m *Module) connectPhone(ctx *fhttp.Context) error {
	var input PhonePairingInput
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.ConnectPhone(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"code": result.Code}))
}

func (m *Module) passkeyChallenge(ctx *fhttp.Context) error {
	result, err := m.service.PasskeyChallenge(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"requestId": result.RequestID, "state": result.State,
		"expiresAt": result.ExpiresAt.UTC(), "publicKey": result.PublicKey,
	}))
}

func (m *Module) passkeyAssertion(ctx *fhttp.Context) error {
	var input internalwhatsapp.SubmitPasskeyAssertionRequest
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return m.answer(ctx, fmt.Errorf("%w: requestId is required", errMalformedRequest))
	}
	if !str.IsUUID(input.RequestID) {
		return m.answer(ctx, fmt.Errorf("%w: requestId must be a UUID", errMalformedRequest))
	}
	result, err := m.service.PasskeyAssertion(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusAccepted, resources.Make(map[string]any{
		"state": result.State, "message": result.Message,
	}))
}

func (m *Module) connectionState(ctx *fhttp.Context) error {
	result, err := m.service.ConnectionState(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"state": result.State, "statusReason": result.StatusReason,
		"instanceName": result.InstanceName, "connectionStatus": result.ConnectionStatus,
		"connected": result.Connected, "loggedIn": result.LoggedIn, "ownerJid": result.OwnerJid,
	}))
}

func (m *Module) logout(ctx *fhttp.Context) error {
	result, err := m.service.Logout(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"instanceName": result.InstanceName, "state": result.State,
		"connectionStatus": result.ConnectionStatus, "message": result.Message,
	}))
}

func (m *Module) setWebhook(ctx *fhttp.Context) error {
	var input httprequest.SetWebhookRequest
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SetWebhook(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), webhooksvc.SetInput{
		URL: input.URL, Enabled: input.Enabled, Events: input.Events, EventsSet: input.EventsSet,
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

func (m *Module) findWebhook(ctx *fhttp.Context) error {
	result, err := m.service.FindWebhook(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}
