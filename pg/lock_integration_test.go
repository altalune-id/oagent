//go:build integration

package pg_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/pg"
)

func openReplica(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	sqlDB, err := pg.Open(t.Context(), pg.Config{DSN: dsn, ConnectTimeout: 10 * time.Second}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func tryLock(t *testing.T, sqlDB *sql.DB, name string) bool {
	t.Helper()
	conn, err := sqlDB.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	var ok bool
	require.NoError(t, conn.QueryRowContext(t.Context(), `SELECT pg_try_advisory_lock($1)`, pg.LockKey(name)).Scan(&ok))
	if ok {
		_, err = conn.ExecContext(t.Context(), `SELECT pg_advisory_unlock($1)`, pg.LockKey(name))
		require.NoError(t, err)
	}
	return ok
}

func waitingOn(t *testing.T, sqlDB *sql.DB, name string) int {
	t.Helper()
	key := pg.LockKey(name)
	var n int
	require.NoError(t, sqlDB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted
		   AND classid = (($1::bigint >> 32) & 4294967295)::oid AND objid = ($1::bigint & 4294967295)::oid`,
		key).Scan(&n))
	return n
}

func TestRunWithAdvisoryLock_SerialisesTwoReplicas(t *testing.T) {
	dsn := pgtest.New(t).DSN
	replicaA, replicaB, observer := openReplica(t, dsn), openReplica(t, dsn), openReplica(t, dsn)
	name := "job-" + uuid.NewString()

	entered := make(chan struct{})
	release := make(chan struct{})
	doneA := make(chan error, 1)
	go func() {
		doneA <- pg.RunWithAdvisoryLock(t.Context(), replicaA, name, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	assert.False(t, tryLock(t, observer, name), "the lock must be held on the server while fn runs")

	var bRan atomic.Bool
	doneB := make(chan error, 1)
	go func() {
		doneB <- pg.RunWithAdvisoryLock(t.Context(), replicaB, name, func(context.Context) error {
			bRan.Store(true)
			return nil
		})
	}()
	require.Eventually(t, func() bool { return waitingOn(t, observer, name) == 1 }, 10*time.Second, 10*time.Millisecond,
		"replica B must queue on the held lock")
	assert.False(t, bRan.Load(), "replica B must not run while A holds the lock")

	close(release)
	require.NoError(t, <-doneA)
	require.NoError(t, <-doneB)
	assert.True(t, bRan.Load())
	assert.True(t, tryLock(t, observer, name), "the lock must be released after both runs")
}

func TestRunWithAdvisoryLock_ReleasesAndPropagatesWhenFnFails(t *testing.T) {
	dsn := pgtest.New(t).DSN
	replica, observer := openReplica(t, dsn), openReplica(t, dsn)
	name := "job-" + uuid.NewString()
	boom := errors.New("boom")

	err := pg.RunWithAdvisoryLock(t.Context(), replica, name, func(context.Context) error { return boom })
	require.ErrorIs(t, err, boom)
	assert.True(t, tryLock(t, observer, name))
	assert.Zero(t, replica.Stats().InUse, "the lock connection must return to the pool")
}

func TestRunWithAdvisoryLock_DifferentNamesDoNotContend(t *testing.T) {
	dsn := pgtest.New(t).DSN
	replicaA, replicaB := openReplica(t, dsn), openReplica(t, dsn)
	suffix := uuid.NewString()

	err := pg.RunWithAdvisoryLock(t.Context(), replicaA, "one-"+suffix, func(ctx context.Context) error {
		return pg.RunWithAdvisoryLock(ctx, replicaB, "two-"+suffix, func(context.Context) error { return nil })
	})
	require.NoError(t, err)
}

func TestRunWithAdvisoryLock_WaiterGivesUpWhenItsContextEnds(t *testing.T) {
	dsn := pgtest.New(t).DSN
	replicaA, replicaB, observer := openReplica(t, dsn), openReplica(t, dsn), openReplica(t, dsn)
	name := "job-" + uuid.NewString()

	entered := make(chan struct{})
	release := make(chan struct{})
	doneA := make(chan error, 1)
	go func() {
		doneA <- pg.RunWithAdvisoryLock(t.Context(), replicaA, name, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithCancel(t.Context())
	doneB := make(chan error, 1)
	var bRan atomic.Bool
	go func() {
		doneB <- pg.RunWithAdvisoryLock(ctx, replicaB, name, func(context.Context) error {
			bRan.Store(true)
			return nil
		})
	}()
	require.Eventually(t, func() bool { return waitingOn(t, observer, name) == 1 }, 10*time.Second, 10*time.Millisecond)
	cancel()
	require.Error(t, <-doneB)
	assert.False(t, bRan.Load())
	require.Eventually(t, func() bool { return waitingOn(t, observer, name) == 0 }, 10*time.Second, 10*time.Millisecond,
		"the cancelled waiter must leave the lock queue")

	close(release)
	require.NoError(t, <-doneA)
	assert.True(t, tryLock(t, observer, name))
}
