// Package events is the hardcoded catalog of outbound webhook event types and their payload versions, the public contract tenants receive.
package events

import "strconv"

// Type names an event, without its version.
type Type string

// Event types; the catalog is additive-only once a type has shipped.
const (
	AgentCreated Type = "agent.created"
	AgentUpdated Type = "agent.updated"

	WebhookPing Type = "webhook.ping"
)

// Spec is one catalog entry.
type Spec struct {
	Type         Type
	Version      int
	Subscribable bool
}

// APIVersion renders the payload version as it appears in the envelope.
func (s Spec) APIVersion() string { return "v" + strconv.Itoa(s.Version) }

// All returns every catalog entry in a stable order.
func All() []Spec {
	return []Spec{
		{Type: AgentCreated, Version: 1, Subscribable: true},
		{Type: AgentUpdated, Version: 1, Subscribable: true},
		{Type: WebhookPing, Version: 1},
	}
}

// Subscribable returns the entries the console picker may offer, in All order.
func Subscribable() []Spec {
	all := All()
	out := make([]Spec, 0, len(all))
	for _, s := range all {
		if s.Subscribable {
			out = append(out, s)
		}
	}
	return out
}

// Lookup returns the entry for t.
func Lookup(t Type) (Spec, bool) {
	for _, s := range All() {
		if s.Type == t {
			return s, true
		}
	}
	return Spec{}, false
}

// CheckPayload reports a *PayloadMismatchError when data is not t's payload type, or an *UnknownTypeError when t is not in the catalog.
func CheckPayload(t Type, data any) error {
	switch t {
	case AgentCreated:
		if _, ok := data.(AgentCreatedV1); !ok {
			return &PayloadMismatchError{Type: t, Data: data}
		}
	case AgentUpdated:
		if _, ok := data.(AgentUpdatedV1); !ok {
			return &PayloadMismatchError{Type: t, Data: data}
		}
	case WebhookPing:
		if _, ok := data.(WebhookPingV1); !ok {
			return &PayloadMismatchError{Type: t, Data: data}
		}
	default:
		return &UnknownTypeError{Type: t}
	}
	return nil
}
