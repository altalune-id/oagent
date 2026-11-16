package agent

import (
	"slices"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/kernel/webpolicy"
)

// The web tool limits; a zero TimeoutMs or MaxCallsPerTurn runs with the recommended value.
const (
	RecommendedWebTimeoutMs       = 15000
	MinWebTimeoutMs               = 1000
	MaxWebTimeoutMs               = 30000
	RecommendedWebMaxCallsPerTurn = 10
	MaxWebMaxCallsPerTurn         = 50
	MaxWebHosts                   = 50
	MaxWebRequestHeaders          = 20
)

// Web is an agent's built-in web tools: which are on, where they may reach, and their shared limits.
type Web struct {
	Search            bool     `json:"search,omitempty"`
	Fetch             bool     `json:"fetch,omitempty"`
	HTTP              bool     `json:"http,omitempty"`
	FetchAllowDomains []string `json:"fetch_allow_domains,omitempty"`
	HTTPHosts         []string `json:"http_hosts,omitempty"`
	RequestHeaders    []string `json:"request_headers,omitempty"`
	TimeoutMs         int32    `json:"timeout_ms,omitempty"`
	MaxCallsPerTurn   int32    `json:"max_calls_per_turn,omitempty"`
}

// On reports whether any web tool is on.
func (w Web) On() bool { return w.Search || w.Fetch || w.HTTP }

// EffectiveTimeoutMs is the per-call timeout a run uses: TimeoutMs, or the recommended value when unset.
func (w Web) EffectiveTimeoutMs() int32 {
	if w.TimeoutMs > 0 {
		return w.TimeoutMs
	}
	return RecommendedWebTimeoutMs
}

// EffectiveMaxCallsPerTurn is the web call cap a turn uses: MaxCallsPerTurn, or the recommended value when unset.
func (w Web) EffectiveMaxCallsPerTurn() int32 {
	if w.MaxCallsPerTurn > 0 {
		return w.MaxCallsPerTurn
	}
	return RecommendedWebMaxCallsPerTurn
}

func (w Web) validate(invalid func(string, apperror.FieldReason) error) error {
	switch {
	case w.TimeoutMs != 0 && (w.TimeoutMs < MinWebTimeoutMs || w.TimeoutMs > MaxWebTimeoutMs):
		return invalid("web.timeout_ms", apperror.FieldInvalid)
	case w.MaxCallsPerTurn < 0 || w.MaxCallsPerTurn > MaxWebMaxCallsPerTurn:
		return invalid("web.max_calls_per_turn", apperror.FieldInvalid)
	case len(w.FetchAllowDomains) > MaxWebHosts:
		return invalid("web.fetch_allow_domains", apperror.FieldTooMany)
	case !allHosts(w.FetchAllowDomains):
		return invalid("web.fetch_allow_domains", apperror.FieldInvalid)
	case len(w.HTTPHosts) > MaxWebHosts:
		return invalid("web.http_hosts", apperror.FieldTooMany)
	case !allHosts(w.HTTPHosts):
		return invalid("web.http_hosts", apperror.FieldInvalid)
	case len(w.RequestHeaders) > MaxWebRequestHeaders:
		return invalid("web.request_headers", apperror.FieldTooMany)
	}
	for _, h := range w.RequestHeaders {
		// SECURITY: the model never sets a reserved header or one only a project secret may own.
		if !webpolicy.ValidHeaderName(h) || webpolicy.Reserved(h) || webpolicy.ModelRefused(h) {
			return invalid("web.request_headers", apperror.FieldInvalid)
		}
	}
	return nil
}

func allHosts(hosts []string) bool {
	return !slices.ContainsFunc(hosts, func(h string) bool { return !webpolicy.ValidHost(h) })
}

func (w Web) equal(o Web) bool {
	return w.Search == o.Search && w.Fetch == o.Fetch && w.HTTP == o.HTTP &&
		slices.Equal(w.FetchAllowDomains, o.FetchAllowDomains) && slices.Equal(w.HTTPHosts, o.HTTPHosts) &&
		slices.Equal(w.RequestHeaders, o.RequestHeaders) && w.TimeoutMs == o.TimeoutMs && w.MaxCallsPerTurn == o.MaxCallsPerTurn
}

func webToProto(w Web) *agentv1.WebTools {
	if w.equal(Web{}) {
		return nil
	}
	return &agentv1.WebTools{
		Search: w.Search, Fetch: w.Fetch, Http: w.HTTP, FetchAllowDomains: w.FetchAllowDomains, HttpHosts: w.HTTPHosts,
		RequestHeaders: w.RequestHeaders, TimeoutMs: w.TimeoutMs, MaxCallsPerTurn: w.MaxCallsPerTurn,
	}
}

func webFromProto(w *agentv1.WebTools) Web {
	return Web{
		Search: w.GetSearch(), Fetch: w.GetFetch(), HTTP: w.GetHttp(), FetchAllowDomains: nilIfEmpty(w.GetFetchAllowDomains()),
		HTTPHosts: nilIfEmpty(w.GetHttpHosts()), RequestHeaders: nilIfEmpty(w.GetRequestHeaders()),
		TimeoutMs: w.GetTimeoutMs(), MaxCallsPerTurn: w.GetMaxCallsPerTurn(),
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
