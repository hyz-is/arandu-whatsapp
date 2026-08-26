package controllers

import (
	"errors"
	"fmt"
	"mime/multipart"
	stdhttp "net/http"

	fhttp "github.com/arandu-io/framework/http"

	requests "github.com/hyz-is/arandu-whatsapp/app/Http/Requests"
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
)

// SendText sends a text message through an instance.
func (m *WhatsAppController) SendText(ctx *fhttp.Context) error {
	var input requests.SendText
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendText(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendLink sends a link message through an instance.
func (m *WhatsAppController) SendLink(ctx *fhttp.Context) error {
	var input requests.SendLink
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendLink(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendMedia sends structured media through an instance.
func (m *WhatsAppController) SendMedia(ctx *fhttp.Context) error {
	var input requests.SendMedia
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendMedia(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendAudio sends structured audio through an instance.
func (m *WhatsAppController) SendAudio(ctx *fhttp.Context) error {
	var input requests.SendWhatsAppAudio
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendAudio(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendContact sends a contact card through an instance.
func (m *WhatsAppController) SendContact(ctx *fhttp.Context) error {
	var input requests.SendContact
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendContact(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendLocation sends geographic coordinates through an instance.
func (m *WhatsAppController) SendLocation(ctx *fhttp.Context) error {
	var input requests.SendLocation
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendLocation(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendReaction sends a reaction to an existing message.
func (m *WhatsAppController) SendReaction(ctx *fhttp.Context) error {
	var input requests.SendReaction
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendReaction(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

// SendMediaFile sends a multipart media upload through an instance.
func (m *WhatsAppController) SendMediaFile(ctx *fhttp.Context) error {
	header, file, err := m.multipartAttachment(ctx)
	if err != nil {
		return m.answer(ctx, err)
	}
	defer removeMultipartFiles(ctx.Request)
	defer file.Close()
	options, err := message.ParseMultipartMessageOptions(
		ctx.Request.FormValue("delay"), ctx.Request.FormValue("presence"),
		ctx.Request.FormValue("quotedMessageId"), ctx.Request.FormValue("quotedMessage"),
		ctx.Request.FormValue("mentionAll"),
	)
	if err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendMediaFile(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"),
		ctx.Request.FormValue("number"), file, header, ctx.Request.FormValue("mediaType"),
		optionalString(ctx.Request.FormValue("caption")), options)
	return m.answerMessageResult(ctx, result, err)
}

// SendAudioFile sends a multipart audio upload through an instance.
func (m *WhatsAppController) SendAudioFile(ctx *fhttp.Context) error {
	header, file, err := m.multipartAttachment(ctx)
	if err != nil {
		return m.answer(ctx, err)
	}
	defer removeMultipartFiles(ctx.Request)
	defer file.Close()
	options, err := message.ParseMultipartAudioOptions(
		ctx.Request.FormValue("delay"), ctx.Request.FormValue("presence"),
		ctx.Request.FormValue("quotedMessageId"), ctx.Request.FormValue("quotedMessage"),
		ctx.Request.FormValue("mentionAll"),
	)
	if err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendAudioFile(ctx.Ctx(), m.Subject(ctx.Request), ctx.Param("instance"),
		ctx.Request.FormValue("number"), file, header, options)
	return m.answerMessageResult(ctx, result, err)
}

func (m *WhatsAppController) multipartAttachment(ctx *fhttp.Context) (*multipart.FileHeader, multipart.File, error) {
	limit := m.cfg.Media.MaxInputBytes + 1024*1024
	ctx.Request.Body = stdhttp.MaxBytesReader(ctx.Response, ctx.Request.Body, limit)
	if err := ctx.Request.ParseMultipartForm(32 << 20); err != nil {
		removeMultipartFiles(ctx.Request)
		var tooLarge *stdhttp.MaxBytesError
		if errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
			return nil, nil, message.ErrPayloadTooLarge
		}
		return nil, nil, fmt.Errorf("%w: invalid multipart body", message.ErrInvalidRequest)
	}
	header, err := firstFileHeader(ctx.Request.MultipartForm, "attachment")
	if err != nil {
		removeMultipartFiles(ctx.Request)
		return nil, nil, err
	}
	if header.Size > m.cfg.Media.MaxInputBytes {
		removeMultipartFiles(ctx.Request)
		return nil, nil, message.ErrPayloadTooLarge
	}
	file, err := header.Open()
	if err != nil {
		removeMultipartFiles(ctx.Request)
		return nil, nil, fmt.Errorf("%w: open attachment", message.ErrInvalidRequest)
	}
	return header, file, nil
}

func firstFileHeader(form *multipart.Form, field string) (*multipart.FileHeader, error) {
	if form == nil || len(form.File[field]) == 0 || form.File[field][0] == nil {
		return nil, fmt.Errorf("%w: attachment is required", message.ErrInvalidRequest)
	}
	return form.File[field][0], nil
}

func removeMultipartFiles(request *stdhttp.Request) {
	if request.MultipartForm != nil {
		_ = request.MultipartForm.RemoveAll()
	}
}

func (m *WhatsAppController) answerMessageResult(ctx *fhttp.Context, result models.SendResult, err error) error {
	if err != nil {
		return m.answer(ctx, err)
	}
	if result.Accepted != nil {
		return answerStruct(ctx, stdhttp.StatusAccepted, result.Accepted)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result.Message)
}
