package run

import (
	"context"

	"github.com/google/uuid"
)

const maxHistoryMessages = 40

func window(msgs []Message) []Message {
	for i, m := range msgs {
		if m.Role == RoleUser {
			return msgs[i:]
		}
	}
	return nil
}

func (s *Service) history(ctx context.Context, threadID, exceptRun uuid.UUID) ([]Message, error) {
	msgs, err := s.store.History(ctx, threadID, exceptRun, maxHistoryMessages)
	if err != nil {
		return nil, err
	}
	if w := window(msgs); len(w) > 0 || len(msgs) == 0 {
		return w, nil
	}
	return s.store.HistoryFrom(ctx, threadID, exceptRun)
}
