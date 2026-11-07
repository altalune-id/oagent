// Package openwa is the WhatsApp channel provider that talks to an OpenWA device.
package openwa

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type envelope struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	APIVersion string          `json:"api_version"`
	CreatedAt  time.Time       `json:"created_at"`
	Tenant     envelopeTenant  `json:"tenant"`
	Data       json.RawMessage `json:"data"`
}

type envelopeTenant struct {
	OrgID       uuid.UUID `json:"org_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	ProjectSlug string    `json:"project_slug"`
}

type messageEvent struct {
	Device  deviceRef  `json:"device"`
	Chat    chatRef    `json:"chat"`
	Sender  sender     `json:"sender"`
	Message message    `json:"message"`
	Matched *matchedBy `json:"matched,omitempty"`
}

type deviceRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type chatRef struct {
	ID   string `json:"id"`
	JID  string `json:"jid"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type sender struct {
	JID      string `json:"jid"`
	LID      string `json:"lid"`
	Phone    string `json:"phone"`
	PushName string `json:"push_name"`
	FromMe   bool   `json:"from_me"`
}

type quoted struct {
	WAID string `json:"wa_id"`
	Body string `json:"body"`
}

type media struct {
	Mime     string `json:"mime"`
	Size     int64  `json:"size"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Voice    bool   `json:"voice"`
}

type location struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Name    string  `json:"name"`
	Address string  `json:"address"`
}

type message struct {
	ID        string    `json:"id"`
	WAID      string    `json:"wa_id"`
	Type      string    `json:"type"`
	Body      string    `json:"body"`
	Caption   string    `json:"caption"`
	Timestamp time.Time `json:"timestamp"`
	Quoted    *quoted   `json:"quoted"`
	Mentions  []string  `json:"mentions"`
	Media     *media    `json:"media"`
	Location  *location `json:"location"`
}

type matchedBy struct {
	Reasons []string `json:"reasons"`
}
