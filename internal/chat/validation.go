package chat

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/arandu-io/hesape/validation"
	watypes "go.mau.fi/whatsmeow/types"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

type ValidationError struct {
	Messages []string
}

func (e ValidationError) Error() string {
	return strings.Join(e.Messages, "; ")
}

var (
	whatsAppNumbersRules = validation.MustCompile(validation.Rules{
		"numbers":   "required|list|min:1",
		"numbers.*": "required|string",
	}, validation.WithMessageOverrides(validation.Messages{
		"numbers.min": "must contain at least one item",
	}))
	readMessagesRules = validation.MustCompile(validation.Rules{
		"ids":          "sometimes|list",
		"messageIds":   "sometimes|list",
		"messageIds.*": "required|string",
	})
	archiveChatRules = validation.MustCompile(validation.Rules{
		"lastMessage.key.remoteJid": "required|string",
		"lastMessage.key.fromMe":    "required|boolean",
		"lastMessage.key.id":        "required|string",
		"archive":                   "required|boolean",
	})
	rejectCallRules = validation.MustCompile(validation.Rules{
		"callId":   "required|string",
		"callFrom": "required|string",
	})
	editMessageRules = validation.MustCompile(validation.Rules{
		"id":   "required|string",
		"text": "required|string|min:1|max:65536",
	}, validation.WithMessageOverrides(validation.Messages{
		"text.max": "must be less than 65536",
	}))
)

func validateData(data validation.Data, rules *validation.Set) error {
	validator := validation.Make(data, rules)
	if validator.Passes() {
		return nil
	}
	errors := validator.Errors()
	messages := make([]string, 0, errors.Count())
	for _, field := range errors.Keys() {
		for _, message := range errors.Get(field) {
			messages = append(messages, displayValidationField(field)+" "+message)
		}
	}
	return ValidationError{Messages: messages}
}

func displayValidationField(field string) string {
	parts := strings.Split(field, ".")
	var output strings.Builder
	for index, part := range parts {
		if _, err := strconv.Atoi(part); err == nil {
			output.WriteString("[")
			output.WriteString(part)
			output.WriteString("]")
			continue
		}
		if index > 0 {
			output.WriteString(".")
		}
		output.WriteString(part)
	}
	return output.String()
}

func stringList(values []string) []any {
	items := make([]any, len(values))
	for index, value := range values {
		items[index] = value
	}
	return items
}

func validateWhatsAppNumbers(input WhatsAppNumbersRequest, limit int) error {
	data := validation.Data{}
	if input.Numbers != nil {
		data["numbers"] = stringList(input.Numbers)
	}
	if err := validateData(data, whatsAppNumbersRules); err != nil {
		return err
	}
	if limit <= 0 {
		limit = DefaultWhatsAppNumbersLimit
	}
	if len(input.Numbers) > limit {
		return ValidationError{Messages: []string{fmt.Sprintf("numbers must contain at most %d items", limit)}}
	}
	for i, number := range input.Numbers {
		if strings.TrimSpace(number) == "" {
			return ValidationError{Messages: []string{fmt.Sprintf("numbers[%d] is required", i)}}
		}
	}
	return nil
}

func validateReadMessages(input ReadMessagesRequest) error {
	data := validation.Data{}
	if input.IDs != nil {
		ids := make([]any, len(input.IDs))
		for index, id := range input.IDs {
			ids[index] = id
		}
		data["ids"] = ids
	}
	if input.MessageIDs != nil {
		data["messageIds"] = stringList(input.MessageIDs)
	}
	if err := validateData(data, readMessagesRules); err != nil {
		return err
	}
	hasIDs := len(input.IDs) > 0
	hasDirect := input.Sender != nil || input.Chat != nil || len(input.MessageIDs) > 0
	if hasIDs == hasDirect {
		return ValidationError{Messages: []string{"exactly one read mode is required"}}
	}
	if hasIDs {
		for _, id := range input.IDs {
			if id <= 0 {
				return ValidationError{Messages: []string{"ids must contain positive integers"}}
			}
		}
		return nil
	}
	if input.Sender == nil || strings.TrimSpace(*input.Sender) == "" {
		return ValidationError{Messages: []string{"sender is required"}}
	}
	if input.Chat == nil || strings.TrimSpace(*input.Chat) == "" {
		return ValidationError{Messages: []string{"chat is required"}}
	}
	if len(input.MessageIDs) == 0 {
		return ValidationError{Messages: []string{"messageIds must contain at least one item"}}
	}
	for _, id := range input.MessageIDs {
		if strings.TrimSpace(id) == "" {
			return ValidationError{Messages: []string{"messageIds must not contain empty items"}}
		}
	}
	return nil
}

func validateFindMessages(input *FindMessagesRequest) error {
	if input == nil {
		return ValidationError{Messages: []string{"body is required"}}
	}
	if input.Offset == 0 {
		input.Offset = DefaultFindMessagesLimit
	}
	if input.Page == 0 {
		input.Page = 1
	}
	if input.Offset < 0 || input.Offset > MaxFindMessagesLimit {
		return ValidationError{Messages: []string{fmt.Sprintf("offset must be between 1 and %d", MaxFindMessagesLimit)}}
	}
	if input.Page < 0 {
		return ValidationError{Messages: []string{"page must be greater than 0"}}
	}
	if input.Where.ID != nil && *input.Where.ID <= 0 {
		return ValidationError{Messages: []string{"where.id must be greater than 0"}}
	}
	if input.Where.Device != nil {
		value := strings.TrimSpace(*input.Where.Device)
		if device := dbtypes.DeviceMessage(value); value != "" && !device.IsValid() {
			return ValidationError{Messages: []string{"where.device is invalid"}}
		}
	}
	if input.Where.MessageTimestampGTE != nil && *input.Where.MessageTimestampGTE < 0 {
		return ValidationError{Messages: []string{"where.messageTimestampGte must be greater than 0"}}
	}
	if input.Where.MessageTimestampLTE != nil && *input.Where.MessageTimestampLTE < 0 {
		return ValidationError{Messages: []string{"where.messageTimestampLte must be greater than 0"}}
	}
	if input.Where.MessageTimestampGTE != nil && input.Where.MessageTimestampLTE != nil &&
		*input.Where.MessageTimestampGTE > *input.Where.MessageTimestampLTE {
		return ValidationError{Messages: []string{"where.messageTimestampGte must be less than where.messageTimestampLte"}}
	}
	return nil
}

func validateArchiveChat(input ArchiveChatRequest) error {
	key := validation.Data{
		"remoteJid": input.LastMessage.Key.RemoteJID,
		"id":        input.LastMessage.Key.ID,
	}
	if input.LastMessage.Key.FromMe != nil {
		key["fromMe"] = *input.LastMessage.Key.FromMe
	}
	data := validation.Data{"lastMessage": validation.Data{"key": key}}
	if input.Archive != nil {
		data["archive"] = *input.Archive
	}
	if err := validateData(data, archiveChatRules); err != nil {
		return err
	}
	if _, err := watypes.ParseJID(strings.TrimSpace(input.LastMessage.Key.RemoteJID)); err != nil {
		return ValidationError{Messages: []string{"lastMessage.key.remoteJid must be a valid WhatsApp JID"}}
	}
	return nil
}

func validateFetchProfilePicture(input FetchProfilePictureRequest) error {
	if _, err := input.ResolveRecipient(); err != nil {
		return ValidationError{Messages: []string{"exactly one of number, chat or recipient is required"}}
	}
	return nil
}

func validateRejectCall(input RejectCallRequest) error {
	if err := validateData(validation.Data{
		"callId": input.CallID, "callFrom": input.CallFrom,
	}, rejectCallRules); err != nil {
		return err
	}
	if _, err := watypes.ParseJID(strings.TrimSpace(input.CallFrom)); err != nil {
		return ValidationError{Messages: []string{"callFrom must be a valid WhatsApp JID"}}
	}
	return nil
}

func validateEditMessage(input EditMessageRequest) error {
	if err := validateData(validation.Data{
		"id": input.ID.String(), "text": input.Text,
	}, editMessageRules); err != nil {
		return err
	}
	if !utf8.ValidString(input.Text) {
		return ValidationError{Messages: []string{"text must be valid unicode text"}}
	}
	return nil
}
