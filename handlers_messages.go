package whatsapp

import (
	"errors"
	"fmt"
	"mime/multipart"
	stdhttp "net/http"

	fhttp "github.com/arandu-io/framework/http"

	"github.com/hyz-is/arandu-whatsapp/internal/message"
)

func (m *Module) sendText(ctx *fhttp.Context) error {
	var input message.SendTextRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendText(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendLink(ctx *fhttp.Context) error {
	var input message.SendLinkRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendLink(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendMedia(ctx *fhttp.Context) error {
	var input message.SendMediaRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendMedia(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendAudio(ctx *fhttp.Context) error {
	var input message.SendWhatsAppAudioRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendAudio(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendContact(ctx *fhttp.Context) error {
	var input message.SendContactRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendContact(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendLocation(ctx *fhttp.Context) error {
	var input message.SendLocationRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendLocation(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendReaction(ctx *fhttp.Context) error {
	var input message.SendReactionRequest
	if err := m.decodeJSON(ctx, &input, false, false); err != nil {
		return m.answer(ctx, err)
	}
	result, err := m.service.SendReaction(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), input)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendMediaFile(ctx *fhttp.Context) error {
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
	result, err := m.service.SendMediaFile(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"),
		ctx.Request.FormValue("number"), file, header, ctx.Request.FormValue("mediaType"),
		optionalString(ctx.Request.FormValue("caption")), options)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) sendAudioFile(ctx *fhttp.Context) error {
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
	result, err := m.service.SendAudioFile(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"),
		ctx.Request.FormValue("number"), file, header, options)
	return m.answerMessageResult(ctx, result, err)
}

func (m *Module) multipartAttachment(ctx *fhttp.Context) (*multipart.FileHeader, multipart.File, error) {
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

func (m *Module) answerMessageResult(ctx *fhttp.Context, result message.SendResult, err error) error {
	if err != nil {
		return m.answer(ctx, err)
	}
	if result.Accepted != nil {
		return answerStruct(ctx, stdhttp.StatusAccepted, result.Accepted)
	}
	return answerStruct(ctx, stdhttp.StatusOK, result.Message)
}
