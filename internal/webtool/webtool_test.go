package webtool

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	webtoolv1 "altalune.id/oagent/gen/go/webtool/v1"
	"altalune.id/oagent/sealer"
)

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

func TestSecretAAD_BindsHostAndHeader(t *testing.T) {
	sl := testSealer(t)
	org, project := uuid.New(), uuid.New()
	sealed, err := sealSecret(sl, org, project, "api.example.com", "x-api-key", "v1")
	require.NoError(t, err)

	got, err := openSecret(sl, org, project, "api.example.com", "x-api-key", sealed)
	require.NoError(t, err)
	assert.Equal(t, "v1", got)

	for name, open := range map[string]func() (string, error){
		"another host": func() (string, error) { return openSecret(sl, org, project, "evil.example.com", "x-api-key", sealed) },
		"another header": func() (string, error) {
			return openSecret(sl, org, project, "api.example.com", "authorization", sealed)
		},
		"another project": func() (string, error) { return openSecret(sl, org, uuid.New(), "api.example.com", "x-api-key", sealed) },
		"another org": func() (string, error) {
			return openSecret(sl, uuid.New(), project, "api.example.com", "x-api-key", sealed)
		},
	} {
		_, err := open()
		assert.Error(t, err, name)
	}
}

func TestSearchKeyAAD_BindsProject(t *testing.T) {
	sl := testSealer(t)
	org, project := uuid.New(), uuid.New()
	sealed, err := sealSearchKey(sl, org, project, "k")
	require.NoError(t, err)
	got, err := openSearchKey(sl, org, project, sealed)
	require.NoError(t, err)
	assert.Equal(t, "k", got)
	_, err = openSearchKey(sl, org, uuid.New(), sealed)
	assert.Error(t, err)
	_, err = openSearchKey(sl, uuid.New(), project, sealed)
	assert.Error(t, err)
}

func TestSealing_WithoutAKeyIsUnavailable(t *testing.T) {
	_, err := sealSearchKey(sealer.Disabled(), uuid.New(), uuid.New(), "k")
	assert.True(t, IsSecretsUnavailableError(err))
	_, err = openSecret(sealer.Disabled(), uuid.New(), uuid.New(), "a.example", "x", []byte{1})
	assert.True(t, IsSecretsUnavailableError(err))
}

func TestNewHTTPSecret(t *testing.T) {
	org, project, now := uuid.New(), uuid.New(), time.Now()
	s, err := NewHTTPSecret(org, project, "api.example.com", "X-Api-Key", now)
	require.NoError(t, err)
	assert.Equal(t, "x-api-key", s.Header)
	assert.Len(t, s.PublicID, 21)
	assert.NotEqual(t, uuid.Nil, s.ID)

	_, err = NewHTTPSecret(org, project, "api.example.com", "Authorization", now)
	assert.NoError(t, err, "a secret may own Authorization")
}

func TestNewHTTPSecret_ReservedHeaderRefused(t *testing.T) {
	for _, h := range []string{"Host", "cookie", "X-Forwarded-For", "Proxy-Authorization"} {
		_, err := NewHTTPSecret(uuid.New(), uuid.New(), "api.example.com", h, time.Now())
		assertInvalid(t, err, "header", h)
	}
}

func TestNewHTTPSecret_InvalidHostRefused(t *testing.T) {
	for _, h := range []string{"1.2.3.4", "localhost", "intranet", "api.example.com:443", "*.example.com", "API.example.com"} {
		_, err := NewHTTPSecret(uuid.New(), uuid.New(), h, "x-api-key", time.Now())
		assertInvalid(t, err, "host", h)
	}
}

func assertInvalid(t *testing.T, err error, field, input string) {
	t.Helper()
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, input)
	assert.Equal(t, apperror.CodeValidation, ae.Code(), input)
	assert.Equal(t, field, ae.Meta()["field"], input)
	assert.Equal(t, string(apperror.FieldInvalid), ae.Meta()["reason"], input)
}

func TestKinds(t *testing.T) {
	assert.Equal(t, []Kind{KindBrave, KindExa, KindTavily}, Kinds())
	assert.Equal(t, []Kind{KindBrave, KindExa}, AvailableKinds())
	assert.True(t, KindExa.Available())
	assert.False(t, KindTavily.Available())
	assert.False(t, Kind("bing").Available())
}

func TestKinds_ProtoMapping(t *testing.T) {
	for _, k := range Kinds() {
		p := kindToProto(k)
		assert.NotEqual(t, webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_UNSPECIFIED, p, k)
		assert.Equal(t, k, kindOf(p), k)
	}
	assert.Equal(t, webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_UNSPECIFIED, kindToProto("bing"))
	assert.Equal(t, Kind(""), kindOf(webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_UNSPECIFIED))
	assert.Len(t, webtoolv1.WebSearchProviderKind_name, len(Kinds())+1, "every proto kind has a domain kind")
}

func TestErrors_ToAppError(t *testing.T) {
	for _, tc := range []struct {
		err  interface{ ToAppError() *apperror.AppError }
		code apperror.Code
		meta map[string]string
	}{
		{&ProviderUnavailableError{Kind: KindTavily}, apperror.CodeWebSearchProviderUnavailable, map[string]string{"kind": "tavily"}},
		{&SecretRequiredError{}, apperror.CodeWebSearchSecretRequired, nil},
		{&SecretsUnavailableError{Cause: &sealer.UnavailableError{}}, apperror.CodeWebSecretsUnavailable, nil},
		{&NotFoundError{}, apperror.CodeHTTPSecretNotFound, nil},
		{&SecretLimitError{}, apperror.CodeHTTPSecretLimitReached, map[string]string{"max": "50"}},
	} {
		ae := tc.err.ToAppError()
		assert.Equal(t, tc.code, ae.Code())
		if tc.meta != nil {
			assert.Equal(t, tc.meta, ae.Meta())
		}
		assert.True(t, IsExpectedError(tc.err.(error)), "%T", tc.err)
		assert.NotEmpty(t, tc.err.(error).Error())
	}
	assert.False(t, IsExpectedError(assert.AnError))
}
