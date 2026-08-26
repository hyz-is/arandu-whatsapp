package controllers

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	stdhttp "net/http"
	"net/textproto"
	"strconv"
	"strings"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/http/resources"

	requests "github.com/hyz-is/arandu-whatsapp/app/Http/Requests"
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
)

// CheckContacts reports which supplied contacts are registered on WhatsApp.
func (m *WhatsAppController) CheckContacts(ctx *fhttp.Context) error {
	var input requests.CheckWhatsAppNumbers
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	items, err := m.service.CheckContacts(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"items": items}))
}

// FindMessages returns stored messages matching a typed query.
func (m *WhatsAppController) FindMessages(ctx *fhttp.Context) error {
	var input requests.FindMessages
	if err := m.decodeJSON(ctx, &input, true, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.FindMessages(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

// ReadMessages marks selected messages as read.
func (m *WhatsAppController) ReadMessages(ctx *fhttp.Context) error {
	var input requests.ReadMessages
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.ReadMessages(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"message": "Read messages", "read": "success",
	}))
}

// ArchiveChat changes a chat's archive state.
func (m *WhatsAppController) ArchiveChat(ctx *fhttp.Context) error {
	var input requests.ArchiveChat
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.ArchiveChat(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}

// DeleteMessage deletes a message for every participant.
func (m *WhatsAppController) DeleteMessage(ctx *fhttp.Context) error {
	raw := strings.TrimSpace(ctx.Param("message"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return m.answer(ctx, models.ValidationError{Messages: []string{"message must be a positive integer"}})
	}
	if err := m.service.DeleteMessage(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), id); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}

// ProfilePicture returns a contact or group profile-picture URL.
func (m *WhatsAppController) ProfilePicture(ctx *fhttp.Context) error {
	var input requests.FetchProfilePicture
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	url, err := m.service.ProfilePicture(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"profilePictureUrl": url}))
}

// RejectCall rejects an incoming WhatsApp call.
func (m *WhatsAppController) RejectCall(ctx *fhttp.Context) error {
	var input requests.RejectCall
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.RejectCall(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}

// EditMessage replaces the text of a sent message.
func (m *WhatsAppController) EditMessage(ctx *fhttp.Context) error {
	var body struct {
		Text string `json:"text"`
	}
	if err := m.decodeJSON(ctx, &body, false, true); err != nil {
		return m.answer(ctx, err)
	}
	identifier, err := messageIdentifier(ctx.Param("message"))
	if err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.EditMessage(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), requests.EditMessage{
		ID: identifier, Text: body.Text,
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

func messageIdentifier(raw string) (requests.MessageIdentifier, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return requests.MessageIdentifier{}, models.ValidationError{Messages: []string{"message is required"}}
	}
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil {
		if numeric <= 0 {
			return requests.MessageIdentifier{}, models.ValidationError{Messages: []string{"message must be a positive integer or non-empty key"}}
		}
		return requests.MessageIdentifier{NumericID: &numeric}, nil
	}
	return requests.MessageIdentifier{KeyID: &value}, nil
}

// DownloadMedia returns media as binary or multipart content.
func (m *WhatsAppController) DownloadMedia(ctx *fhttp.Context) error {
	binary, err := parseOptionalBool(ctx.Query("binary"))
	if err != nil {
		return m.answer(ctx, err)
	}
	var input requests.DownloadMedia
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	if _, err := input.Validate(); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.MediaData(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	if binary {
		return writeBinaryMedia(ctx, result)
	}
	return writeMultipartMedia(ctx, result)
}

func writeBinaryMedia(ctx *fhttp.Context, result models.MediaDownloadResult) error {
	header := ctx.Response.Header()
	header.Set("Content-Type", result.MIMEType)
	header.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": result.FileName}))
	header.Set("Cache-Control", "private, no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Length", strconv.Itoa(len(result.Data)))
	ctx.Response.WriteHeader(stdhttp.StatusOK)
	_, err := ctx.Response.Write(result.Data)
	return err
}

func writeMultipartMedia(ctx *fhttp.Context, result models.MediaDownloadResult) error {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for _, field := range []struct{ name, value string }{
		{"mediaType", result.MediaType},
		{"fileName", result.FileName},
	} {
		if err := writer.WriteField(field.name, field.value); err != nil {
			return err
		}
	}
	size, err := json.Marshal(result.Size)
	if err != nil {
		return err
	}
	if err := writer.WriteField("size", string(size)); err != nil {
		return err
	}
	if err := writer.WriteField("mimetype", result.MIMEType); err != nil {
		return err
	}
	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name": "file", "filename": result.FileName,
	}))
	headers.Set("Content-Type", result.MIMEType)
	part, err := writer.CreatePart(headers)
	if err != nil {
		return err
	}
	if _, err := part.Write(result.Data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	header := ctx.Response.Header()
	header.Set("Content-Type", writer.FormDataContentType())
	header.Set("Cache-Control", "private, no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Length", strconv.Itoa(buffer.Len()))
	ctx.Response.WriteHeader(stdhttp.StatusOK)
	_, err = ctx.Response.Write(buffer.Bytes())
	return err
}

// CreateGroup creates a WhatsApp group.
func (m *WhatsAppController) CreateGroup(ctx *fhttp.Context) error {
	var input requests.CreateGroup
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.CreateGroup(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusCreated, result)
}

// UpdateGroupPicture replaces a group's profile picture.
func (m *WhatsAppController) UpdateGroupPicture(ctx *fhttp.Context) error {
	var body struct {
		Image string `json:"image"`
	}
	if err := m.decodeJSON(ctx, &body, false, true); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.UpdateGroupPicture(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), requests.UpdateGroupPicture{
		Image: body.Image, GroupJID: ctx.Param("group"),
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

// GroupInvite returns the active invitation for a group.
func (m *WhatsAppController) GroupInvite(ctx *fhttp.Context) error {
	result, err := m.service.GroupInviteCode(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), ctx.Param("group"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"invitation": result.Invitation}))
}

// RevokeGroupInvite revokes the active invitation for a group.
func (m *WhatsAppController) RevokeGroupInvite(ctx *fhttp.Context) error {
	if err := m.service.RevokeGroupInvite(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), ctx.Param("group")); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusOK)
}

// UpdateGroupParticipants applies a group membership or role change.
func (m *WhatsAppController) UpdateGroupParticipants(ctx *fhttp.Context) error {
	var input requests.UpdateGroupParticipant
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.UpdateGroupParticipants(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), ctx.Param("group"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusOK)
}

// LeaveGroup removes the connected account from a group.
func (m *WhatsAppController) LeaveGroup(ctx *fhttp.Context) error {
	if err := m.service.LeaveGroup(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), ctx.Param("group")); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusOK)
}
