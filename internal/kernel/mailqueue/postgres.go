package mailqueue

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/schema/entity"
	"altalune.id/oagent/sealer"
)

var (
	_ Queue       = (*postgresStore)(nil)
	_ WorkerStore = (*postgresStore)(nil)
)

type postgresStore struct {
	pool       db.Pool
	q          *entity.MailQueue
	sealer     sealer.Sealer
	allowPlain bool
}

// NewStore builds the Postgres queue; allowPlain stores unsealed JSON when sl has no key.
func NewStore(pool db.Pool, schema, prefix string, sl sealer.Sealer, allowPlain bool) *postgresStore {
	if sl == nil {
		sl = sealer.Disabled()
	}
	return &postgresStore{pool: pool, q: entity.NewMailQueue(schema, prefix), sealer: sl, allowPlain: allowPlain}
}

func (s *postgresStore) seal(id uuid.UUID, m mailer.Message) (payload []byte, sealed bool, err error) {
	plain, err := json.Marshal(m)
	if err != nil {
		return nil, false, fmt.Errorf("mailqueue: encode: %w", err)
	}
	out, err := s.sealer.Seal(plain, id[:])
	switch {
	case err == nil:
		return out, true, nil
	case sealer.IsUnavailableError(err) && s.allowPlain:
		return plain, false, nil
	case sealer.IsUnavailableError(err):
		return nil, false, &SealerRequiredError{}
	default:
		return nil, false, fmt.Errorf("mailqueue: seal: %w", err)
	}
}

// Open unseals the message of e, binding the seal to the entry id.
func (s *postgresStore) Open(e Entry) (mailer.Message, error) {
	if e.Payload == nil {
		return mailer.Message{}, &PayloadError{ID: e.ID.String(), Reason: "missing"}
	}
	plain := e.Payload
	if e.Sealed {
		var err error
		if plain, err = s.sealer.Open(e.Payload, e.ID[:]); err != nil {
			return mailer.Message{}, &PayloadError{ID: e.ID.String(), Reason: openReason(err)}
		}
	}
	var m mailer.Message
	if err := json.Unmarshal(plain, &m); err != nil {
		return mailer.Message{}, &PayloadError{ID: e.ID.String(), Reason: "not a message"}
	}
	return m, nil
}

func openReason(err error) string {
	if sealer.IsUnavailableError(err) {
		return "sealed, but no encryption key is configured"
	}
	if sealer.IsOpenFailedError(err) {
		return "failed authentication (wrong key or tampered)"
	}
	return "cannot be opened"
}
