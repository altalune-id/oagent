package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	job := Job{Name: "todo.log_completion", Version: 1}
	require.NoError(t, a.Declare([]Job{job}, nil))
	require.NoError(t, b.Declare([]Job{job}, nil))

	require.NoError(t, a.Submit(t.Context(), job, map[string]string{"from": "a"}))

	assert.Equal(t, uint64(1), streamInfo(t, a, streamWork).State.Msgs)
	assert.Equal(t, uint64(0), streamInfo(t, b, streamWork).State.Msgs)
}
