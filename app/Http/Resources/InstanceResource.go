// Package resources declares the fields that may leave through HTTP responses.
package resources

import (
	"encoding/json"

	models "github.com/hyz-is/arandu-whatsapp/app/Models"
)

// InstanceResource declares the fields that may leave the module.
type InstanceResource struct{ instance models.Instance }

// NewInstanceResource wraps one instance for an Arandu JSON response.
func NewInstanceResource(instance models.Instance) InstanceResource {
	return InstanceResource{instance: instance}
}

// ToArray returns the public representation of an instance.
func (r InstanceResource) ToArray() map[string]any {
	return map[string]any{
		"id": r.instance.ID, "name": r.instance.Name, "description": r.instance.Description,
		"status": r.instance.Status, "connectionStatus": r.instance.ConnectionStatus,
		"ownerJid": r.instance.OwnerJID, "profilePicUrl": r.instance.ProfilePictureURL,
		"whatsappOwnerJid": r.instance.WhatsAppOwnerJID, "whatsappPhoneNumber": r.instance.WhatsAppPhone,
		"lastConnectedAt": r.instance.LastConnectedAt, "lastDisconnectedAt": r.instance.LastDisconnectedAt,
		"connectionAttempts": r.instance.ConnectionAttempts, "createdAt": r.instance.CreatedAt.UTC(),
		"updatedAt": r.instance.UpdatedAt.UTC(), "externalAttributes": jsonValue(r.instance.ExternalAttributes),
	}
}

// With returns no top-level metadata.
func (InstanceResource) With() map[string]any { return nil }

// InstanceCollection is a tenant-scoped instance page.
type InstanceCollection struct{ page models.InstancePage }

// NewInstanceCollection wraps an instance page for an Arandu JSON response.
func NewInstanceCollection(page models.InstancePage) InstanceCollection {
	return InstanceCollection{page: page}
}

// ToArray returns the page items and forward-pagination metadata.
func (c InstanceCollection) ToArray() map[string]any {
	items := make([]map[string]any, 0, len(c.page.Items))
	for _, item := range c.page.Items {
		items = append(items, NewInstanceResource(item).ToArray())
	}
	var nextCursor any
	if c.page.NextCursor != "" {
		nextCursor = c.page.NextCursor
	}
	return map[string]any{"items": items, "perPage": c.page.PerPage, "nextCursor": nextCursor}
}

// With returns no top-level metadata.
func (InstanceCollection) With() map[string]any { return nil }

func jsonValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return map[string]any{}
	}
	return value
}
