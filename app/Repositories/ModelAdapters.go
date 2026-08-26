package repositories

import (
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// WebhookFromDatabase converts a persistence webhook into its application model.
func WebhookFromDatabase(item dbtypes.Webhook) models.Webhook {
	return models.Webhook{
		ID: item.ID, URL: item.URL, Enabled: item.Enabled, Events: item.Events,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, InstanceID: item.InstanceID,
	}
}

// MessageFromDatabase converts a persistence message into its application model.
func MessageFromDatabase(item dbtypes.Message) models.Message {
	return models.Message{
		ID: item.ID, KeyID: item.KeyID, KeyRemoteJid: item.KeyRemoteJid,
		KeyLid: item.KeyLid, KeyFromMe: item.KeyFromMe, KeyParticipant: item.KeyParticipant,
		KeyParticipantLid: item.KeyParticipantLid, PushName: item.PushName,
		MessageType: item.MessageType, Content: item.Content,
		MessageTimestamp: item.MessageTimestamp, Device: models.DeviceMessage(item.Device),
		IsGroup: item.IsGroup, InstanceID: item.InstanceID, Metadata: item.Metadata,
		ExternalAttributes: item.ExternalAttributes,
	}
}

// MessageListFromDatabase converts a persistence page into its application model.
func MessageListFromDatabase(item dbtypes.MessageListResult) models.MessageListResult {
	var records []models.MessageWithUpdates
	if item.Messages.Records != nil {
		records = make([]models.MessageWithUpdates, 0, len(item.Messages.Records))
		for _, record := range item.Messages.Records {
			var updates []models.MessageUpdateSummary
			if record.MessageUpdate != nil {
				updates = make([]models.MessageUpdateSummary, 0, len(record.MessageUpdate))
				for _, update := range record.MessageUpdate {
					updates = append(updates, models.MessageUpdateSummary{
						Status: update.Status, DateTime: update.DateTime,
					})
				}
			}
			records = append(records, models.MessageWithUpdates{
				Message: MessageFromDatabase(record.Message), MessageUpdate: updates,
			})
		}
	}
	return models.MessageListResult{Messages: models.MessagePage{
		Total: item.Messages.Total, Pages: item.Messages.Pages,
		CurrentPage: item.Messages.CurrentPage, Records: records,
	}}
}
