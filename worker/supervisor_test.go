package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWorker struct {
	name string
	run  func(context.Context) error
}

func (f fakeWorker) Name() string                  { return f.name }
func (f fakeWorker) Run(ctx context.Context) error { return f.run(ctx) }

func TestSupervisor_NoWorkers(t *testing.T) {
	assert.NoError(t, New(nil).Run(context.Background()))
}

func TestSupervisor_AllReturnNil(t *testing.T) {
	s := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 2)
	s.Register(fakeWorker{name: "a", run: func(ctx context.Context) error { started <- struct{}{}; <-ctx.Done(); return nil }})
	s.Register(fakeWorker{name: "b", run: func(ctx context.Context) error { started <- struct{}{}; <-ctx.Done(); return nil }})

	go func() {
		<-started
		<-started
		cancel()
	}()
	require.NoError(t, s.Run(ctx))
}

func TestSupervisor_ErrorCancelsSiblings(t *testing.T) {
	s := New(nil)
	boom := errors.New("boom")
	siblingCanceled := make(chan struct{})
	s.Register(fakeWorker{name: "bad", run: func(context.Context) error { return boom }})
	s.Register(fakeWorker{name: "sibling", run: func(ctx context.Context) error { <-ctx.Done(); close(siblingCanceled); return nil }})

	assert.ErrorIs(t, s.Run(context.Background()), boom)
	select {
	case <-siblingCanceled:
	case <-time.After(time.Second):
		t.Fatal("sibling was not canceled")
	}
}

func TestSupervisor_ContextCancel(t *testing.T) {
	s := New(nil)
	started := make(chan struct{})
	s.Register(fakeWorker{name: "a", run: func(ctx context.Context) error { close(started); <-ctx.Done(); return nil }})

	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	require.NoError(t, s.Run(ctx))
}

func TestSupervisor_GracefulCanceledIsClean(t *testing.T) {
	s := New(nil)
	s.Register(fakeWorker{name: "a", run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.NoError(t, s.Run(ctx)) // a Canceled child during shutdown is not an error
}

func TestSupervisor_NilLogger(t *testing.T) {
	assert.NotNil(t, New(nil))
}

func TestSupervisor_WorkersReportsRegistrationOrder(t *testing.T) {
	s := New(nil)
	s.Register(fakeWorker{name: "a", run: func(context.Context) error { return nil }})
	s.Register(fakeWorker{name: "b", run: func(context.Context) error { return nil }})

	ws := s.Workers()
	require.Len(t, ws, 2)
	assert.Equal(t, "a", ws[0].Name())
	assert.Equal(t, "b", ws[1].Name())
}
