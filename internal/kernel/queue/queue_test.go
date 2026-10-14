package queue

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testJob() Job { return Job{Name: "test.run", Version: 1} }

func testBroadcast() Broadcast { return Broadcast{Name: "test.ping", Version: 1} }

type testPayload struct {
	Title string `json:"title"`
}

func TestDeclare_Rejects(t *testing.T) {
	c := Disabled(slog.New(slog.DiscardHandler))

	tests := []struct {
		name       string
		jobs       []Job
		broadcasts []Broadcast
		check      func(error) bool
	}{
		{"invalid job", []Job{{Name: "bad", Version: 1}}, nil, IsInvalidJobError},
		{"invalid broadcast", nil, []Broadcast{{Name: "test.ping", Version: 0}}, IsInvalidBroadcastError},
		{"duplicate job", []Job{testJob(), testJob()}, nil, IsHandlerWiringError},
		{"duplicate broadcast", nil, []Broadcast{testBroadcast(), testBroadcast()}, IsHandlerWiringError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Declare(tt.jobs, tt.broadcasts)
			require.Error(t, err)
			assert.True(t, tt.check(err), "got %T: %v", err, err)
		})
	}
}

func TestDeclare_DuplicateNamesEverySubject(t *testing.T) {
	c := Disabled(slog.New(slog.DiscardHandler))
	err := c.Declare([]Job{testJob(), testJob()}, []Broadcast{testBroadcast(), testBroadcast()})

	we, ok := errors.AsType[*HandlerWiringError](err)
	require.True(t, ok)
	assert.Equal(t, []string{"broadcast.test.ping.v1", "jobs.test.run.v1"}, we.Duplicate)
}

func TestDeclare_SameNameAsJobAndBroadcastIsNotDuplicate(t *testing.T) {
	c := Disabled(slog.New(slog.DiscardHandler))
	require.NoError(t, c.Declare([]Job{{Name: "test.x", Version: 1}}, []Broadcast{{Name: "test.x", Version: 1}}))
}

func TestDisabled(t *testing.T) {
	c := Disabled(slog.New(slog.DiscardHandler))

	require.NoError(t, c.Declare([]Job{testJob()}, []Broadcast{testBroadcast()}))
	require.NoError(t, c.Submit(t.Context(), testJob(), testPayload{}))
	require.NoError(t, c.Submit(t.Context(), Job{Name: "test.undeclared", Version: 1}, testPayload{}))
	require.NoError(t, c.Emit(t.Context(), testBroadcast(), testPayload{}))
}

func TestDisabled_NilLogger(t *testing.T) {
	c := Disabled(nil)
	require.NoError(t, c.Submit(t.Context(), testJob(), testPayload{}))
}

func TestDisabled_DeclareStillValidates(t *testing.T) {
	c := Disabled(slog.New(slog.DiscardHandler))
	assert.True(t, IsInvalidJobError(c.Declare([]Job{{Name: "bad", Version: 1}}, nil)))
}

func TestErrors_Messages(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		check func(error) bool
	}{
		{"publish", &PublishError{Subject: "jobs.a.b.v1", Cause: context.DeadlineExceeded}, IsPublishError},
		{"invalid", &InvalidJobError{Job: testJob(), Reason: "x"}, IsInvalidJobError},
		{"invalid broadcast", &InvalidBroadcastError{Broadcast: testBroadcast(), Reason: "x"}, IsInvalidBroadcastError},
		{"undeclared job", &UndeclaredJobError{Job: testJob()}, IsUndeclaredJobError},
		{"undeclared broadcast", &UndeclaredBroadcastError{Broadcast: testBroadcast()}, IsUndeclaredBroadcastError},
		{"wiring", &HandlerWiringError{Duplicate: []string{"jobs.a.b.v1"}}, IsHandlerWiringError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), "queue:")
			assert.True(t, tt.check(tt.err))
			assert.False(t, tt.check(context.Canceled))
		})
	}
	assert.ErrorIs(t, &PublishError{Cause: context.DeadlineExceeded}, context.DeadlineExceeded)
}

func TestDisabled_ConsumerIsDisabledError(t *testing.T) {
	w, err := Disabled(nil).Consumer(t.Context(), []Handler{{Job: testJob(), Handle: func(context.Context, Message) error { return nil }}})
	assert.Nil(t, w)
	require.Error(t, err)
	assert.True(t, IsDisabledError(err), "got %T: %v", err, err)
}

func TestDisabled_ListenerNilHandleNamesTheBroadcastSubject(t *testing.T) {
	_, err := Disabled(nil).Listener(t.Context(), []Listener{{Broadcast: testBroadcast()}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast.test.ping.v1")
}

func TestDisabled_ListenerIsDisabledError(t *testing.T) {
	w, err := Disabled(nil).Listener(t.Context(), []Listener{{Broadcast: testBroadcast(), Handle: func(context.Context, Message) error { return nil }}})
	assert.Nil(t, w)
	require.Error(t, err)
	assert.True(t, IsDisabledError(err), "got %T: %v", err, err)
}

func TestDeclared_ZeroValueAcceptsNothing(t *testing.T) {
	var d Declared
	assert.False(t, d.HasJob(testJob()))
	assert.False(t, d.HasBroadcast(testBroadcast()))
}

func TestNewDeclared(t *testing.T) {
	d, err := NewDeclared([]Job{testJob()}, []Broadcast{testBroadcast()})
	require.NoError(t, err)
	assert.True(t, d.HasJob(testJob()))
	assert.False(t, d.HasJob(Job{Name: "test.other", Version: 1}))
	assert.True(t, d.HasBroadcast(testBroadcast()))
	assert.False(t, d.HasBroadcast(Broadcast{Name: "test.run", Version: 1}), "a job is not a broadcast")
}

func TestCheckHandlers(t *testing.T) {
	ok := func(context.Context, Message) error { return nil }
	require.NoError(t, CheckHandlers(nil))
	require.NoError(t, CheckHandlers([]Handler{{Job: testJob(), Handle: ok}}))
	assert.True(t, IsNilHandlerError(CheckHandlers([]Handler{{Job: testJob()}})))
	assert.True(t, IsInvalidJobError(CheckHandlers([]Handler{{Job: Job{Name: "bad", Version: 1}, Handle: ok}})))
	assert.True(t, IsHandlerWiringError(CheckHandlers([]Handler{{Job: testJob(), Handle: ok}, {Job: testJob(), Handle: ok}})))
}

func TestCheckListeners(t *testing.T) {
	ok := func(context.Context, Message) error { return nil }
	require.NoError(t, CheckListeners([]Listener{{Broadcast: testBroadcast(), Handle: ok}}))
	assert.True(t, IsNilHandlerError(CheckListeners([]Listener{{Broadcast: testBroadcast()}})))
	assert.True(t, IsInvalidBroadcastError(CheckListeners([]Listener{{Broadcast: Broadcast{Name: "bad", Version: 1}, Handle: ok}})))
	assert.True(t, IsHandlerWiringError(CheckListeners([]Listener{{Broadcast: testBroadcast(), Handle: ok}, {Broadcast: testBroadcast(), Handle: ok}})))
}
