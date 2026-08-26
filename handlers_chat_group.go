package whatsapp

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

	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
)

func (m *Module) checkContacts(ctx *fhttp.Context) error {
	var input chat.WhatsAppNumbersRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	items, err := m.service.CheckContacts(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"items": items}))
}

func (m *Module) findMessages(ctx *fhttp.Context) error {
	var input chat.FindMessagesRequest
	if err := m.decodeJSON(ctx, &input, true, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.FindMessages(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

func (m *Module) readMessages(ctx *fhttp.Context) error {
	var input chat.ReadMessagesRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.ReadMessages(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{
		"message": "Read messages", "read": "success",
	}))
}

func (m *Module) archiveChat(ctx *fhttp.Context) error {
	var input chat.ArchiveChatRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.ArchiveChat(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}

func (m *Module) deleteMessage(ctx *fhttp.Context) error {
	raw := strings.TrimSpace(ctx.Param("message"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return m.answer(ctx, chat.ValidationError{Messages: []string{"message must be a positive integer"}})
	}
	if err := m.service.DeleteMessage(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), id); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}

func (m *Module) profilePicture(ctx *fhttp.Context) error {
	var input chat.FetchProfilePictureRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	url, err := m.service.ProfilePicture(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"profilePictureUrl": url}))
}

func (m *Module) rejectCall(ctx *fhttp.Context) error {
	var input chat.RejectCallRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.RejectCall(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}

func (m *Module) editMessage(ctx *fhttp.Context) error {
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
	result, err := m.service.EditMessage(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), chat.EditMessageRequest{
		ID: identifier, Text: body.Text,
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

func messageIdentifier(raw string) (chat.MessageIdentifier, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return chat.MessageIdentifier{}, chat.ValidationError{Messages: []string{"message is required"}}
	}
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil {
		if numeric <= 0 {
			return chat.MessageIdentifier{}, chat.ValidationError{Messages: []string{"message must be a positive integer or non-empty key"}}
		}
		return chat.MessageIdentifier{NumericID: &numeric}, nil
	}
	return chat.MessageIdentifier{KeyID: &value}, nil
}

func (m *Module) downloadMedia(ctx *fhttp.Context) error {
	binary, err := parseOptionalBool(ctx.Query("binary"))
	if err != nil {
		return m.answer(ctx, err)
	}
	var input chat.MediaDataRequest
	if err := m.decodeJSON(ctx, &input, false, true); err != nil {
		return m.answer(ctx, err)
	}
	if _, err := input.Validate(); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.MediaData(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	if binary {
		return writeBinaryMedia(ctx, result)
	}
	return writeMultipartMedia(ctx, result)
}

func writeBinaryMedia(ctx *fhttp.Context, result chat.MediaDownloadResult) error {
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

func writeMultipartMedia(ctx *fhttp.Context, result chat.MediaDownloadResult) error {
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

func (m *Module) createGroup(ctx *fhttp.Context) error {
	var input group.CreateRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.CreateGroup(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusCreated, result)
}

func (m *Module) updateGroupPicture(ctx *fhttp.Context) error {
	var body struct {
		Image string `json:"image"`
	}
	if err := m.decodeJSON(ctx, &body, false, true); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.UpdateGroupPicture(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), group.UpdatePictureRequest{
		Image: body.Image, GroupJID: ctx.Param("group"),
	})
	if err != nil {
		return m.answer(ctx, err)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result)
}

func (m *Module) groupInvite(ctx *fhttp.Context) error {
	result, err := m.service.GroupInviteCode(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), ctx.Param("group"))
	if err != nil {
		return m.answer(ctx, err)
	}
	return ctx.JSON(stdhttp.StatusOK, resources.Make(map[string]any{"invitation": result.Invitation}))
}

func (m *Module) revokeGroupInvite(ctx *fhttp.Context) error {
	if err := m.service.RevokeGroupInvite(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), ctx.Param("group")); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusOK)
}

func (m *Module) updateGroupParticipants(ctx *fhttp.Context) error {
	var input group.UpdateParticipantRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	if err := m.service.UpdateGroupParticipants(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), ctx.Param("group"), input); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusOK)
}

func (m *Module) leaveGroup(ctx *fhttp.Context) error {
	if err := m.service.LeaveGroup(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), ctx.Param("group")); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusOK)
}
