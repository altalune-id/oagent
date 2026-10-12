package webhook

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/events"
)

func TestNew(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e, err := New(org, project, "https://example.com/hook", " desc ", []events.Type{events.AgentCreated})
	require.NoError(t, err)
	assert.Equal(t, org, e.OrgID)
	assert.Equal(t, project, e.ProjectID)
	assert.Equal(t, "https://example.com/hook", e.URL)
	assert.Equal(t, "desc", e.Description)
	assert.True(t, e.Active)
	assert.NotEmpty(t, e.PublicID)
	assert.True(t, e.Subscribes(events.AgentCreated))
	assert.False(t, e.Subscribes(events.AgentUpdated))
}

func TestNewRejects(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	types := []events.Type{events.AgentCreated}
	long := "https://example.com/" + strings.Repeat("a", MaxURLLen)

	for name, tc := range map[string]struct {
		url    string
		types  []events.Type
		reason string
	}{
		"empty url":      {"", types, string(URLRequired)},
		"long url":       {long, types, string(URLTooLong)},
		"bad utf8 url":   {"https://e.com/\xff", types, string(URLNotUTF8)},
		"unparsable url": {"https://e.com/%zz", types, string(URLUnparsable)},
		"http":           {"http://example.com", types, string(URLHTTPSRequired)},
		"credentials":    {"https://user:pass@example.com", types, string(URLHasCredentials)},
		"no host":        {"https://", types, string(URLNoHost)},
		"no types":       {"https://example.com", nil, string(EventTypesRequired)},
		"unknown type":   {"https://example.com", []events.Type{"nope"}, string(EventTypeUnknown)},
		"twice":          {"https://example.com", []events.Type{events.AgentCreated, events.AgentCreated}, string(EventTypeDuplicate)},
		"ping":           {"https://example.com", []events.Type{events.WebhookPing}, string(EventTypeNotSubscribable)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(org, project, tc.url, "d", tc.types)
			ae, ok := apperror.AsAppError(err)
			require.True(t, ok, "%v carries no code", err)
			assert.Equal(t, tc.reason, ae.Meta()["reason"])
		})
	}

	_, err := New(org, project, "https://example.com", "d", []events.Type{events.AgentCreated, "nope"})
	assert.True(t, IsInvalidEventTypesError(err))
	ae, _ := apperror.AsAppError(err)
	assert.Equal(t, "nope", ae.Meta()["event"])
}

func TestNewAllowsInsecureURL(t *testing.T) {
	_, err := New(uuid.New(), uuid.New(), "http://localhost:8080", "", []events.Type{events.AgentCreated}, AllowInsecureURL())
	require.NoError(t, err)
}

func TestUpdate(t *testing.T) {
	e, err := New(uuid.New(), uuid.New(), "https://example.com", "", []events.Type{events.AgentCreated})
	require.NoError(t, err)

	require.NoError(t, e.Update("https://example.com/v2", "new", []events.Type{events.AgentUpdated}, false))
	assert.Equal(t, "https://example.com/v2", e.URL)
	assert.False(t, e.Active)
	assert.True(t, e.Subscribes(events.AgentUpdated))

	assert.True(t, IsInvalidURLError(e.Update("", "d", []events.Type{events.AgentCreated}, true)))
}

func TestSetSecrets(t *testing.T) {
	e, err := New(uuid.New(), uuid.New(), "https://example.com", "", []events.Type{events.AgentCreated})
	require.NoError(t, err)
	e.SetSecrets(SealedSecrets{Primary: []byte("p")})
	assert.Equal(t, []byte("p"), e.Secrets.Primary)
}

func TestValidateDescriptionTooLong(t *testing.T) {
	long := make([]rune, MaxDescriptionRunes+1)
	for i := range long {
		long[i] = 'a'
	}
	_, err := New(uuid.New(), uuid.New(), "https://example.com", string(long), []events.Type{events.AgentCreated})
	assert.True(t, IsInvalidDescriptionError(err))
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, string(DescriptionTooLong), ae.Meta()["reason"])

	_, err = New(uuid.New(), uuid.New(), "https://example.com", "\xff", []events.Type{events.AgentCreated})
	ae, ok = apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, string(DescriptionNotUTF8), ae.Meta()["reason"])
}

func TestResolveValidateOpts(t *testing.T) {
	assert.True(t, resolveValidateOpts([]ValidateOption{AllowInsecureURL()}).allowInsecureURL)
	assert.False(t, resolveValidateOpts(nil).allowInsecureURL)
}

func TestValidationTimestamps(t *testing.T) {
	e, err := New(uuid.New(), uuid.New(), "https://example.com", "", []events.Type{events.AgentCreated})
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), e.CreatedAt, time.Second)
	assert.NotEqual(t, uuid.Nil, e.ID)
}
