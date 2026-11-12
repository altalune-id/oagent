package run

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var legalMoves = map[Status][]Status{
	StatusRunning:          {StatusCompleted, StatusLimitReached, StatusFailed, StatusCancelled, StatusInterrupted, StatusAwaitingApproval, StatusExpired},
	StatusInterrupted:      {StatusRunning, StatusFailed, StatusCancelled},
	StatusAwaitingApproval: {StatusRunning, StatusExpired, StatusCancelled},
}

func TestMove_EveryPair(t *testing.T) {
	require.Len(t, Statuses(), 8)
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	for _, from := range Statuses() {
		for _, to := range Statuses() {
			r := &Run{Status: from, Attempt: 1}
			err := r.Move(to, now)
			legal := false
			for _, ok := range legalMoves[from] {
				legal = legal || ok == to
			}
			if legal {
				assert.NoError(t, err, "%s -> %s", from, to)
				assert.Equal(t, to, r.Status)
				assert.Equal(t, now, r.UpdatedAt)
				continue
			}
			e, ok := errors.AsType[*IllegalMoveError](err)
			require.True(t, ok, "%s -> %s must be refused, got %v", from, to, err)
			assert.Equal(t, from, e.From)
			assert.Equal(t, to, e.To)
			assert.Equal(t, from, r.Status, "a refused move changes nothing")
		}
	}
}

func TestMove_TerminalIsFinal(t *testing.T) {
	for _, s := range []Status{StatusCompleted, StatusFailed, StatusCancelled, StatusLimitReached, StatusExpired} {
		assert.True(t, s.Terminal(), s)
		for _, to := range Statuses() {
			assert.True(t, IsIllegalMoveError((&Run{Status: s}).Move(to, time.Now())), "%s -> %s", s, to)
		}
	}
	for _, s := range []Status{StatusRunning, StatusInterrupted, StatusAwaitingApproval} {
		assert.False(t, s.Terminal(), s)
	}
}

func TestMove_SetsAttemptAndEnd(t *testing.T) {
	start := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	r := New(uuid.New(), uuid.New(), uuid.New(), uuid.New(), start)
	assert.Equal(t, StatusRunning, r.Status)
	assert.Equal(t, int32(1), r.Attempt)
	assert.Len(t, r.PublicID, 21)
	assert.Equal(t, start, r.StartedAt)
	assert.Nil(t, r.EndedAt)

	require.NoError(t, r.Move(StatusInterrupted, start.Add(time.Second)))
	assert.Nil(t, r.EndedAt, "interrupted is not an end")
	require.NoError(t, r.Move(StatusRunning, start.Add(2*time.Second)))
	assert.Equal(t, int32(2), r.Attempt, "a resume is the next attempt")

	end := start.Add(3 * time.Second)
	require.NoError(t, r.Move(StatusCompleted, end))
	require.NotNil(t, r.EndedAt)
	assert.Equal(t, end, *r.EndedAt)
}

func TestSessionID_ShapeAndLength(t *testing.T) {
	id := uuid.MustParse("0193a2b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b")
	assert.Equal(t, "t0193a2b4c5d67e8f9a0b1c2d3e4f5a6b", SessionID(id))
	assert.Regexp(t, regexp.MustCompile(`^t[0-9a-f]{32}$`), SessionID(uuid.New()))
}

func TestAgentKey_Shape(t *testing.T) {
	assert.Regexp(t, regexp.MustCompile(`^a[0-9a-f]{32}$`), AgentKey(uuid.New()))
	assert.Equal(t, "a"+SessionID(uuid.Nil)[1:], AgentKey(uuid.Nil))
}

func price(v int64) *int64 { return new(v) }

func TestRun_CostRoundsUp(t *testing.T) {
	one := &Prices{Input: price(1)}
	micros, priced := Cost(Usage{InputTokens: 1}, one)
	assert.True(t, priced)
	assert.Equal(t, int64(1), micros, "1 token at 1 micro per 1M is 1 micro")

	p := &Prices{Input: price(2_500_000), Output: price(10_000_000), CacheRead: price(1_250_000), CacheWrite: price(0)}
	micros, priced = Cost(Usage{InputTokens: 1000, OutputTokens: 1, CacheReadTokens: 3, CacheWriteTokens: 7}, p)
	assert.True(t, priced)
	assert.Equal(t, int64(2500+10+4+0), micros, "each class rounds up on its own, then they are summed")

	micros, priced = Cost(Usage{}, nil)
	assert.False(t, priced, "no price snapshot is unpriced")
	assert.Zero(t, micros)
}

func TestCost_UnpricedClassIsUnpriced(t *testing.T) {
	p := &Prices{Input: price(1_000_000), Output: price(1_000_000)}
	_, priced := Cost(Usage{InputTokens: 10, CacheReadTokens: 1}, p)
	assert.False(t, priced, "tokens in a class without a price make the run unpriced")
	micros, priced := Cost(Usage{InputTokens: 10, OutputTokens: 2}, p)
	assert.True(t, priced, "an unpriced class with no tokens costs nothing")
	assert.Equal(t, int64(12), micros)
}

func TestCost_SaturatesInsteadOfOverflowing(t *testing.T) {
	huge := &Prices{Input: price(math.MaxInt64), Output: price(math.MaxInt64)}
	micros, priced := Cost(Usage{InputTokens: math.MaxInt64}, huge)
	assert.True(t, priced)
	assert.Equal(t, int64(math.MaxInt64), micros, "a product past int64 reads as the largest cost, never negative")
	micros, _ = Cost(Usage{InputTokens: math.MaxInt64}, &Prices{Input: price(2_000_000)})
	assert.Equal(t, int64(math.MaxInt64), micros, "a quotient past int64 saturates")
	micros, _ = Cost(Usage{InputTokens: 9_223_367_425_171_063_222}, &Prices{Input: price(2_000_001)})
	assert.Equal(t, int64(math.MaxInt64), micros, "a quotient of 2^64-1 with a remainder saturates instead of wrapping on the round-up")
	micros, _ = Cost(Usage{InputTokens: 1 << 41, OutputTokens: 1 << 41}, &Prices{Input: price(1 << 41), Output: price(1 << 41)})
	assert.Equal(t, int64(math.MaxInt64), micros, "a sum past int64 saturates too")
	micros, _ = Cost(Usage{InputTokens: 3_000_000_000_000}, &Prices{Input: price(4_000_000_000)})
	assert.Equal(t, int64(12_000_000_000_000_000), micros, "a product past int64 whose quotient fits is exact")
	assert.Equal(t, int64(math.MaxInt64/10_000+1), Cents(math.MaxInt64))
}

func TestCents_RoundsUp(t *testing.T) {
	for micros, want := range map[int64]int64{0: 0, 1: 1, 9_999: 1, 10_000: 1, 10_001: 2, 25_000: 3} {
		assert.Equal(t, want, Cents(micros), "%d micros", micros)
	}
}

func TestErrors_NameTheirSubject(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
		is   func(error) bool
	}{
		{&IllegalMoveError{From: StatusCompleted, To: StatusRunning}, "completed -> running", IsIllegalMoveError},
		{&LeaseLostError{ThreadID: "t1"}, "t1", IsLeaseLostError},
		{&ThreadNotFoundError{ID: "t2"}, "t2", IsThreadNotFoundError},
		{&NotFoundError{ID: "r1"}, "r1", IsNotFoundError},
		{&UnsettledError{Status: StatusRunning}, "running", func(error) bool { return true }},
	} {
		assert.Contains(t, c.err.Error(), c.want)
		assert.True(t, c.is(fmt.Errorf("wrapped: %w", c.err)))
	}
}

func TestCost_EmbeddingClassOwnPrice(t *testing.T) {
	p := &Prices{Input: new(int64(2_500_000)), Output: new(int64(10_000_000)), Embedding: new(int64(20_000))}
	micros, priced := Cost(Usage{InputTokens: 1_000_000, EmbeddingTokens: 1_000_000}, p)
	require.True(t, priced)
	assert.Equal(t, int64(2_500_000+20_000), micros, "embedding tokens at the embedding price, never the main input price")

	_, priced = Cost(Usage{EmbeddingTokens: 1}, &Prices{Input: new(int64(1))})
	assert.False(t, priced, "embedding tokens with no embedding price leave the run unpriced")
	micros, priced = Cost(Usage{InputTokens: 10}, &Prices{Input: new(int64(1_000_000))})
	assert.True(t, priced, "no embedding tokens need no embedding price")
	assert.Equal(t, int64(10), micros)
}
