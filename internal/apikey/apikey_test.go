package apikey

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/scope"
)

func future() *time.Time {
	t := time.Now().UTC().Add(time.Hour)
	return &t
}

func TestScheme_MintProject(t *testing.T) {
	k, plaintext, err := NewScheme("oagent_").MintProject(uuid.New(), uuid.New(), "ci", []string{scope.AgentRead}, nil, future(), time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, KindProject, k.Kind)
	assert.NotEqual(t, uuid.Nil, k.ProjectID)
	assert.True(t, strings.HasPrefix(plaintext, "oagent_"))
	assert.True(t, k.Matches(plaintext))
	assert.False(t, k.Matches(plaintext+"x"))
	assert.Len(t, k.SecretHash, 32)
}

func TestScheme_MintProject_OrgScopeRejected(t *testing.T) {
	_, _, err := NewScheme("").MintProject(uuid.New(), uuid.New(), "ci", []string{scope.OrgWrite}, nil, future(), time.Now().UTC())
	assert.IsType(t, &ScopeLevelError{}, err)
}

func TestScheme_MintOrg(t *testing.T) {
	k, plaintext, err := NewScheme("").MintOrg(uuid.New(), "org", []string{scope.OrgRead}, ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, KindOrg, k.Kind)
	assert.True(t, k.AllProjects)
	assert.True(t, k.HasGrant())
	assert.True(t, k.Matches(plaintext))
}

func TestScheme_MintOrg_GrantErrors(t *testing.T) {
	_, _, err := NewScheme("").MintOrg(uuid.New(), "org", nil, ProjectGrant{}, future(), time.Now().UTC())
	assert.IsType(t, &EmptyGrantError{}, err)

	_, _, err = NewScheme("").MintOrg(uuid.New(), "org", nil, ProjectGrant{All: true, ProjectIDs: []uuid.UUID{uuid.New()}}, future(), time.Now().UTC())
	assert.IsType(t, &GrantConflictError{}, err)
}

func TestScheme_Mint_Validation(t *testing.T) {
	now := time.Now().UTC()
	_, _, err := NewScheme("").MintOrg(uuid.New(), "", nil, ProjectGrant{All: true}, future(), now)
	assert.IsType(t, &InvalidNameError{}, err)

	_, _, err = NewScheme("").MintOrg(uuid.New(), "x", []string{"nope:read"}, ProjectGrant{All: true}, future(), now)
	assert.IsType(t, &UnknownScopeError{}, err)

	for _, s := range []string{scope.MemberWrite, scope.ApikeyWrite} {
		_, _, err = NewScheme("").MintOrg(uuid.New(), "x", []string{s}, ProjectGrant{All: true}, future(), now)
		assert.True(t, IsPersonOnlyScopeError(err), s)
	}

	_, _, err = NewScheme("").MintOrg(uuid.New(), "x", nil, ProjectGrant{All: true}, nil, now)
	assert.IsType(t, &ExpiryRequiredError{}, err)

	past := now.Add(-time.Hour)
	_, _, err = NewScheme("").MintOrg(uuid.New(), "x", nil, ProjectGrant{All: true}, &past, now)
	assert.IsType(t, &ExpiryInPastError{}, err)

	tooLong := now.Add(400 * 24 * time.Hour)
	_, _, err = NewScheme("").MintOrg(uuid.New(), "x", nil, ProjectGrant{All: true}, &tooLong, now)
	assert.IsType(t, &ExpiryTooLongError{}, err)
}

func TestKey_UsableRevoke(t *testing.T) {
	k, _, err := NewScheme("").MintOrg(uuid.New(), "x", nil, ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	now := time.Now().UTC()
	assert.True(t, k.Usable(now))
	k.Revoke(now)
	assert.False(t, k.Usable(now))
	assert.Equal(t, now, *k.RevokedAt)
}

func TestKey_ExpiredUnusable(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	k := &APIKey{ExpiresAt: &past}
	assert.False(t, k.Usable(time.Now().UTC()))
}

func TestReachableProjects(t *testing.T) {
	pid := uuid.New()
	assert.Equal(t, []uuid.UUID{pid}, (&APIKey{Kind: KindProject, ProjectID: pid}).ReachableProjects())

	oid := uuid.New()
	assert.Equal(t, []uuid.UUID{oid}, (&APIKey{Kind: KindOrg, ProjectIDs: []uuid.UUID{oid}}).ReachableProjects())
}

func TestValidateName(t *testing.T) {
	assert.NoError(t, ValidateName("ci"))
	assert.IsType(t, &InvalidNameError{}, ValidateName(""))
	assert.IsType(t, &InvalidNameError{}, ValidateName(strings.Repeat("a", 101)))
}

func TestDedupe(t *testing.T) {
	id := uuid.New()
	out := dedupe([]uuid.UUID{uuid.Nil, id, id, uuid.New(), uuid.Nil})
	require.Len(t, out, 2)
	assert.Equal(t, id, out[0])
}
