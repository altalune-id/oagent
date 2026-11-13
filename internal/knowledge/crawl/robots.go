// Package crawl fetches a URL source's pages under robots.txt and the egress guard.
package crawl

import (
	"regexp"
	"slices"
	"strings"
)

// RobotsAgent is the user agent token matched in robots.txt groups.
const RobotsAgent = "oagent"

// MaxRobotsBytes is how much of a robots.txt is read; the rest is ignored.
const MaxRobotsBytes = 512000

type robotsRule struct {
	allow  bool
	length int
	re     *regexp.Regexp
}

// Rules is one host's parsed robots.txt for this crawler.
type Rules struct {
	rules   []robotsRule
	denyAll bool
}

func compileRobotsPattern(pattern string) (*regexp.Regexp, error) {
	anchored := strings.HasSuffix(pattern, "$")
	body := strings.TrimSuffix(pattern, "$")
	parts := strings.Split(body, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	expr := "^" + strings.Join(parts, ".*")
	if anchored {
		expr += "$"
	}
	return regexp.Compile(expr)
}

type robotsGroup struct {
	agents []string
	rules  []robotsRule
}

// ParseRobots reads a robots.txt per RFC 9309 for agent: its own groups, else the `*` groups.
func ParseRobots(text, agent string) Rules {
	var (
		groups  []*robotsGroup
		current *robotsGroup
	)
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch {
		case key == "user-agent":
			if current == nil || len(current.rules) > 0 {
				current = &robotsGroup{}
				groups = append(groups, current)
			}
			current.agents = append(current.agents, strings.ToLower(value))
		case (key == "allow" || key == "disallow") && current != nil && value != "":
			if re, err := compileRobotsPattern(value); err == nil {
				current.rules = append(current.rules, robotsRule{allow: key == "allow", length: len(value), re: re})
			}
		}
	}
	pick := func(name string) (rules []robotsRule, found bool) {
		for _, g := range groups {
			if slices.Contains(g.agents, name) {
				rules = append(rules, g.rules...)
				found = true
			}
		}
		return rules, found
	}
	if own, found := pick(strings.ToLower(agent)); found {
		return Rules{rules: own}
	}
	star, _ := pick("*")
	return Rules{rules: star}
}

// RulesFromFetch turns a robots.txt fetch into rules: 2xx parses, 4xx allows all, anything else disallows all.
func RulesFromFetch(status int, body []byte, err error) Rules {
	switch {
	case err != nil:
		return Rules{denyAll: true}
	case status >= 200 && status < 300:
		if len(body) > MaxRobotsBytes {
			body = body[:MaxRobotsBytes]
		}
		return ParseRobots(string(body), RobotsAgent)
	case status >= 400 && status < 500:
		return Rules{}
	default:
		return Rules{denyAll: true}
	}
}

// Allows reports whether the path (with its query) may be fetched.
func (r Rules) Allows(pathAndQuery string) bool {
	if pathAndQuery == "/robots.txt" {
		return true
	}
	if r.denyAll {
		return false
	}
	var best *robotsRule
	for i := range r.rules {
		rule := &r.rules[i]
		if !rule.re.MatchString(pathAndQuery) {
			continue
		}
		if best == nil || rule.length > best.length || (rule.length == best.length && rule.allow) {
			best = rule
		}
	}
	return best == nil || best.allow
}
