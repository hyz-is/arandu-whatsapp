package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/http/resources"

	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	services "github.com/hyz-is/arandu-whatsapp/app/Services"
	config "github.com/hyz-is/arandu-whatsapp/config"

	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

var errMalformedRequest = errors.New("malformed request")

// WhatsAppController adapts HTTP requests to the authorized application service.
type WhatsAppController struct {
	cfg      config.Config
	service  *services.Service
	sessions *security.SessionStore
}

// NewWhatsAppController returns the native HTTP controller.
func NewWhatsAppController(cfg config.Config, service *services.Service, sessions *security.SessionStore) *WhatsAppController {
	return &WhatsAppController{cfg: cfg, service: service, sessions: sessions}
}

// Subject obtains identity exclusively from the Arandu SessionStore.
func (m *WhatsAppController) Subject(r *stdhttp.Request) security.Subject {
	subject, err := m.sessions.Load(r.Context(), r)
	if err != nil || subject.ID == "" {
		return security.Guest(m.cfg.Tenant)
	}
	return subject
}

func structFields(value any) (map[string]any, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode response payload: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("decode response payload: %w", err)
	}
	if fields == nil {
		return nil, errors.New("response payload must be a JSON object")
	}
	return fields, nil
}

func (m *WhatsAppController) decodeJSON(ctx *fhttp.Context, dst any, allowEmpty, strict bool) error {
	limit := m.cfg.Media.MaxInputBytes*2 + 1024*1024
	if limit < 1024*1024 {
		limit = 1024 * 1024
	}
	ctx.Request.Body = stdhttp.MaxBytesReader(ctx.Response, ctx.Request.Body, limit)
	decoder := json.NewDecoder(ctx.Request.Body)
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return nil
		}
		var tooLarge *stdhttp.MaxBytesError
		if errors.As(err, &tooLarge) {
			return message.ErrPayloadTooLarge
		}
		return fmt.Errorf("%w: %v", errMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: body must contain one JSON value", errMalformedRequest)
		}
		return fmt.Errorf("%w: %v", errMalformedRequest, err)
	}
	return nil
}

func (m *WhatsAppController) answer(ctx *fhttp.Context, err error) error {
	status, ok := statusForError(err)
	if !ok {
		return err
	}
	fields := map[string]any{
		"statusCode": status,
		"error":      errorKind(status),
		"messages":   messagesForError(err, status),
	}
	if code := publicErrorCode(err); code != "" {
		fields["code"] = code
	}
	return ctx.JSON(status, resources.Make(fields))
}

func answerStruct(ctx *fhttp.Context, status int, value any) error {
	fields, err := structFields(value)
	if err != nil {
		return err
	}
	return ctx.JSON(status, resources.Make(fields))
}

func statusForError(err error) (int, bool) {
	var validation chat.ValidationError
	var dependencies *models.InstanceDependenciesError
	switch {
	case err == nil:
		return 0, false
	case errors.Is(err, security.ErrForbidden),
		errors.Is(err, internalwhatsapp.ErrInstanceInactive),
		errors.Is(err, chat.ErrMessageNotOutgoing):
		return stdhttp.StatusForbidden, true
	case errors.Is(err, errMalformedRequest),
		errors.Is(err, models.ErrInvalidInput),
		errors.Is(err, models.ErrInvalidJSON),
		errors.Is(err, models.ErrInvalidWebhookEvent),
		errors.Is(err, models.ErrInvalidWebhookURL),
		errors.Is(err, models.ErrInvalidCursor),
		errors.Is(err, internalwhatsapp.ErrInvalidPhoneNumber),
		errors.Is(err, message.ErrInvalidRequest),
		errors.Is(err, message.ErrMentionAllRequiresGroup),
		errors.Is(err, message.ErrMentionAllUnsupported),
		errors.Is(err, message.ErrRecipientInvalid),
		errors.Is(err, message.ErrPresenceInvalid),
		errors.Is(err, message.ErrDelayInvalid),
		errors.Is(err, message.ErrQuotedMessageInvalid),
		errors.Is(err, message.ErrInvalidAudioDuration),
		errors.Is(err, chat.ErrInvalidRecipient),
		errors.Is(err, chat.ErrInvalidRequestMode),
		errors.Is(err, chat.ErrInvalidMediaRequest),
		errors.Is(err, chat.ErrUnsupportedMediaType),
		errors.Is(err, chat.ErrInvalidMediaContent),
		errors.Is(err, chat.ErrMessageIsNotMedia),
		errors.Is(err, group.ErrInvalidGroupJID),
		errors.Is(err, group.ErrInvalidParticipant),
		errors.Is(err, group.ErrInvalidRequest),
		errors.Is(err, address.ErrInvalidAddress):
		return stdhttp.StatusBadRequest, true
	case errors.As(err, &validation), errors.Is(err, chat.ErrMessageNotEditable):
		return stdhttp.StatusUnprocessableEntity, true
	case errors.Is(err, models.ErrInstanceNotFound),
		errors.Is(err, models.ErrWebhookNotFound),
		errors.Is(err, models.ErrMessageNotFound),
		errors.Is(err, internalwhatsapp.ErrPasskeyInstanceNotFound),
		errors.Is(err, internalwhatsapp.ErrPairingSessionNotFound),
		errors.Is(err, chat.ErrMediaMessageNotFound),
		errors.Is(err, address.ErrRecipientNotOnWhatsApp):
		return stdhttp.StatusNotFound, true
	case errors.Is(err, models.ErrInstanceNameAlreadyExists),
		errors.Is(err, models.ErrWhatsAppDeviceAlreadyLinked),
		errors.Is(err, models.ErrWebhookAlreadyExists),
		errors.Is(err, internalwhatsapp.ErrConnectionInProgress),
		errors.Is(err, internalwhatsapp.ErrInstanceConnected),
		errors.Is(err, internalwhatsapp.ErrPairingSessionNotActive),
		errors.Is(err, internalwhatsapp.ErrInvalidPairingState),
		errors.Is(err, internalwhatsapp.ErrPasskeyRequestMismatch),
		errors.Is(err, internalwhatsapp.ErrPasskeyChallengeAlreadyUsed),
		errors.Is(err, address.ErrAmbiguousRecipient),
		errors.As(err, &dependencies):
		return stdhttp.StatusConflict, true
	case errors.Is(err, internalwhatsapp.ErrPasskeyChallengeExpired):
		return stdhttp.StatusGone, true
	case errors.Is(err, internalwhatsapp.ErrInvalidPasskeyAssertion),
		errors.Is(err, internalwhatsapp.ErrPasskeyNotAvailable):
		return stdhttp.StatusUnprocessableEntity, true
	case errors.Is(err, internalwhatsapp.ErrQRCodeTimeout):
		return stdhttp.StatusRequestTimeout, true
	case errors.Is(err, message.ErrPayloadTooLarge),
		errors.Is(err, chat.ErrMediaTooLarge),
		errors.Is(err, group.ErrImageTooLarge):
		return stdhttp.StatusRequestEntityTooLarge, true
	case errors.Is(err, message.ErrUnsupportedMediaType):
		return stdhttp.StatusUnsupportedMediaType, true
	case errors.Is(err, message.ErrPersistenceFailed),
		errors.Is(err, message.ErrQuotedMessageLookup),
		errors.Is(err, chat.ErrDatabaseOperation):
		return stdhttp.StatusNotAcceptable, true
	case errors.Is(err, internalwhatsapp.ErrQRChannelClosed),
		errors.Is(err, internalwhatsapp.ErrPairingFailed),
		errors.Is(err, internalwhatsapp.ErrClientOutdated),
		errors.Is(err, internalwhatsapp.ErrWhatsAppUnavailable),
		errors.Is(err, internalwhatsapp.ErrClientNotConnected),
		errors.Is(err, internalwhatsapp.ErrPasskeyServiceUnavailable),
		errors.Is(err, internalwhatsapp.ErrSessionMissing),
		errors.Is(err, internalwhatsapp.ErrDeviceMismatch),
		errors.Is(err, chat.ErrInstanceDisconnected),
		errors.Is(err, chat.ErrRemoteOperation),
		errors.Is(err, chat.ErrMediaDownloadFailed),
		errors.Is(err, group.ErrInstanceDisconnected),
		errors.Is(err, group.ErrRemoteOperation),
		errors.Is(err, group.ErrDownloadFailed):
		return stdhttp.StatusServiceUnavailable, true
	case errors.Is(err, message.ErrDownloadFailed),
		errors.Is(err, message.ErrUploadFailed),
		errors.Is(err, message.ErrSendFailed),
		errors.Is(err, message.ErrAudioProcessing):
		return stdhttp.StatusInternalServerError, true
	default:
		return 0, false
	}
}

func messagesForError(err error, status int) []string {
	if status == stdhttp.StatusForbidden && errors.Is(err, security.ErrForbidden) {
		return []string{stdhttp.StatusText(status)}
	}
	var validation chat.ValidationError
	if errors.As(err, &validation) && len(validation.Messages) > 0 {
		return validation.Messages
	}
	var dependencies *models.InstanceDependenciesError
	if errors.As(err, &dependencies) {
		return []string{dependencies.Error()}
	}
	if status >= stdhttp.StatusInternalServerError {
		return []string{stdhttp.StatusText(status)}
	}
	return []string{err.Error()}
}

func publicErrorCode(err error) string {
	switch {
	case errors.Is(err, message.ErrMentionAllRequiresGroup):
		return "MENTION_ALL_REQUIRES_GROUP"
	case errors.Is(err, message.ErrMentionAllUnsupported):
		return "MENTION_ALL_NOT_SUPPORTED_FOR_MESSAGE_TYPE"
	case errors.Is(err, internalwhatsapp.ErrPasskeyInstanceNotFound):
		return "INSTANCE_NOT_FOUND"
	case errors.Is(err, internalwhatsapp.ErrPairingSessionNotFound):
		return "PAIRING_SESSION_NOT_FOUND"
	case errors.Is(err, internalwhatsapp.ErrPairingSessionNotActive):
		return "PAIRING_SESSION_NOT_ACTIVE"
	case errors.Is(err, internalwhatsapp.ErrInvalidPairingState):
		return "INVALID_PAIRING_STATE"
	case errors.Is(err, internalwhatsapp.ErrPasskeyRequestMismatch):
		return "PASSKEY_REQUEST_MISMATCH"
	case errors.Is(err, internalwhatsapp.ErrPasskeyChallengeAlreadyUsed):
		return "PASSKEY_CHALLENGE_ALREADY_USED"
	case errors.Is(err, internalwhatsapp.ErrInstanceConnected):
		return "INSTANCE_ALREADY_CONNECTED"
	case errors.Is(err, internalwhatsapp.ErrPasskeyChallengeExpired):
		return "PASSKEY_CHALLENGE_EXPIRED"
	case errors.Is(err, internalwhatsapp.ErrInvalidPasskeyAssertion):
		return "INVALID_PASSKEY_ASSERTION"
	case errors.Is(err, internalwhatsapp.ErrPasskeyNotAvailable):
		return "PASSKEY_NOT_AVAILABLE"
	case errors.Is(err, internalwhatsapp.ErrClientNotConnected):
		return "WHATSAPP_CLIENT_NOT_CONNECTED"
	case errors.Is(err, internalwhatsapp.ErrPasskeyServiceUnavailable):
		return "PASSKEY_SERVICE_UNAVAILABLE"
	default:
		return ""
	}
}

func errorKind(status int) string {
	switch status {
	case stdhttp.StatusRequestEntityTooLarge:
		return "payload-too-large"
	case stdhttp.StatusUnsupportedMediaType:
		return "unsupported-media-type"
	case stdhttp.StatusInternalServerError:
		return "internal-server-error"
	default:
		return strings.ReplaceAll(strings.ToLower(stdhttp.StatusText(status)), " ", "-")
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func parseOptionalBool(value string) (bool, error) {
	switch strings.TrimSpace(value) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf("%w: expected true or false", errMalformedRequest)
	}
}
