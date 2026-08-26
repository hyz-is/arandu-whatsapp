package models

import (
	"encoding/json"
	"time"
)

// Webhook is the persisted webhook configuration exposed by the application.
type Webhook struct {
	ID         int64           `json:"id"`
	URL        string          `json:"url"`
	Enabled    bool            `json:"enabled"`
	Events     json.RawMessage `json:"events"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	InstanceID int64           `json:"instanceId"`
}
