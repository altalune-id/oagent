package mcpserver

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validInput() Input {
	return Input{Name: "yasaku", URL: "https://yasaku.example.com/mcp", AuthKind: AuthBearer, TimeoutMS: 5000}
}

func TestNew(t *testing.T) {
	orgID, projectID := uuid.New(), uuid.New()
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	in := validInput()
	in.Name = "  yasaku "
	in.Trusted = true

	s, err := New(orgID, projectID, in, now)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, s.ID)
	assert.Len(t, s.PublicID, 21)
	assert.Equal(t, orgID, s.OrgID)
	assert.Equal(t, projectID, s.ProjectID)
	assert.Equal(t, "yasaku", s.Name)
	assert.Equal(t, "https://yasaku.example.com/mcp", s.URL)
	assert.Equal(t, AuthBearer, s.AuthKind)
	assert.Empty(t, s.AuthHeader)
	assert.Equal(t, int32(5000), s.TimeoutMS)
	assert.True(t, s.Trusted)
	assert.Equal(t, now, s.CreatedAt)
	assert.Equal(t, now, s.UpdatedAt)
	assert.Empty(t, s.AuthSealed)
}

func TestNew_InvalidURL(t *testing.T) {
	for raw, reason := range map[string]URLReason{
		"": URLRequired,
		"https://e.com/" + strings.Repeat("a", MaxURLLen): URLTooLong,
		"https://e.com/\xff":                              URLNotUTF8,
		"https://e.com/%zz":                               URLUnparsable,
		"http://e.com/mcp":                                URLHTTPSRequired,
		"https://u:p@e.com/mcp":                           URLHasCredentials,
		"https:///mcp":                                    URLNoHost,
	} {
		in := validInput()
		in.URL = raw
		s, err := New(uuid.New(), uuid.New(), in, time.Now())
		assert.Nil(t, s)
		e, ok := errors.AsType[*InvalidURLError](err)
		require.True(t, ok, "%q: %v", raw, err)
		assert.Equal(t, reason, e.Reason, raw)
	}
}

func TestNew_AllowInsecureURL(t *testing.T) {
	in := validInput()
	in.URL = "http://127.0.0.1:5190/mcp"
	_, err := New(uuid.New(), uuid.New(), in, time.Now(), AllowInsecureURL())
	require.NoError(t, err)
}

func TestNew_HeaderAuth(t *testing.T) {
	in := validInput()
	in.AuthKind = AuthHeader
	in.AuthHeader = " X-Api-Key "
	s, err := New(uuid.New(), uuid.New(), in, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "X-Api-Key", s.AuthHeader)
}

func TestNew_HeaderNameRefused(t *testing.T) {
	for _, name := range []string{
		"", "  ", "X Api", "X-Api:", "X-Ä",
		"Host", "content-length", "Transfer-Encoding", "CONNECTION", "Content-Type", "accept", "Mcp-Session-Id",
		"mcp-protocol-version", "TE", "Trailer", "Upgrade", "Keep-Alive", "Proxy-Authorization", "Proxy-Connection", "Expect",
	} {
		in := validInput()
		in.AuthKind = AuthHeader
		in.AuthHeader = name
		_, err := New(uuid.New(), uuid.New(), in, time.Now())
		e, ok := errors.AsType[*SecretRequiredError](err)
		require.True(t, ok, "%q: %v", name, err)
		assert.Equal(t, SecretHeaderName, e.Reason, name)
	}
}

func TestNew_HeaderNameOnlyForHeaderAuth(t *testing.T) {
	for _, kind := range []AuthKind{AuthNone, AuthBearer} {
		in := validInput()
		in.AuthKind = kind
		in.AuthHeader = "X-Api-Key"
		s, err := New(uuid.New(), uuid.New(), in, time.Now())
		require.NoError(t, err)
		assert.Empty(t, s.AuthHeader, kind)
	}
}

func TestUpdate(t *testing.T) {
	created := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	s, err := New(uuid.New(), uuid.New(), validInput(), created)
	require.NoError(t, err)
	id, publicID := s.ID, s.PublicID
	s.AuthSealed = []byte{1}

	in := validInput()
	in.TimeoutMS = 9000
	in.Trusted = true
	later := created.Add(time.Hour)
	changed, err := s.Update(in, later)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, int32(9000), s.TimeoutMS)
	assert.True(t, s.Trusted)
	assert.Equal(t, later, s.UpdatedAt)
	assert.Equal(t, created, s.CreatedAt)
	assert.Equal(t, []byte{1}, s.AuthSealed)
	assert.Equal(t, id, s.ID)
	assert.Equal(t, publicID, s.PublicID)

	in.URL = "https://other.example.com/mcp"
	changed, err = s.Update(in, later)
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestUpdate_NameImmutable(t *testing.T) {
	s, err := New(uuid.New(), uuid.New(), validInput(), time.Now())
	require.NoError(t, err)
	in := validInput()
	in.Name = "renamed"
	_, err = s.Update(in, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "yasaku", s.Name, "the name is the tool prefix and never changes")
}

func TestUpdate_InvalidLeavesServerUnchanged(t *testing.T) {
	s, err := New(uuid.New(), uuid.New(), validInput(), time.Now())
	require.NoError(t, err)
	before := *s
	in := validInput()
	in.URL = "http://e.com"
	in.TimeoutMS = 9000
	_, err = s.Update(in, time.Now())
	assert.True(t, IsInvalidURLError(err))
	assert.Equal(t, before, *s)
}

func TestReservedHeaders(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"Host", "Content-Length", "Transfer-Encoding", "Connection", "Content-Type", "Accept", "Mcp-Session-Id",
		"MCP-Protocol-Version", "TE", "Trailer", "Upgrade", "Keep-Alive", "Proxy-Authorization", "Proxy-Connection", "Expect",
	}, ReservedHeaders)
}

func TestUpdate_ToNoneClearsTheSecretAndHeader(t *testing.T) {
	in := validInput()
	in.AuthKind = AuthHeader
	in.AuthHeader = "X-Api-Key"
	s, err := New(uuid.New(), uuid.New(), in, time.Now())
	require.NoError(t, err)
	s.AuthSealed = []byte{1}

	in.AuthKind = AuthNone
	_, err = s.Update(in, time.Now())
	require.NoError(t, err)
	assert.Nil(t, s.AuthSealed)
	assert.Empty(t, s.AuthHeader, "a header name sent with none is dropped")
}

func TestUpdate_HeaderNameDroppedForBearer(t *testing.T) {
	s, err := New(uuid.New(), uuid.New(), validInput(), time.Now())
	require.NoError(t, err)
	s.AuthSealed = []byte{1}
	in := validInput()
	in.AuthHeader = "X-Api-Key"
	_, err = s.Update(in, time.Now())
	require.NoError(t, err)
	assert.Empty(t, s.AuthHeader)
	assert.Equal(t, []byte{1}, s.AuthSealed, "bearer keeps its secret")
}
