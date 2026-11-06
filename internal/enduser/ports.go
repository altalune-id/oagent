package enduser

import (
	"context"

	"github.com/google/uuid"
)

// KeyHints names an API key by its public hint, for the label of an end user that is a key; an unknown key has an empty hint.
type KeyHints interface {
	KeyHint(ctx context.Context, keyID uuid.UUID) (string, error)
}
