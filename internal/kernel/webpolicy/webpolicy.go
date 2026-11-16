// Package webpolicy holds the host and header rules the web tools share with the runtime.
package webpolicy

import (
	"regexp"
	"slices"
	"strings"
)

// ReservedHeaders are the header names no agent or project secret may set.
var ReservedHeaders = []string{ //nolint:gochecknoglobals // a fixed list the runtime mirrors.
	"Host", "Content-Length", "Transfer-Encoding", "Connection", "Keep-Alive", "TE", "Trailer", "Upgrade", "Expect",
	"Proxy-Authorization", "Proxy-Connection", "Cookie", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host",
	"X-Forwarded-Proto", "X-Real-IP", "X-HTTP-Method-Override", "X-HTTP-Method", "X-Method-Override",
}

// ModelRefusedHeaders are the header names a model may not send, though a project secret may.
var ModelRefusedHeaders = []string{"Authorization"} //nolint:gochecknoglobals // a fixed list the runtime mirrors.

// HeaderNamePattern is the RFC 9110 token a header name must match.
const HeaderNamePattern = "^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,64}$"

var (
	headerNameRe = regexp.MustCompile(HeaderNamePattern)
	labelRe      = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	numericRe    = regexp.MustCompile(`^(\d+|0x[0-9a-f]*)$`)
)

// Reserved reports whether name is a reserved header, ignoring case.
func Reserved(name string) bool { return has(ReservedHeaders, name) }

// ModelRefused reports whether name is a header only a project secret may set, ignoring case.
func ModelRefused(name string) bool { return has(ModelRefusedHeaders, name) }

// ValidHeaderName reports whether name is a header name token of at most 64 bytes.
func ValidHeaderName(name string) bool { return headerNameRe.MatchString(name) }

// ValidHost reports whether h is a lowercase public host name. SECURITY: a numeric last label is refused; URL parsers read it as IPv4.
func ValidHost(h string) bool {
	if len(h) > 253 {
		return false
	}
	labels := strings.Split(h, ".")
	if len(labels) < 2 || numericRe.MatchString(labels[len(labels)-1]) {
		return false
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return false
		}
	}
	return true
}

func has(list []string, name string) bool {
	return slices.ContainsFunc(list, func(h string) bool { return strings.EqualFold(h, name) })
}
