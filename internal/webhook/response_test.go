package webhook

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadReceipt(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"X-Trace": {"abc"}, "Set-Cookie": {"secret=1"}},
		Body:       io.NopCloser(strings.NewReader("hello")),
	}
	rc := readReceipt(resp)
	assert.Equal(t, 200, rc.status)
	assert.Equal(t, "hello", rc.body)
	assert.False(t, rc.truncated)
	assert.Equal(t, []Header{{Name: "X-Trace", Value: "abc"}}, rc.headers) // cookie dropped
}

func TestReadReceiptTruncates(t *testing.T) {
	big := strings.Repeat("a", MaxResponseBodyBytes+10)
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(big))}
	rc := readReceipt(resp)
	assert.True(t, rc.truncated)
	assert.Len(t, rc.body, MaxResponseBodyBytes)
}

func TestBoundResponse(t *testing.T) {
	a := BoundResponse(Attempt{
		ResponseBody:    "ok\x00",
		ResponseHeaders: []Header{{Name: "X-Auth-Token", Value: "x"}, {Name: "Server", Value: "nginx"}},
	})
	assert.NotContains(t, a.ResponseBody, "\x00")
	assert.Equal(t, []Header{{Name: "Server", Value: "nginx"}}, a.ResponseHeaders)
}

func TestBoundResponseBodyCap(t *testing.T) {
	a := BoundResponse(Attempt{ResponseBody: strings.Repeat("a", MaxResponseBodyBytes+1)})
	assert.True(t, a.ResponseTruncated)
	assert.Len(t, a.ResponseBody, MaxResponseBodyBytes)
}

func TestBoundResponseHeaderCap(t *testing.T) {
	hs := make([]Header, MaxResponseHeaders+5)
	for i := range hs {
		hs[i] = Header{Name: "H", Value: "v"}
	}
	a := BoundResponse(Attempt{ResponseHeaders: hs})
	assert.Len(t, a.ResponseHeaders, MaxResponseHeaders)
}

func TestStorableText(t *testing.T) {
	s, trimmed := storableText("abc", 10)
	assert.Equal(t, "abc", s)
	assert.False(t, trimmed)

	s, trimmed = storableText("abcdef", 3)
	assert.Equal(t, "abc", s)
	assert.True(t, trimmed)
}

func TestIsCredentialHeader(t *testing.T) {
	assert.True(t, isCredentialHeader("Authorization"))
	assert.True(t, isCredentialHeader("X-API-Key"))
	assert.False(t, isCredentialHeader("Content-Type"))
}
