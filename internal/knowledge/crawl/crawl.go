package crawl

import (
	"bytes"
	"cmp"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"altalune.id/oagent/httpclient"
)

// The crawl caps.
const (
	MaxPages        = 50
	MaxDepth        = 2
	MaxPageBytes    = 2 << 20
	PageTimeout     = 15 * time.Second
	PerHostInterval = time.Second
	maxRedirects    = 5
)

// MaxSitemaps is how many distinct sitemaps one crawl reads; MaxCrawlDuration bounds the whole crawl.
const (
	MaxSitemaps      = 5
	MaxCrawlDuration = 30 * time.Minute
)

// Config bounds one crawl.
type Config struct {
	MaxPages          int
	MaxDepth          int
	MaxPageBytes      int
	PageTimeout       time.Duration
	PerHostInterval   time.Duration
	AllowPrivateHosts bool
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	c.MaxPages = cmp.Or(c.MaxPages, d.MaxPages)
	c.MaxDepth = cmp.Or(c.MaxDepth, d.MaxDepth)
	c.MaxPageBytes = cmp.Or(c.MaxPageBytes, d.MaxPageBytes)
	c.PageTimeout = cmp.Or(c.PageTimeout, d.PageTimeout)
	c.PerHostInterval = cmp.Or(c.PerHostInterval, d.PerHostInterval)
	return c
}

// DefaultConfig returns the source crawl caps.
func DefaultConfig() Config {
	return Config{
		MaxPages: MaxPages, MaxDepth: MaxDepth, MaxPageBytes: MaxPageBytes,
		PageTimeout: PageTimeout, PerHostInterval: PerHostInterval,
	}
}

// Page is one fetched page.
type Page struct {
	URL         string
	ContentType string
	Body        []byte
}

// InvalidSeedError reports that the seed URL is not an https URL with a host.
type InvalidSeedError struct{ URL string }

func (e *InvalidSeedError) Error() string { return "crawl: invalid seed url" }

// IsInvalidSeedError reports whether err's tree contains an *InvalidSeedError.
func IsInvalidSeedError(err error) bool {
	_, ok := errors.AsType[*InvalidSeedError](err)
	return ok
}

// OffHostRedirectError reports that the seed redirects to another host; the source should use that host's address.
type OffHostRedirectError struct{ Host string }

func (e *OffHostRedirectError) Error() string {
	return "crawl: the seed redirects to another host: " + e.Host
}

// AsOffHostRedirectError returns the *OffHostRedirectError in err's tree, if any.
func AsOffHostRedirectError(err error) (*OffHostRedirectError, bool) {
	return errors.AsType[*OffHostRedirectError](err)
}

var (
	errOffHost         = errors.New("crawl: redirect leaves the seed host or robots.txt")
	errRobotsRedirects = errors.New("crawl: robots.txt redirects too often")
	errRobotsDowngrade = errors.New("crawl: robots.txt redirects to a non-https URL")
	errRobotsPort      = errors.New("crawl: robots.txt redirects to a port other than 443")
)

// Crawler fetches a seed's pages on the seed's host only.
type Crawler struct {
	Client *http.Client
	Config Config
}

// New returns a Crawler on the SSRF-guarded httpclient.
func New(cfg Config) *Crawler {
	cfg = cfg.withDefaults()
	client := httpclient.New(
		httpclient.WithAllowPrivateHosts(cfg.AllowPrivateHosts),
		httpclient.WithoutProxy(),
		httpclient.WithTimeout(cfg.PageTimeout),
	)
	return &Crawler{Client: client, Config: cfg}
}

type queued struct {
	url   string
	depth int
}

type run struct {
	cfg     Config
	host    string
	client  *http.Client
	next    time.Time
	robots  Rules
	visited []string
	seen    map[string]bool
	fetches int
}

// Crawl fetches the seed, then same-host https links breadth first, and calls visit for each 2xx page.
func (c *Crawler) Crawl(ctx context.Context, seed string, visit func(Page) error) ([]string, error) {
	cfg := c.Config.withDefaults()
	start, ok := normalize(seed, nil)
	// SECURITY: a public crawl dials only the https port; every later URL must share the seed's host and port.
	if !ok || (start.Port() != "" && !cfg.AllowPrivateHosts) {
		return nil, &InvalidSeedError{URL: seed}
	}
	ctx, cancel := context.WithTimeout(ctx, MaxCrawlDuration)
	defer cancel()
	r := &run{cfg: cfg, host: hostOf(start), seen: map[string]bool{}}
	robotsClient := *c.Client
	robotsClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errRobotsDowngrade
		}
		if port := req.URL.Port(); port != "" && port != "443" && !cfg.AllowPrivateHosts {
			return errRobotsPort
		}
		if len(via) > maxRedirects {
			return errRobotsRedirects
		}
		return r.wait(req.Context())
	}
	r.client = &robotsClient
	robots := r.fetch(ctx, start.Scheme+"://"+start.Host+"/robots.txt")
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	r.robots = RulesFromFetch(robots.status, robots.body, robots.err)

	client := *c.Client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects || req.URL.Scheme != "https" {
			return errOffHost
		}
		if host := hostOf(req.URL); host != r.host {
			return &OffHostRedirectError{Host: host}
		}
		if !r.robots.Allows(req.URL.RequestURI()) {
			return errOffHost
		}
		return r.wait(req.Context())
	}
	r.client = &client

	queue := []queued{{url: start.String()}}
	r.seen[start.String()] = true
	for _, u := range r.sitemap(ctx, robots) {
		queue = append(queue, queued{url: u, depth: 1})
	}
	for len(queue) > 0 && len(r.visited) < r.cfg.MaxPages && r.budgetLeft() {
		item := queue[0]
		queue = queue[1:]
		found, err := r.page(ctx, item, visit)
		if err != nil {
			return r.visited, err
		}
		queue = append(queue, found...)
	}
	return r.visited, nil
}

func (r *run) page(ctx context.Context, item queued, visit func(Page) error) ([]queued, error) {
	u, _ := url.Parse(item.url)
	if !r.robots.Allows(u.RequestURI()) {
		return nil, nil
	}
	res := r.fetch(ctx, item.url)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e, ok := AsOffHostRedirectError(res.err); ok && item.depth == 0 {
		return nil, e
	}
	if !res.ok() {
		return nil, nil
	}
	final := res.final
	if final != item.url {
		if r.seen[final] || slices.Contains(r.visited, final) {
			return nil, nil
		}
		r.seen[final] = true
		u, _ = url.Parse(final)
	}
	r.visited = append(r.visited, final)
	if err := visit(Page{URL: final, ContentType: res.ctype, Body: res.body}); err != nil {
		return nil, err
	}
	if item.depth >= r.cfg.MaxDepth || !isHTML(res.ctype) {
		return nil, nil
	}
	var out []queued
	for _, link := range pageLinks(u, res.body, r.cfg.MaxPages) {
		if r.seen[link] {
			continue
		}
		r.seen[link] = true
		out = append(out, queued{url: link, depth: item.depth + 1})
	}
	return out, nil
}

func (r *run) budgetLeft() bool { return r.fetches < 2*r.cfg.MaxPages }

func (r *run) wait(ctx context.Context) error {
	if d := time.Until(r.next); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	r.next = time.Now().Add(r.cfg.PerHostInterval)
	return nil
}

type fetched struct {
	status int
	body   []byte
	ctype  string
	final  string
	err    error
}

func (f fetched) ok() bool { return f.err == nil && f.status >= 200 && f.status <= 299 }

var errPageTooLarge = errors.New("crawl: page over the size cap")

func (r *run) fetch(ctx context.Context, target string) fetched {
	r.fetches++
	if err := r.wait(ctx); err != nil {
		return fetched{err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, r.cfg.PageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fetched{err: err}
	}
	req.Header.Set("User-Agent", RobotsAgent)
	resp, err := r.client.Do(req)
	if err != nil {
		return fetched{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(r.cfg.MaxPageBytes)+1))
	if err == nil && len(body) > r.cfg.MaxPageBytes {
		err = errPageTooLarge
	}
	if err != nil {
		return fetched{status: resp.StatusCode, err: err}
	}
	final := target
	if fu, ok := normalize(resp.Request.URL.String(), nil); ok {
		final = fu.String()
	}
	return fetched{status: resp.StatusCode, body: body, ctype: resp.Header.Get("Content-Type"), final: final}
}

type urlset struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

func (r *run) sitemap(ctx context.Context, robots fetched) []string {
	maps := []string{}
	if robots.ok() {
		for line := range strings.SplitSeq(string(robots.body), "\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.EqualFold(strings.TrimSpace(key), "sitemap") {
				maps = append(maps, strings.TrimSpace(value))
			}
		}
	}
	if len(maps) == 0 {
		maps = append(maps, "https://"+r.host+"/sitemap.xml")
	}
	var out []string
	read := map[string]bool{}
	for _, m := range maps {
		mu, ok := normalize(m, nil)
		if !ok || hostOf(mu) != r.host || !r.robots.Allows(mu.RequestURI()) || read[mu.String()] {
			continue
		}
		if len(read) == MaxSitemaps || !r.budgetLeft() {
			break
		}
		read[mu.String()] = true
		res := r.fetch(ctx, mu.String())
		if !res.ok() {
			continue
		}
		var set urlset
		if xml.NewDecoder(bytes.NewReader(res.body)).Decode(&set) != nil {
			continue
		}
		for _, loc := range set.URLs {
			lu, ok := normalize(strings.TrimSpace(loc.Loc), nil)
			if !ok || hostOf(lu) != r.host || r.seen[lu.String()] {
				continue
			}
			r.seen[lu.String()] = true
			out = append(out, lu.String())
			if len(out) == r.cfg.MaxPages {
				return out
			}
		}
	}
	return out
}

func normalize(raw string, base *url.URL) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, false
	}
	u.Fragment = ""
	u.RawFragment = ""
	u.Host = strings.ToLower(u.Host)
	if u.Port() == "443" {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, true
}

func hostOf(u *url.URL) string {
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if port := u.Port(); port != "" && port != "443" {
		return host + ":" + port
	}
	return host
}

func isHTML(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	return err == nil && (media == "text/html" || media == "application/xhtml+xml")
}

// SECURITY: a streaming tokenizer reads the anchors without building a tree, so a hostile page costs bounded memory.
func pageLinks(base *url.URL, body []byte, limit int) []string {
	host := hostOf(base)
	seen := map[string]bool{}
	var out []string
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			name, more := z.TagName()
			if atom.Lookup(name) != atom.A {
				continue
			}
			for more {
				var key, val []byte
				key, val, more = z.TagAttr()
				if string(key) != "href" {
					continue
				}
				if u, ok := normalize(strings.TrimSpace(string(val)), base); ok && hostOf(u) == host && !seen[u.String()] {
					seen[u.String()] = true
					if out = append(out, u.String()); len(out) == limit {
						return out
					}
				}
			}
		}
	}
}

// Vanished lists the pages of the previous crawl that the current one no longer found.
func Vanished(previous, current []string) []string {
	var out []string
	for _, p := range previous {
		if !slices.Contains(current, p) {
			out = append(out, p)
		}
	}
	return out
}
