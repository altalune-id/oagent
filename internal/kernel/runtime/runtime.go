package runtime

import "context"

// Runtime runs one agent turn and streams its events.
type Runtime interface {
	Turn(ctx context.Context, req TurnRequest) (Stream, error)
}

// Stream is one turn's events; Next returns io.EOF after the terminal event, and Close ends the turn early.
type Stream interface {
	Next() (Event, error)
	Close() error
}
