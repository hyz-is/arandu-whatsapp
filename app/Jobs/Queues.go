// Package jobs declares the durable jobs owned by the WhatsApp module.
package jobs

import (
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
)

// WebhookQueueName is the native queue used for webhook deliveries.
const WebhookQueueName = webhooksvc.WebhookQueueName

// MessageQueueName is the native queue used for durable message processing.
const MessageQueueName = message.MessageQueueName

// WebhookDeliveryJobName is the stable webhook delivery handler name.
const WebhookDeliveryJobName = webhooksvc.WebhookDeliveryJobName

// MessageProcessingJobName is the stable mention-all processing handler name.
const MessageProcessingJobName = message.MessageProcessingJobName

// MessageProcessingCleanupJobName is the stable mention-all cleanup handler name.
const MessageProcessingCleanupJobName = message.MessageProcessingCleanupJobName
