package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrincipalContext_RoundTrip(t *testing.T) {
	p := Principal{UserID: uuid.New(), Source: SourceLocal}
	assert.Equal(t, p.UserID, PrincipalFrom(PrincipalInto(context.Background(), p)).UserID)
}

func TestPrincipalFrom_EmptyWhenMissing(t *testing.T) {
	assert.Equal(t, Principal{}, PrincipalFrom(context.Background()))
}

func TestPrincipal_JSONDropsAuthority(t *testing.T) {
	p := Principal{
		UserID: uuid.New(), Source: SourceLocal, Email: "a@example.com",
		Scopes: []string{"*"}, Operator: true, AllProjects: true,
		ProjectIDs: []uuid.UUID{uuid.New()}, ResourceIDs: []uuid.UUID{uuid.New()},
	}
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	var back Principal
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, p.UserID, back.UserID)
	assert.Equal(t, p.Email, back.Email)
	assert.Empty(t, back.Scopes)
	assert.False(t, back.Operator)
	assert.False(t, back.AllProjects)
	assert.Empty(t, back.ProjectIDs)
	assert.Empty(t, back.ResourceIDs)
}

func TestPrincipal_IsPerson(t *testing.T) {
	assert.True(t, Principal{Source: SourceLocal, UserID: uuid.New()}.IsPerson())
	assert.False(t, Principal{Source: SourceBasic}.IsPerson())
	assert.False(t, Principal{Source: SourceAPIKey, KeyID: uuid.New()}.IsPerson())
	assert.False(t, Principal{}.IsPerson())
}

func TestSystemPrincipal(t *testing.T) {
	p := System()
	assert.Equal(t, SourceSystem, p.Source)
	assert.Equal(t, "stdio", p.Name)
	assert.True(t, p.Operator)
	assert.False(t, p.IsPerson())
}

func TestSIDIntoAndFrom(t *testing.T) {
	sid, ok := SIDFrom(context.Background())
	assert.False(t, ok)
	assert.Empty(t, sid)
	sid, ok = SIDFrom(SIDInto(context.Background(), "sid-1"))
	assert.True(t, ok)
	assert.Equal(t, "sid-1", sid)
	_, ok = SIDFrom(SIDInto(context.Background(), ""))
	assert.False(t, ok, "an empty sid is no sid")
}
