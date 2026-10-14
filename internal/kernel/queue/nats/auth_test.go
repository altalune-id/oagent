package nats

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/testutil/natstest"
)

func accountOptions(url, user, password string) testSetup {
	o := testOptions(url)
	o.user, o.password = user, password
	return o
}

func TestOpen_UserPasswordCreatesStreamsInItsAccount(t *testing.T) {
	c := connectTest(t, accountOptions(natstest.StartAccounts(t).ClientURL(), natstest.UserA, natstest.PasswordA))

	assert.Equal(t, []string{"jobs.>"}, streamInfo(t, c, streamWork).Config.Subjects)
}

func TestOpen_AccountsKeepSameNamedStreamsApart(t *testing.T) {
	url := natstest.StartAccounts(t).ClientURL()
	a := connectTest(t, accountOptions(url, natstest.UserA, natstest.PasswordA))
	b := connectTest(t, accountOptions(url, natstest.UserB, natstest.PasswordB))
	job := queue.Job{Name: "todo.log_completion", Version: 1}
	require.NoError(t, a.Declare([]queue.Job{job}, nil))
	require.NoError(t, b.Declare([]queue.Job{job}, nil))

	require.NoError(t, a.Submit(t.Context(), job, map[string]string{"from": "a"}))

	assert.Equal(t, uint64(1), streamInfo(t, a, streamWork).State.Msgs)
	assert.Equal(t, uint64(0), streamInfo(t, b, streamWork).State.Msgs)
}

func accountQueue(t *testing.T, url, user, password string) *Client {
	t.Helper()
	c := connectTest(t, accountOptions(url, user, password))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, []queue.Broadcast{testBroadcast()}))
	return c
}

func forward(got chan<- queue.Message) func(context.Context, queue.Message) error {
	return func(_ context.Context, m queue.Message) error {
		got <- m
		return nil
	}
}

func received(got <-chan queue.Message, within time.Duration) bool {
	select {
	case <-got:
		return true
	case <-time.After(within):
		return false
	}
}

// Review Focus 2: like the shared server, an account sees only its own jobs and broadcasts; the same account shares them.
func TestQueue_AccountsIsolateJobsAndBroadcasts(t *testing.T) {
	url := natstest.StartAccounts(t).ClientURL()
	producer := accountQueue(t, url, natstest.UserA, natstest.PasswordA)
	sameAccount := accountQueue(t, url, natstest.UserA, natstest.PasswordA)
	otherAccount := accountQueue(t, url, natstest.UserB, natstest.PasswordB)

	run := func(q queue.Queue) (jobs, broadcasts chan queue.Message) {
		jobs, broadcasts = make(chan queue.Message, 4), make(chan queue.Message, 4)
		cons, err := q.Consumer(t.Context(), []queue.Handler{{Job: testJob(), Handle: forward(jobs)}})
		require.NoError(t, err)
		runWorker(t, cons)
		l, err := q.Listener(t.Context(), []queue.Listener{{Broadcast: testBroadcast(), Handle: forward(broadcasts)}})
		require.NoError(t, err)
		runWorker(t, l)
		return jobs, broadcasts
	}
	sameJobs, sameBroadcasts := run(sameAccount)
	otherJobs, otherBroadcasts := run(otherAccount)

	require.NoError(t, producer.Submit(t.Context(), testJob(), testPayload{Title: "a"}))
	require.NoError(t, producer.Emit(t.Context(), testBroadcast(), testPayload{Title: "a"}))

	assert.True(t, received(sameJobs, 5*time.Second), "the same account consumes the job")
	assert.True(t, received(sameBroadcasts, 5*time.Second), "the same account hears the broadcast")
	assert.False(t, received(otherJobs, 500*time.Millisecond), "another account never sees the job")
	assert.False(t, received(otherBroadcasts, 100*time.Millisecond), "another account never hears the broadcast")
}
