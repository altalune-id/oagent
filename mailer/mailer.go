// Package mailer defines the Message and Mailer contract that the console, smtp and resend drivers implement.
package mailer

import (
	"context"
	"strings"
)

// Message is one email being sent; an empty From uses the driver's default sender.
type Message struct {
	To       string
	From     string
	Subject  string
	TextBody string
	HTMLBody string
}

// Validate rejects CR or LF in header fields (RFC 5322 section 2.2.3), an empty To or Subject, and a Message with no body.
func (m Message) Validate() error {
	for _, f := range []struct{ name, val string }{
		{"From", m.From},
		{"To", m.To},
		{"Subject", m.Subject},
	} {
		if strings.ContainsAny(f.val, "\r\n") {
			return &HeaderInjectionError{Field: f.name}
		}
	}
	switch {
	case m.To == "":
		return &IncompleteMessageError{Field: "to"}
	case m.Subject == "":
		return &IncompleteMessageError{Field: "subject"}
	case m.TextBody == "" && m.HTMLBody == "":
		return &IncompleteMessageError{Field: "body"}
	}
	return nil
}

// Mailer sends a Message through one driver.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

type idempotencyKeyCtx struct{}

// WithIdempotencyKey returns ctx carrying key, which drivers that support it send so a retried Send delivers once.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, idempotencyKeyCtx{}, key)
}

// IdempotencyKey returns the key WithIdempotencyKey put on ctx; an empty key counts as none.
func IdempotencyKey(ctx context.Context) (string, bool) {
	key, _ := ctx.Value(idempotencyKeyCtx{}).(string)
	return key, key != ""
}
