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
