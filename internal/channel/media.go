package channel

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// Media reads inbound photos and sends read receipts through an active connection's provider.
type Media struct {
	Service  *Service
	Provider Provider
}

// Fetch reads one inbound photo of at most maxBytes.
func (m Media) Fetch(
	ctx context.Context, connectionID uuid.UUID, messageID, mime string, maxBytes int64,
) (data []byte, mimeType string, err error) {
	c, secrets, err := m.Service.Open(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}
	body, mimeType, err := m.Provider.FetchMedia(ctx, *c, secrets, MediaRef{MessageID: messageID, Mime: mime}, maxBytes)
	if err != nil {
		return nil, "", m.refused(ctx, c, err)
	}
	defer func() { _ = body.Close() }()
	data, err = io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", &TooLargeError{Limit: maxBytes}
	}
	return data, mimeType, nil
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
