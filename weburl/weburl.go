// Package weburl checks the outbound https URLs tenants configure.
package weburl

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxLen bounds a URL, in bytes.
const MaxLen = 2048

// Reason is the stable cause a URL is refused.
type Reason string

// The causes a URL is refused.
const (
	Required       Reason = "required"
	TooLong        Reason = "too_long"
	NotUTF8        Reason = "not_utf8"
	Unparsable     Reason = "unparsable"
	HTTPSRequired  Reason = "https_required"
	HasCredentials Reason = "has_credentials"
	NoHost         Reason = "no_host"
)

// Check trims raw and returns it with an empty Reason, or "" and the first rule it breaks; allowHTTP also accepts http.
func Check(raw string, allowHTTP bool) (string, Reason) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", Required
	}
	if len(raw) > MaxLen {
		return "", TooLong
	}
	if !utf8.ValidString(raw) {
		return "", NotUTF8
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", Unparsable
	}
	if u.Scheme != "https" && (!allowHTTP || u.Scheme != "http") {
		return "", HTTPSRequired
	}
	if u.User != nil {
		return "", HasCredentials
	}
	if u.Hostname() == "" {
		return "", NoHost
	}
	return raw, ""
}

// Text completes the sentence "URL ..." for the reason.
func (r Reason) Text() string {
	switch r {
	case Required:
		return "is required"
	case TooLong:
		return "is longer than " + strconv.Itoa(MaxLen) + " characters"
	case NotUTF8:
		return "is not valid UTF-8"
	case Unparsable:
		return "cannot be parsed"
	case HTTPSRequired:
		return "must use https"
	case HasCredentials:
		return "must not carry credentials"
	case NoHost:
		return "has no host"
	default:
		return "is invalid"
	}
}
