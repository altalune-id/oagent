package run

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"altalune.id/oagent/internal/kernel/runtime"
)

const (
	toolWebSearch   = "web_search"
	toolWebFetch    = "web_fetch"
	toolHTTPRequest = "http_request"
)

type openedSearch struct {
	provider WebSearchProvider
	err      error
}

func (s *Service) searchProvider(ctx context.Context, t *turn) (WebSearchProvider, error) {
	if t.search == nil {
		sp, err := s.webTools.SearchProvider(ctx, t.run.ProjectID)
		t.search = &openedSearch{provider: sp, err: err}
	}
	return t.search.provider, t.search.err
}

func (s *Service) pinWebSearch(ctx context.Context, t *turn, p *Prices) *Prices {
	if p == nil {
		return nil
	}
	pinned := *p
	if t.version.Web.Search && s.webTools != nil {
		if sp, err := s.searchProvider(ctx, t); err == nil {
			pinned.WebSearch = &sp.PricePerCallMicros
		}
	}
	return &pinned
}

// SECURITY: secrets go only for the version's own hosts; an unresolvable tool is left out, and a store error fails the turn.
func (s *Service) web(ctx context.Context, t *turn, req *runtime.TurnRequest) error {
	w := t.version.Web
	if !w.On() || s.webTools == nil {
		return nil
	}
	spec := runtime.WebSpec{TimeoutMs: w.TimeoutMs, MaxCallsPerTurn: w.MaxCalls}
	if w.Search {
		spec.Search = s.webSearch(ctx, t)
	}
	if w.Fetch && s.helperUsable(t) {
		spec.Fetch = &runtime.WebFetchSpec{AllowDomains: lower(w.FetchAllowDomains)}
		setHelper(req, t)
	}
	if hosts := lower(w.HTTPHosts); w.HTTP && len(hosts) > 0 {
		http, err := s.webHTTP(ctx, t, hosts, w.RequestHeaders)
		if err != nil {
			return err
		}
		spec.HTTP = http
	}
	if spec.Search == nil && spec.Fetch == nil && spec.HTTP == nil {
		return nil
	}
	req.Web = &spec
	if spec.Search != nil && req.Limits.Budget != nil {
		req.Limits.Budget.WebSearchPrice = t.run.Prices.WebSearch
	}
	maps.Copy(t.grants, WebGrants(spec))
	return nil
}

func (s *Service) webSearch(ctx context.Context, t *turn) *runtime.WebSearchSpec {
	sp, err := s.searchProvider(ctx, t)
	reason := ""
	switch {
	case errors.Is(err, ErrNoSearchProvider):
		reason = "search_provider_missing"
	case errors.Is(err, ErrSearchKeyUnsealable):
		reason = "search_key_unsealable"
	case err != nil:
		reason = "search_provider_error"
	case t.run.Prices != nil && t.run.Prices.WebSearch == nil:
		// NOTE: a provider set after the run opened has no pinned price, and a priced run never takes on calls it cannot price.
		reason = "search_provider_missing"
	}
	if reason != "" {
		s.log.WarnContext(ctx, "run: web search left out", "run", t.run.PublicID, "reason", reason)
		return nil
	}
	return &runtime.WebSearchSpec{Kind: sp.Kind, APIKey: sp.APIKey}
}

func (s *Service) webHTTP(ctx context.Context, t *turn, hosts, headers []string) (*runtime.WebHTTPSpec, error) {
	opened, unsealable, err := s.webTools.Secrets(ctx, t.run.ProjectID, hosts)
	if err != nil {
		return nil, err
	}
	if unsealable > 0 {
		s.log.WarnContext(ctx, "run: http secrets left out", "run", t.run.PublicID, "reason", "http_secret_unsealable", "count", unsealable)
	}
	out := &runtime.WebHTTPSpec{Hosts: hosts, RequestHeaders: orEmpty(headers), Secrets: []runtime.WebSecret{}}
	for _, sec := range opened {
		if slices.Contains(hosts, sec.Host) {
			out.Secrets = append(out.Secrets, runtime.WebSecret{Host: sec.Host, Header: sec.Header, Value: sec.Value})
		}
	}
	return out, nil
}

// NOTE: mirrors extractMode; a priced run never takes on helper tokens it cannot price.
func (s *Service) helperUsable(t *turn) bool {
	switch {
	case t.run.Prices != nil && t.run.Prices.HelperInput == nil:
		s.log.Debug("run: web fetch left out; the helper is unpriced", "run", t.run.PublicID)
		return false
	case separateHelper(t.version) && t.helper == nil:
		s.log.Debug("run: web fetch left out; the helper is unresolved", "run", t.run.PublicID)
		return false
	}
	return true
}

// WebGrants are the fixed grants of the web tools a spec sends. SECURITY: keep in step with the runtime's specs (approval.json).
func WebGrants(spec runtime.WebSpec) map[string]ToolGrant {
	out := map[string]ToolGrant{}
	if spec.Search != nil {
		out[toolWebSearch] = ToolGrant{Name: toolWebSearch}
	}
	if spec.Fetch != nil {
		out[toolWebFetch] = ToolGrant{Name: toolWebFetch}
	}
	if spec.HTTP != nil {
		freeGET, _ := json.Marshal([]map[string]any{{"arg": "method", "values": []string{"GET"}}})
		out[toolHTTPRequest] = ToolGrant{Name: toolHTTPRequest, RequireApproval: true, FreeWhen: freeGET}
	}
	return out
}

func lower(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, strings.ToLower(h))
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
