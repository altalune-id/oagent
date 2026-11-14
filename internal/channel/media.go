package channel

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/run"
)

// Media reads inbound photos and voice notes and sends read receipts through an active connection's provider.
type Media struct {
	Service  *Service
	Provider Provider
}

// Fetch reads one inbound photo or voice note of at most maxBytes; a refused type or size wraps run.ErrMediaType or run.ErrMediaTooLarge.
func (m Media) Fetch(
	ctx context.Context, connectionID uuid.UUID, messageID, mime string, maxBytes int64,
) (data []byte, mimeType string, err error) {
	c, secrets, err := m.Service.Open(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}
	body, mimeType, err := m.Provider.FetchMedia(ctx, *c, secrets, MediaRef{MessageID: messageID, Mime: mime}, maxBytes)
	if err != nil {
		return nil, "", mediaErr(m.refused(ctx, c, err))
	}
	defer func() { _ = body.Close() }()
	data, err = io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", mediaErr(&TooLargeError{Limit: maxBytes})
	}
	return data, mimeType, nil
}

func mediaErr(err error) error {
	switch {
	case IsTooLargeError(err):
		return fmt.Errorf("%w: %w", run.ErrMediaTooLarge, err)
	case IsMediaTypeError(err):
		return fmt.Errorf("%w: %w", run.ErrMediaType, err)
	}
	return err
}

// MarkRead marks the chat of messageID read up to it.
func (m Media) MarkRead(ctx context.Context, connectionID uuid.UUID, messageID string) error {
	c, secrets, err := m.Service.Open(ctx, connectionID)
	if err != nil {
		return err
	}
	return m.refused(ctx, c, m.Provider.MarkRead(ctx, *c, secrets, messageID))
}

func (m Media) refused(ctx context.Context, c *Connection, err error) error {
	if IsKeyRefusedError(err) {
		_ = m.Service.Record(ctx, c.ID, ResultKeyRefused)
	}
	return err
}
