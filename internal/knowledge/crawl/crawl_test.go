package crawl_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge/crawl"
)

type route struct {
	status   int
	body     string
	location string
	ctype    string
}

type site struct {
	mu       sync.Mutex
	routes   map[string]route
	requests []string
	agents   []string
	times    []time.Time
}

func (s *site) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req.URL.String())
	s.agents = append(s.agents, req.Header.Get("User-Agent"))
	s.times = append(s.times, time.Now())
	r, ok := s.routes[req.URL.String()]
	s.mu.Unlock()
	if !ok {
		r = route{status: http.StatusNotFound}
	}
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.ctype == "" {
		r.ctype = "text/html; charset=utf-8"
	}
	h := http.Header{"Content-Type": {r.ctype}}
	if r.location != "" {
		h.Set("Location", r.location)
	}
	return &http.Response{StatusCode: r.status, Header: h, Body: io.NopCloser(strings.NewReader(r.body)), Request: req}, nil
}

func (s *site) fetched() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{}
	for _, u := range s.requests {
		if !strings.HasSuffix(u, "/robots.txt") && !strings.HasSuffix(u, "/sitemap.xml") {
			out = append(out, u)
		}
	}
	return out
}

func links(hrefs ...string) string {
	var b strings.Builder
	b.WriteString("<html><body><p>text</p>")
	for _, h := range hrefs {
		fmt.Fprintf(&b, `<a href="%s">l</a>`, h)
	}
	b.WriteString("</body></html>")
	return b.String()
}

func crawler(s *site, over func(*crawl.Config)) *crawl.Crawler {
	cfg := crawl.DefaultConfig()
	cfg.PerHostInterval = time.Nanosecond
	if over != nil {
		over(&cfg)
	}
	return &crawl.Crawler{Client: &http.Client{Transport: s}, Config: cfg}
}

func run(t *testing.T, c *crawl.Crawler, seed string) ([]string, []crawl.Page) {
	t.Helper()
	var pages []crawl.Page
	visited, err := c.Crawl(context.Background(), seed, func(p crawl.Page) error {
		pages = append(pages, p)
		return nil
	})
	require.NoError(t, err)
	return visited, pages
}

func TestCrawl_SameHostOnly(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/":  {body: links("/a", "https://docs.example.com/b#frag", "https://evil.example.net/x", "http://docs.example.com/c", "https://other.docs.example.com/d", "mailto:a@b.c")},
		"https://docs.example.com/a": {body: links()},
		"https://docs.example.com/b": {body: links()},
	}}
	visited, pages := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/a", "https://docs.example.com/b"}, visited)
	assert.Equal(t, visited, s.fetched())
	assert.Len(t, pages, 3)
	for _, ua := range s.agents {
		assert.Equal(t, crawl.RobotsAgent, ua)
	}
}

func TestCrawl_PageAndDepthCaps(t *testing.T) {
	routes := map[string]route{
		"https://docs.example.com/":   {body: links("/d1")},
		"https://docs.example.com/d1": {body: links("/d2")},
		"https://docs.example.com/d2": {body: links("/d3")},
		"https://docs.example.com/d3": {body: links()},
	}
	s := &site{routes: routes}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/d1", "https://docs.example.com/d2"}, visited, "depth 2 from the seed")

	hrefs := make([]string, 0, 70)
	wide := map[string]route{}
	for i := range 70 {
		hrefs = append(hrefs, fmt.Sprintf("/p%d", i))
		wide[fmt.Sprintf("https://docs.example.com/p%d", i)] = route{body: links()}
	}
	wide["https://docs.example.com/"] = route{body: links(hrefs...)}
	s = &site{routes: wide}
	visited, _ = run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Len(t, visited, crawl.MaxPages)
	assert.Len(t, s.fetched(), crawl.MaxPages)
}

func TestCrawl_RedirectToPrivateRefused(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/":   {body: links("/r1", "/r2", "/r3")},
		"https://docs.example.com/r1": {status: http.StatusFound, location: "https://10.0.0.1/admin"},
		"https://docs.example.com/r2": {status: http.StatusFound, location: "https://evil.example.net/"},
		"https://docs.example.com/r3": {status: http.StatusFound, location: "/a"},
		"https://docs.example.com/a":  {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/a"}, visited)
	for _, u := range s.requests {
		assert.NotContains(t, u, "10.0.0.1")
		assert.NotContains(t, u, "evil.example.net")
	}
}

func TestCrawl_RespectsRobots(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {ctype: "text/plain", body: "User-agent: *\nDisallow: /private\n"},
		"https://docs.example.com/":           {body: links("/private/x", "/public")},
		"https://docs.example.com/public":     {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/public"}, visited)
}

func TestCrawl_Robots5xxStopsTheCrawl(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {status: http.StatusServiceUnavailable},
		"https://docs.example.com/":           {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Empty(t, visited)
}

func TestCrawl_ReadsTheSitemap(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {ctype: "text/plain", body: "Sitemap: https://docs.example.com/map.xml\n"},
		"https://docs.example.com/map.xml":    {ctype: "application/xml", body: `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://docs.example.com/hidden</loc></url><url><loc>https://evil.example.net/x</loc></url></urlset>`},
		"https://docs.example.com/":           {body: links()},
		"https://docs.example.com/hidden":     {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/hidden"}, visited)
}

func TestCrawl_SkipsOversizePagesAndKeepsGoing(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/":    {body: links("/big", "/ok")},
		"https://docs.example.com/big": {body: strings.Repeat("x", crawl.MaxPageBytes+1)},
		"https://docs.example.com/ok":  {body: links()},
	}}
	visited, pages := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/ok"}, visited)
	for _, p := range pages {
		assert.LessOrEqual(t, len(p.Body), crawl.MaxPageBytes)
	}
}

func TestCrawl_NonHTMLPagesAreVisitedButNotParsed(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/":         {body: links("/guide.md")},
		"https://docs.example.com/guide.md": {ctype: "text/markdown", body: "[x](/never)"},
	}}
	_, pages := run(t, crawler(s, nil), "https://docs.example.com/")
	require.Len(t, pages, 2)
	assert.Equal(t, "text/markdown", pages[1].ContentType)
	assert.Equal(t, []byte("[x](/never)"), pages[1].Body)
}

func TestCrawl_RefusesABadSeed(t *testing.T) {
	for _, seed := range []string{"http://docs.example.com/", "ftp://docs.example.com/", "https:///nohost", "::bad"} {
		_, err := crawler(&site{}, nil).Crawl(context.Background(), seed, func(crawl.Page) error { return nil })
		assert.True(t, crawl.IsInvalidSeedError(err), "%s: %v", seed, err)
	}
}

func TestCrawl_VisitErrorStops(t *testing.T) {
	s := &site{routes: map[string]route{"https://docs.example.com/": {body: links("/a")}, "https://docs.example.com/a": {body: links()}}}
	boom := fmt.Errorf("store down")
	_, err := crawler(s, nil).Crawl(context.Background(), "https://docs.example.com/", func(crawl.Page) error { return boom })
	assert.ErrorIs(t, err, boom)
}

func TestCrawl_RateOneRPS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &site{routes: map[string]route{
			"https://docs.example.com/":  {body: links("/a", "/b")},
			"https://docs.example.com/a": {body: links()},
			"https://docs.example.com/b": {body: links()},
		}}
		c := crawler(s, func(cfg *crawl.Config) { cfg.PerHostInterval = time.Second })
		_, err := c.Crawl(context.Background(), "https://docs.example.com/", func(crawl.Page) error { return nil })
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(s.times), 4)
		for i := 1; i < len(s.times); i++ {
			assert.GreaterOrEqual(t, s.times[i].Sub(s.times[i-1]), time.Second, "request %d", i)
		}
	})
}

func TestCrawl_CancelStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &site{routes: map[string]route{"https://docs.example.com/": {body: links("/a")}, "https://docs.example.com/a": {body: links()}}}
		c := crawler(s, func(cfg *crawl.Config) { cfg.PerHostInterval = time.Hour })
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(time.Minute)
			cancel()
		}()
		_, err := c.Crawl(ctx, "https://docs.example.com/", func(crawl.Page) error { return nil })
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRefresh_DeletesVanishedPages(t *testing.T) {
	previous := []string{"https://docs.example.com/", "https://docs.example.com/old", "https://docs.example.com/kept"}
	current := []string{"https://docs.example.com/", "https://docs.example.com/kept", "https://docs.example.com/new"}
	assert.Equal(t, []string{"https://docs.example.com/old"}, crawl.Vanished(previous, current))
}

func TestNew_UsesTheGuardedClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 10))
	}))
	defer srv.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/", http.NoBody)
	require.NoError(t, err)
	resp, err := crawl.New(crawl.DefaultConfig()).Client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err, "the SSRF guard refuses a loopback host")
}

func TestCrawl_ManyLinksOnOnePageStayFast(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := range 45_000 {
		fmt.Fprintf(&b, `<a href="/p%d">l</a><a href="/p%d">d</a>`, i, i)
	}
	b.WriteString("</body></html>")
	s := &site{routes: map[string]route{"https://docs.example.com/": {body: b.String()}}}
	start := time.Now()
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Less(t, b.Len(), crawl.MaxPageBytes)
	assert.Len(t, s.fetched(), 1+crawl.MaxPages, "links past the page cap are never queued")
	assert.Len(t, visited, 1, "only the seed exists on this fake site")
}

func TestCrawl_RobotsCheckedOnRedirectTargets(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {ctype: "text/plain", body: "User-agent: *\nDisallow: /private\n"},
		"https://docs.example.com/":           {body: links("/a")},
		"https://docs.example.com/a":          {status: http.StatusFound, location: "/private/x"},
		"https://docs.example.com/private/x":  {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/"}, visited)
	assert.NotContains(t, s.requests, "https://docs.example.com/private/x")
}

func TestCrawl_RobotsFollowsUpToFiveRedirects(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {status: http.StatusMovedPermanently, location: "https://www.example.com/robots.txt"},
		"https://www.example.com/robots.txt":  {ctype: "text/plain", body: "User-agent: *\nDisallow: /private\n"},
		"https://docs.example.com/":           {body: links("/private/x", "/ok")},
		"https://docs.example.com/ok":         {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/ok"}, visited, "the redirected robots.txt applies")
}

func TestCrawl_RobotsPastFiveRedirectsDisallowsAll(t *testing.T) {
	routes := map[string]route{
		"https://docs.example.com/":          {body: links("/private/x")},
		"https://docs.example.com/private/x": {body: links()},
	}
	prev := "https://docs.example.com/robots.txt"
	for i := range 6 {
		next := fmt.Sprintf("https://docs.example.com/r%d/robots.txt", i)
		routes[prev] = route{status: http.StatusFound, location: next}
		prev = next
	}
	routes[prev] = route{ctype: "text/plain", body: "User-agent: *\nDisallow: /\n"}
	s := &site{routes: routes}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Empty(t, visited, "past 5 hops robots.txt counts as unreachable: disallow all")
	assert.Empty(t, s.fetched())
}

func TestCrawl_ZeroConfigGetsTheDefaults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &site{routes: map[string]route{
			"https://docs.example.com/":  {body: links("/a")},
			"https://docs.example.com/a": {body: links()},
		}}
		c := &crawl.Crawler{Client: &http.Client{Transport: s}}
		visited, err := c.Crawl(context.Background(), "https://docs.example.com/", func(crawl.Page) error { return nil })
		require.NoError(t, err)
		assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/a"}, visited)
		assert.GreaterOrEqual(t, s.times[len(s.times)-1].Sub(s.times[0]), 3*time.Second, "the default 1 s pacing applies")
	})
	assert.Equal(t, crawl.DefaultConfig(), crawl.New(crawl.Config{}).Config)
}

func TestCrawl_HTTPSeedMakesNoRequest(t *testing.T) {
	s := &site{}
	_, err := crawler(s, nil).Crawl(context.Background(), "http://docs.example.com/", func(crawl.Page) error { return nil })
	assert.True(t, crawl.IsInvalidSeedError(err))
	assert.Empty(t, s.requests)
}

func TestCrawl_RobotsDowngradeToHTTPDisallowsAll(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {status: http.StatusMovedPermanently, location: "http://docs.example.com/robots.txt"},
		"https://docs.example.com/":           {body: links()},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Empty(t, visited, "an https to http robots redirect counts as unreachable")
	for _, u := range s.requests {
		assert.False(t, strings.HasPrefix(u, "http://"), "http is never fetched: %s", u)
	}
}

func redirectChain(n int) map[string]route {
	routes := map[string]route{"https://docs.example.com/": {body: links("/hop0")}}
	for i := range n {
		routes[fmt.Sprintf("https://docs.example.com/hop%d", i)] = route{status: http.StatusFound, location: fmt.Sprintf("/hop%d", i+1)}
	}
	routes[fmt.Sprintf("https://docs.example.com/hop%d", n)] = route{body: links()}
	return routes
}

func TestCrawl_PageFollowsFiveRedirectsNotSix(t *testing.T) {
	five := &site{routes: redirectChain(5)}
	visited, _ := run(t, crawler(five, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/hop5"}, visited)
	six := &site{routes: redirectChain(6)}
	visited, _ = run(t, crawler(six, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/"}, visited)
}

func TestCrawl_OffHostSeedRedirectExplained(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://example.com/robots.txt": {ctype: "text/plain", body: "User-agent: *\nDisallow: /private\n"},
		"https://example.com/":           {status: http.StatusMovedPermanently, location: "https://www.example.com/private"},
		"https://example.com/deep":       {status: http.StatusFound, location: "https://elsewhere.example.net/"},
		"https://www.example.com/":       {body: links()},
	}}
	visited, err := crawler(s, nil).Crawl(context.Background(), "https://example.com/", func(crawl.Page) error { return nil })
	e, ok := crawl.AsOffHostRedirectError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, "www.example.com", e.Host, "named even when the target path is disallowed here")
	assert.Empty(t, visited)
	for _, u := range s.requests {
		assert.NotContains(t, u, "www.example.com", "the off-host target is named, never fetched")
	}

	s.routes["https://example.com/"] = route{body: links("/deep", "/a")}
	s.routes["https://example.com/a"] = route{body: links()}
	visited, err = crawler(s, nil).Crawl(context.Background(), "https://example.com/", func(crawl.Page) error { return nil })
	require.NoError(t, err, "a deeper page that leaves the host is skipped, not explained")
	assert.Equal(t, []string{"https://example.com/", "https://example.com/a"}, visited, "the crawl goes on past it")
	for _, u := range s.requests {
		assert.NotContains(t, u, "elsewhere.example.net", "the off-host target gets no request")
	}
}

func TestCrawl_FetchBudgetCountsFailures(t *testing.T) {
	hrefs, deep := make([]string, 0, crawl.MaxPages), make([]string, 0, crawl.MaxPages)
	for i := range crawl.MaxPages - 1 {
		hrefs = append(hrefs, fmt.Sprintf("/gone%d", i))
		deep = append(deep, fmt.Sprintf("/deep%d", i))
	}
	deep = append(deep, "/deep-last")
	s := &site{routes: map[string]route{
		"https://docs.example.com/":    {body: links(append(hrefs, "/hub")...)},
		"https://docs.example.com/hub": {body: links(deep...)},
	}}
	visited, _ := run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, []string{"https://docs.example.com/", "https://docs.example.com/hub"}, visited)
	assert.LessOrEqual(t, len(s.requests), 2*crawl.MaxPages, "404s count toward the fetch budget")
}

func TestCrawl_SitemapEntriesCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for i := range 4 * crawl.MaxPages {
		fmt.Fprintf(&b, "<url><loc>https://docs.example.com/m%d</loc></url>", i)
	}
	b.WriteString("</urlset>")
	s := &site{routes: map[string]route{
		"https://docs.example.com/":            {body: links()},
		"https://docs.example.com/sitemap.xml": {ctype: "application/xml", body: b.String()},
	}}
	run(t, crawler(s, nil), "https://docs.example.com/")
	fromMap := 0
	for _, u := range s.requests {
		if strings.Contains(u, "/m") {
			fromMap++
		}
	}
	assert.LessOrEqual(t, fromMap, crawl.MaxPages)
}

func TestCrawl_SitemapObeysRobots(t *testing.T) {
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {ctype: "text/plain", body: "User-agent: *\nDisallow: /sitemap.xml\n"},
		"https://docs.example.com/":           {body: links()},
	}}
	run(t, crawler(s, nil), "https://docs.example.com/")
	for _, u := range s.requests {
		assert.NotContains(t, u, "sitemap.xml")
	}
}

func TestCrawl_RedirectHopsKeepTheRate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &site{routes: map[string]route{
			"https://docs.example.com/robots.txt":  {status: http.StatusMovedPermanently, location: "/robots2.txt"},
			"https://docs.example.com/robots2.txt": {ctype: "text/plain", body: "User-agent: *\nAllow: /\n"},
			"https://docs.example.com/":            {body: links("/r")},
			"https://docs.example.com/r":           {status: http.StatusFound, location: "/r2"},
			"https://docs.example.com/r2":          {status: http.StatusFound, location: "/a"},
			"https://docs.example.com/a":           {body: links()},
		}}
		c := crawler(s, func(cfg *crawl.Config) { cfg.PerHostInterval = time.Second })
		visited, err := c.Crawl(context.Background(), "https://docs.example.com/", func(crawl.Page) error { return nil })
		require.NoError(t, err)
		assert.Contains(t, visited, "https://docs.example.com/a")
		for i := 1; i < len(s.times); i++ {
			assert.GreaterOrEqual(t, s.times[i].Sub(s.times[i-1]), time.Second, "request %d: %s", i, s.requests[i])
		}
	})
}

func (s *site) count(suffix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, u := range s.requests {
		if strings.HasSuffix(u, suffix) {
			n++
		}
	}
	return n
}

func TestCrawl_ReadsAtMostFiveDistinctSitemaps(t *testing.T) {
	var robots strings.Builder
	for i := range 8 {
		fmt.Fprintf(&robots, "Sitemap: https://docs.example.com/m%d.xml\nSitemap: https://docs.example.com/m%d.xml\n", i, i)
	}
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {ctype: "text/plain", body: robots.String()},
		"https://docs.example.com/":           {body: links()},
	}}
	run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Equal(t, crawl.MaxSitemaps, s.count(".xml"))
}

func TestCrawl_SitemapsSpendTheFetchBudget(t *testing.T) {
	var robots strings.Builder
	for i := range 5 {
		fmt.Fprintf(&robots, "Sitemap: https://docs.example.com/m%d.xml\n", i)
	}
	s := &site{routes: map[string]route{
		"https://docs.example.com/robots.txt": {ctype: "text/plain", body: robots.String()},
		"https://docs.example.com/":           {body: links()},
	}}
	run(t, crawler(s, func(cfg *crawl.Config) { cfg.MaxPages = 1 }), "https://docs.example.com/")
	assert.LessOrEqual(t, len(s.requests), 2, "robots.txt and the sitemaps count against the 2×MaxPages fetch budget")
}

func TestCrawl_StopsAtItsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		routes := map[string]route{}
		for i := range crawl.MaxPages {
			routes[fmt.Sprintf("https://docs.example.com/p%d", i)] = route{body: links(fmt.Sprintf("/p%d", i+1))}
		}
		routes["https://docs.example.com/"] = route{body: links("/p0")}
		c := crawler(&site{routes: routes}, func(cfg *crawl.Config) { cfg.PerHostInterval = time.Minute; cfg.MaxDepth = crawl.MaxPages + 1 })
		start := time.Now()
		_, err := c.Crawl(context.Background(), "https://docs.example.com/", func(crawl.Page) error { return nil })
		require.ErrorIs(t, err, context.DeadlineExceeded, "a crawl cut short fails, so its unreached pages are not deleted as vanished")
		assert.Equal(t, crawl.MaxCrawlDuration, time.Since(start).Truncate(time.Minute))
	})
}

// SECURITY: a public crawl only dials the https port, so it cannot be pointed at another service on a host.
func TestCrawl_OnlyPort443UnlessPrivateHostsAreAllowed(t *testing.T) {
	routes := map[string]route{"https://docs.example.com:8443/": {body: links()}, "https://docs.example.com/": {body: links()}}
	s := &site{routes: routes}
	_, err := crawler(s, nil).Crawl(context.Background(), "https://docs.example.com:8443/", func(crawl.Page) error { return nil })
	assert.True(t, crawl.IsInvalidSeedError(err), "%v", err)
	assert.Empty(t, s.requests)

	visited, _ := run(t, crawler(s, nil), "https://docs.example.com:443/")
	assert.Equal(t, []string{"https://docs.example.com/"}, visited, "443 is the https port")
	visited, _ = run(t, crawler(s, func(cfg *crawl.Config) { cfg.AllowPrivateHosts = true }), "https://docs.example.com:8443/")
	assert.Equal(t, []string{"https://docs.example.com:8443/"}, visited)
}

func TestPageLinks_FindsSameHostAnchorsInDocumentOrder(t *testing.T) {
	base, err := url.Parse("https://docs.example.com/guide/")
	require.NoError(t, err)
	page := `<!doctype html><html><head><title>T</title><link rel="stylesheet" href="/s.css"></head><body>
<nav><a href="/a">A</a> <a href="b">B</a></nav>
<p>Text <a href=" /c#top ">C</a> <A HREF="/d">D</A> <a>no href</a> <a href="/a">dup</a></p>
<script>var s = '<a href="/in-script">x</a>';</script>
<!-- <a href="/in-comment">x</a> -->
<a href="https://evil.example.net/x">off</a> <a href="http://docs.example.com/plain">http</a>
<a href="https://docs.example.com:8443/port">port</a> <a href="mailto:x@e.com">mail</a>
<div><span><a href="../up">nested</a></span></div>
<a href="/e">E</a></body></html>`
	assert.Equal(t, []string{
		"https://docs.example.com/a", "https://docs.example.com/guide/b", "https://docs.example.com/c",
		"https://docs.example.com/d", "https://docs.example.com/up", "https://docs.example.com/e",
	}, crawl.PageLinks(base, []byte(page), 50))
	assert.Equal(t, []string{"https://docs.example.com/a", "https://docs.example.com/guide/b"}, crawl.PageLinks(base, []byte(page), 2), "stops at the limit")
	assert.Empty(t, crawl.PageLinks(base, []byte("not html at all"), 50))
}

// SECURITY: robots.txt may move to another host, but every hop keeps to the https port.
func TestCrawl_RobotsRedirectKeepsToPort443(t *testing.T) {
	routes := map[string]route{
		"https://docs.example.com/robots.txt":     {status: http.StatusMovedPermanently, location: "https://cdn.example.com:8443/robots.txt"},
		"https://cdn.example.com:8443/robots.txt": {ctype: "text/plain", body: "User-agent: *\nAllow: /\n"},
		"https://docs.example.com/":               {body: links()},
	}
	s := &site{routes: routes}
	run(t, crawler(s, nil), "https://docs.example.com/")
	assert.Zero(t, s.count(":8443/robots.txt"), "the redirect to another port is not followed")

	s = &site{routes: routes}
	run(t, crawler(s, func(cfg *crawl.Config) { cfg.AllowPrivateHosts = true }), "https://docs.example.com/")
	assert.Equal(t, 1, s.count(":8443/robots.txt"), "allowed when private hosts are")
}
