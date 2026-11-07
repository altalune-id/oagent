package enduser

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	orgID, projectID := uuid.New(), uuid.New()
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	e := New(orgID, projectID, now)
	assert.NotEqual(t, uuid.Nil, e.ID)
	assert.Len(t, e.PublicID, 21)
	assert.Equal(t, orgID, e.OrgID)
	assert.Equal(t, projectID, e.ProjectID)
	assert.Equal(t, now, e.CreatedAt)
	assert.Equal(t, now, e.UpdatedAt)
}

func TestIdentity_Validate(t *testing.T) {
	for _, k := range []Kind{KindAPIUser, KindWAPhone, KindWALid, KindWidgetAnon, KindWidgetSub} {
		require.NoError(t, Identity{Kind: k, Value: "x"}.Validate(), k)
	}
	require.NoError(t, Identity{Kind: KindAPIUser, Value: strings.Repeat("é", 256)}.Validate(), "256 characters, counted in runes")
	for name, c := range map[string]struct {
		id     Identity
		reason IdentityReason
	}{
		"unknown kind":   {Identity{Kind: "email", Value: "a@b.c"}, IdentityKind},
		"empty value":    {Identity{Kind: KindAPIUser, Value: ""}, IdentityValue},
		"257 characters": {Identity{Kind: KindAPIUser, Value: strings.Repeat("v", 257)}, IdentityValue},
		"not utf8":       {Identity{Kind: KindAPIUser, Value: "\xff"}, IdentityValue},
	} {
		err := c.id.Validate()
		e, ok := errors.AsType[*InvalidIdentityError](err)
		require.True(t, ok, "%s: %v", name, err)
		assert.Equal(t, c.reason, e.Reason, name)
	}
}

func TestNameOf_KindAndMaskedHintOnly(t *testing.T) {
	for _, c := range []struct {
		id   Identity
		hint string
	}{
		{Identity{Kind: KindWAPhone, Value: "6281234567890"}, "90"},
		{Identity{Kind: KindWALid, Value: "123456789012345"}, "45"},
		{Identity{Kind: KindAPIUser, Value: "key:Kabcdef12"}, "ef12"},
		{Identity{Kind: KindAPIUser, Value: "alice"}, ""},
		{Identity{Kind: KindAPIUser, Value: "key:0d7c4e0e-8f8a-4a43-9b51-2f1b6f0a9c11"}, "9c11"},
	} {
		n := NameOf("", []Identity{c.id})
		assert.Equal(t, c.id.Kind, n.Kind)
		assert.Equal(t, c.hint, n.Hint)
		assert.NotContains(t, n.Label(), c.id.Value)
	}
	assert.Equal(t, Name{}, NameOf("", nil), "no identity, no name")
}

func TestName_LabelPrefersTheDisplayNameWithNoRawKind(t *testing.T) {
	phone := []Identity{{Kind: KindWAPhone, Value: "6281234567890"}}
	assert.Equal(t, "Sari Wulandari", NameOf("Sari Wulandari", phone).Label(), "a person's name is what admins expect")
	assert.Equal(t, "…90", NameOf("", phone).Label(), "else a masked hint, never the raw kind")
	assert.NotContains(t, NameOf("", phone).Label(), "wa_phone")
	assert.Empty(t, NameOf("", []Identity{{Kind: KindAPIUser, Value: "alice"}}).Label())
}

func TestList_AQueryIsABoundParameterNeverStatementText(t *testing.T) {
	text, args := ListSQL("6281234567890")
	assert.NotContains(t, text, "6281234567890", "a database span records the statement text")
	assert.Contains(t, args, "6281234567890")
	assert.Contains(t, args, "%6281234567890%")
}
