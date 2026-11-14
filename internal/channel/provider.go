package channel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// Kind is a chat's shape: a direct message or a group.
type Kind string

// The chat kinds.
const (
	KindDM    Kind = "dm"
	KindGroup Kind = "group"
)

// Sender is who wrote an inbound message, as the provider names them; Phone is empty when WhatsApp hides it.
type Sender struct {
	Phone, LID, DisplayName string
	FromMe                  bool
}

// MediaRef names an attachment to fetch from the provider at turn build; the bytes are never stored.
type MediaRef struct {
	MessageID, Mime string
	Size            int64
	Voice           bool
}

// Inbound is one provider-neutral inbound message.
type Inbound struct {
	EventType, DeviceID  string
	ExternalID, ChatKey  string
	Kind                 Kind
	ChatAddress          string
	Sender               Sender
	Type, Text, QuotedID string
	Mentions             []string
	Media                *MediaRef
	Time                 time.Time
}

// Reply is one outbound message; To is the provider address and IdempotencyKey makes a retried part a no-op.
type Reply struct {
	To, Text, ReplyTo, IdempotencyKey string
}

// Provider is a channel provider as the channel domain uses it.
type Provider interface {
	Verify(r *http.Request, body []byte, conn Connection, secrets Secrets) error
	DeliveryID(r *http.Request) string
	Parse(body []byte) (Inbound, error)
	Send(ctx context.Context, conn Connection, secrets Secrets, r Reply) (externalID string, err error)
	FetchMedia(
		ctx context.Context, conn Connection, secrets Secrets, ref MediaRef, maxBytes int64,
	) (body io.ReadCloser, mime string, err error)
	MarkRead(ctx context.Context, conn Connection, secrets Secrets, externalID string) error
}

// IgnoredError reports a delivery the provider understood but that carries no work, answered with success so it is not retried.
type IgnoredError struct{ Reason string }

func (e *IgnoredError) Error() string { return "channel: ignored delivery: " + e.Reason }

// IsIgnoredError reports whether err's tree contains an *IgnoredError.
func IsIgnoredError(err error) bool {
	_, ok := errors.AsType[*IgnoredError](err)
	return ok
}

// TooLargeError reports media over the size the caller allows.
type TooLargeError struct{ Limit int64 }

func (e *TooLargeError) Error() string { return "channel: media over the size limit" }

// IsTooLargeError reports whether err's tree contains a *TooLargeError.
func IsTooLargeError(err error) bool {
	_, ok := errors.AsType[*TooLargeError](err)
	return ok
}

// MediaTypeError reports media the provider does not hand over.
type MediaTypeError struct{}

func (e *MediaTypeError) Error() string { return "channel: unsupported media type" }

// IsMediaTypeError reports whether err's tree contains a *MediaTypeError.
func IsMediaTypeError(err error) bool {
	_, ok := errors.AsType[*MediaTypeError](err)
	return ok
}
