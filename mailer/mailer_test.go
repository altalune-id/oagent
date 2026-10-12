package mailer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessage_ValidateRejectsHeaderInjection(t *testing.T) {
	cases := []struct {
		name  string
		m     Message
		field string
	}{
		{"crlf in from", Message{From: "f@x\r\nBcc: attacker@evil.com", To: "t@x", Subject: "s"}, "From"},
		{"lf in to", Message{From: "f@x", To: "t@x\nBcc: attacker@evil.com", Subject: "s"}, "To"},
		{"cr in subject", Message{From: "f@x", To: "t@x", Subject: "hi\rBcc: attacker@evil.com"}, "Subject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			require.True(t, IsHeaderInjectionError(err), "got %T: %v", err, err)
			var target *HeaderInjectionError
			require.ErrorAs(t, err, &target)
			assert.Equal(t, tc.field, target.Field)
		})
	}
}

func TestMessage_ValidateRequiresFields(t *testing.T) {
	cases := []struct {
		name  string
		m     Message
		field string
	}{
		{"empty to", Message{Subject: "s", TextBody: "b"}, "to"},
		{"empty subject", Message{To: "t@x", TextBody: "b"}, "subject"},
		{"no body", Message{To: "t@x", Subject: "s"}, "body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			var target *IncompleteMessageError
			require.ErrorAs(t, err, &target)
			assert.Equal(t, tc.field, target.Field)
			assert.True(t, IsIncompleteMessageError(err))
		})
	}
	require.NoError(t, Message{To: "t@x", Subject: "s", HTMLBody: "<p>b</p>"}.Validate())
}

func TestIdempotencyKey(t *testing.T) {
	_, ok := IdempotencyKey(t.Context())
	assert.False(t, ok)
	got, ok := IdempotencyKey(WithIdempotencyKey(t.Context(), "k-1"))
	assert.True(t, ok)
	assert.Equal(t, "k-1", got)
	_, ok = IdempotencyKey(WithIdempotencyKey(context.Background(), ""))
	assert.False(t, ok, "an empty key is no key")
}

func TestErrorMessages(t *testing.T) {
	for _, err := range []error{
		&HeaderInjectionError{Field: "To"},
		&IncompleteMessageError{Field: "to"},
		&MissingConfigError{Field: "from"},
		&InvalidConfigError{Field: "driver", Value: "x", Reason: "unknown driver"},
		&InvalidAddressError{Field: "To"},
	} {
		assert.True(t, strings.HasPrefix(err.Error(), "mailer: "), err.Error())
	}
	assert.False(t, IsInvalidAddressError(errors.New("x")))
	assert.False(t, IsMissingConfigError(errors.New("x")))
	assert.False(t, IsInvalidConfigError(errors.New("x")))
}
