package slug_test

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/slug"
)

var shape = regexp.MustCompile(`^[a-z]{3,12}-[a-z]{3,12}-[1-9][0-9]{3}$`)

func TestGenerate_Shape(t *testing.T) {
	for range 2000 {
		assert.Regexp(t, shape, slug.Generate())
	}
}

func TestGenerate_Varies(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		seen[slug.Generate()] = true
	}
	assert.GreaterOrEqual(t, len(seen), 100)
}

func TestCombinations(t *testing.T) {
	assert.GreaterOrEqual(t, slug.Combinations(), 1_000_000)
}

var errTaken = errors.New("taken")

func isTaken(err error) bool { return errors.Is(err, errTaken) }

func TestRetry_ReturnsTheFirstFreeCandidate(t *testing.T) {
	calls := 0
	got, err := slug.Retry(slug.MaxAttempts, isTaken, func(candidate string) (string, error) {
		calls++
		if calls < slug.MaxAttempts {
			return "", errTaken
		}
		return candidate, nil
	})
	require.NoError(t, err)
	assert.Equal(t, slug.MaxAttempts, calls)
	assert.Regexp(t, shape, got)
}

func TestRetry_GivesUpWithTheLastTakenError(t *testing.T) {
	calls := 0
	_, err := slug.Retry(slug.MaxAttempts, isTaken, func(string) (string, error) {
		calls++
		return "", errTaken
	})
	assert.True(t, isTaken(err))
	assert.Equal(t, slug.MaxAttempts, calls)
}

func TestRetry_StopsOnAnyOtherError(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	_, err := slug.Retry(slug.MaxAttempts, isTaken, func(string) (string, error) {
		calls++
		return "", boom
	})
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 1, calls)
}

func TestRetry_TriesAtLeastOnce(t *testing.T) {
	calls := 0
	_, err := slug.Retry(0, isTaken, func(c string) (string, error) {
		calls++
		return c, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestIsValid(t *testing.T) {
	assert.True(t, slug.IsValid(slug.Generate()))
	for _, bad := range []string{
		"", "wispy-frost", "wispy-frost-48", "wispy-frost-48211",
		"wispy_frost_4821", "nope-nope-1234", "wispy-frost-0123",
	} {
		assert.False(t, slug.IsValid(bad), "IsValid(%q)", bad)
	}
}
