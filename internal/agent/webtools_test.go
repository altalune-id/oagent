package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
)

func validWeb() Web {
	return Web{
		Search: true, Fetch: true, HTTP: true,
		FetchAllowDomains: []string{"docs.example.com"}, HTTPHosts: []string{"api.example.com"},
		RequestHeaders: []string{"X-Api-Version"}, TimeoutMs: 20000, MaxCallsPerTurn: 5,
	}
}

func webViolation(t *testing.T, w Web) string {
	t.Helper()
	ae, ok := apperror.AsAppError(Config{Web: w}.Validate())
	require.True(t, ok)
	require.Len(t, ae.Violations(), 1)
	return ae.Violations()[0].Field
}

func TestValidate_WebAccepted(t *testing.T) {
	require.NoError(t, Config{Web: validWeb()}.Validate())
	require.NoError(t, Config{Web: Web{TimeoutMs: MinWebTimeoutMs, MaxCallsPerTurn: MaxWebMaxCallsPerTurn}}.Validate())
	require.NoError(t, Config{Web: Web{TimeoutMs: MaxWebTimeoutMs}}.Validate())
}

func TestValidate_WebHeaders(t *testing.T) {
	for _, h := range []string{"Authorization", "authorization", "Cookie", "Host", "X-HTTP-Method-Override", "X Api", ""} {
		w := validWeb()
		w.RequestHeaders = []string{h}
		assert.Equal(t, "config.web.request_headers", webViolation(t, w), "%q", h)
	}
	w := validWeb()
	w.RequestHeaders = make([]string, MaxWebRequestHeaders+1)
	for i := range w.RequestHeaders {
		w.RequestHeaders[i] = "X-H" + strings.Repeat("a", i+1)
	}
	assert.Equal(t, "config.web.request_headers", webViolation(t, w), "too many")
}

func TestValidate_WebHosts(t *testing.T) {
	raw, err := os.ReadFile("../kernel/webpolicy/testdata/hosts.json")
	require.NoError(t, err)
	var vectors []struct {
		Host  string `json:"host"`
		Valid bool   `json:"valid"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	for _, v := range vectors {
		fetch, http := validWeb(), validWeb()
		fetch.FetchAllowDomains, http.HTTPHosts = []string{v.Host}, []string{v.Host}
		if v.Valid {
			assert.NoError(t, Config{Web: fetch}.Validate(), v.Host)
			assert.NoError(t, Config{Web: http}.Validate(), v.Host)
			continue
		}
		assert.Equal(t, "config.web.fetch_allow_domains", webViolation(t, fetch), v.Host)
		assert.Equal(t, "config.web.http_hosts", webViolation(t, http), v.Host)
	}
	w := validWeb()
	w.HTTPHosts = make([]string, MaxWebHosts+1)
	for i := range w.HTTPHosts {
		w.HTTPHosts[i] = "h" + strings.Repeat("a", i+1) + ".example.com"
	}
	assert.Equal(t, "config.web.http_hosts", webViolation(t, w), "too many")
}

func TestValidate_WebLimits(t *testing.T) {
	for _, tc := range []struct {
		web   Web
		field string
	}{
		{Web{TimeoutMs: MinWebTimeoutMs - 1}, "config.web.timeout_ms"},
		{Web{TimeoutMs: MaxWebTimeoutMs + 1}, "config.web.timeout_ms"},
		{Web{TimeoutMs: -1}, "config.web.timeout_ms"},
		{Web{MaxCallsPerTurn: MaxWebMaxCallsPerTurn + 1}, "config.web.max_calls_per_turn"},
		{Web{MaxCallsPerTurn: -1}, "config.web.max_calls_per_turn"},
	} {
		assert.Equal(t, tc.field, webViolation(t, tc.web), "%+v", tc.web)
	}
}

func TestWeb_Effective(t *testing.T) {
	assert.Equal(t, int32(RecommendedWebTimeoutMs), Web{}.EffectiveTimeoutMs())
	assert.Equal(t, int32(RecommendedWebMaxCallsPerTurn), Web{}.EffectiveMaxCallsPerTurn())
	assert.Equal(t, int32(2000), Web{TimeoutMs: 2000}.EffectiveTimeoutMs())
	assert.Equal(t, int32(3), Web{MaxCallsPerTurn: 3}.EffectiveMaxCallsPerTurn())
	assert.False(t, Web{FetchAllowDomains: []string{"a.example"}}.On())
	assert.True(t, Web{HTTP: true}.On())
}

func TestConfigEqual_Web(t *testing.T) {
	base := Config{Web: validWeb()}
	assert.True(t, base.Equal(Config{Web: validWeb()}))
	for name, edit := range map[string]func(*Web){
		"search":   func(w *Web) { w.Search = false },
		"fetch":    func(w *Web) { w.Fetch = false },
		"http":     func(w *Web) { w.HTTP = false },
		"domains":  func(w *Web) { w.FetchAllowDomains = []string{"other.example.com"} },
		"hosts":    func(w *Web) { w.HTTPHosts = nil },
		"headers":  func(w *Web) { w.RequestHeaders = append(w.RequestHeaders, "X-B") },
		"timeout":  func(w *Web) { w.TimeoutMs = 1000 },
		"maxcalls": func(w *Web) { w.MaxCallsPerTurn = 1 },
	} {
		w := validWeb()
		edit(&w)
		assert.False(t, base.Equal(Config{Web: w}), name)
	}
	assert.True(t, Config{}.Equal(Config{Web: Web{FetchAllowDomains: []string{}, HTTPHosts: []string{}}}), "nil and empty lists are equal")
}

func TestConfig_WebJSON(t *testing.T) {
	raw, err := json.Marshal(Config{})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "web", "a config without web tools stores no web key")
	raw, err = json.Marshal(Config{Web: validWeb()})
	require.NoError(t, err)
	var back Config
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, validWeb(), back.Web)
}

func TestAgentConfig_WebRoundTrip(t *testing.T) {
	c := Config{Web: validWeb()}
	assert.Equal(t, c.Web, configFromProto(configToProto(c)).Web)
	assert.Nil(t, configToProto(Config{}).GetWeb(), "no web tools sends no web message")
	assert.Equal(t, Web{}, configFromProto(&agentv1.AgentConfig{}).Web)
}

func fieldRules(t *testing.T, msg protoreflect.MessageDescriptor, name string) *validate.FieldRules {
	t.Helper()
	fd := msg.Fields().ByName(protoreflect.Name(name))
	require.NotNil(t, fd, name)
	rules, ok := proto.GetExtension(fd.Options(), validate.E_Field).(*validate.FieldRules)
	require.True(t, ok, name)
	return rules
}

func TestWebDefaults_PinnedToProtoRules(t *testing.T) {
	msg := (&agentv1.WebTools{}).ProtoReflect().Descriptor()
	assert.Equal(t, uint64(MaxWebHosts), fieldRules(t, msg, "fetch_allow_domains").GetRepeated().GetMaxItems())
	assert.Equal(t, uint64(MaxWebHosts), fieldRules(t, msg, "http_hosts").GetRepeated().GetMaxItems())
	assert.Equal(t, uint64(MaxWebRequestHeaders), fieldRules(t, msg, "request_headers").GetRepeated().GetMaxItems())
	assert.Equal(t, int32(MaxWebMaxCallsPerTurn), fieldRules(t, msg, "max_calls_per_turn").GetInt32().GetLte())
	timeout := fieldRules(t, msg, "timeout_ms").GetCel()
	require.Len(t, timeout, 1)
	assert.Contains(t, timeout[0].GetExpression(), "1000")
	assert.Contains(t, timeout[0].GetExpression(), "30000")
	assert.Equal(t, int32(1000), int32(MinWebTimeoutMs))
	assert.Equal(t, int32(30000), int32(MaxWebTimeoutMs))
}

func TestDefaults_WebFields(t *testing.T) {
	d := defaultsToProto()
	assert.Equal(t, int32(RecommendedWebTimeoutMs), d.GetWebTimeoutMs())
	assert.Equal(t, int32(MaxWebTimeoutMs), d.GetMaxWebTimeoutMs())
	assert.Equal(t, int32(RecommendedWebMaxCallsPerTurn), d.GetWebMaxCallsPerTurn())
	assert.Equal(t, int32(MaxWebMaxCallsPerTurn), d.GetMaxWebMaxCallsPerTurn())
	assert.Equal(t, int32(15000), d.GetWebTimeoutMs())
	assert.Equal(t, int32(10), d.GetWebMaxCallsPerTurn())
}

func TestReadsUntrusted_Web(t *testing.T) {
	assert.False(t, readsUntrusted(Config{}, false))
	assert.True(t, readsUntrusted(Config{Web: Web{Search: true}}, false))
	assert.True(t, readsUntrusted(Config{Web: Web{Fetch: true}}, false))
	assert.True(t, readsUntrusted(Config{Web: Web{HTTP: true}}, false))
	assert.False(t, readsUntrusted(Config{Web: Web{HTTPHosts: []string{"a.example.com"}}}, false), "settings alone are not a tool")
	assert.True(t, readsUntrusted(Config{}, true), "a store that remembers API users still counts")
}
