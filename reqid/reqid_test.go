package reqid_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/reqid"
)

func TestNew(t *testing.T) {
	a, b := reqid.New(), reqid.New()
	assert.True(t, nanoid.IsValid(a, reqid.IDLen))
	assert.NotEqual(t, a, b)
}

func TestContext(t *testing.T) {
	assert.Empty(t, reqid.FromContext(context.Background()))

	ctx := reqid.WithContext(context.Background(), "test-id")
	assert.Equal(t, "test-id", reqid.FromContext(ctx))
}

func TestEnsure(t *testing.T) {
	ctx, id := reqid.Ensure(reqid.WithContext(context.Background(), "existing"))
	assert.Equal(t, "existing", id)
	assert.Equal(t, "existing", reqid.FromContext(ctx))

	ctx, id = reqid.Ensure(context.Background())
	assert.True(t, nanoid.IsValid(id, reqid.IDLen))
	assert.Equal(t, id, reqid.FromContext(ctx))
}

func TestSanitize(t *testing.T) {
	long := strings.Repeat("a", reqid.MaxLength+1)
	atCap := strings.Repeat("a", reqid.MaxLength)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"uuid v7", "0192a3f1-c7c1-7c1d-b1d1-abcdef012345", "0192a3f1-c7c1-7c1d-b1d1-abcdef012345"},
		{"traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{"underscore dot colon", "svc_a.edge:1", "svc_a.edge:1"},
		{"at the length cap", atCap, atCap},
		{"empty", "", ""},
		{"oversized", long, ""},
		{"space", "abc def", ""},
		{"newline injection", "abc\n{\"level\":\"ERROR\"}", ""},
		{"json breakout", `abc","forged":"yes`, ""},
		{"non ascii", "abcédef", ""},
		{"del byte", "abc\x7f", ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reqid.Sanitize(tt.in))

			h := http.Header{}
			h.Set(reqid.Header, tt.in)
			assert.Equal(t, tt.want, reqid.FromHeader(h))
		})
	}
}

func TestFromHTTPHeader(t *testing.T) {
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)
	r.Header.Set(reqid.Header, "hdr-id")
	assert.Equal(t, "hdr-id", reqid.FromHTTPHeader(r))

	r.Header.Set(reqid.Header, "bad id with spaces")
	assert.Empty(t, reqid.FromHTTPHeader(r))
}
