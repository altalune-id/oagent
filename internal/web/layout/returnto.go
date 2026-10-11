package layout

import (
	"net/url"
	"strings"
)

// SanitizeReturnTo returns raw when it is a same-origin path under basePath, else "". SECURITY: blocks open redirects.
func SanitizeReturnTo(basePath, raw string) string {
	u, err := url.Parse(strings.ReplaceAll(raw, `\`, "/"))
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.User != nil {
		return ""
	}
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	if bp := strings.TrimRight(basePath, "/"); bp != "" && u.Path != bp && !strings.HasPrefix(u.Path, bp+"/") {
		return ""
	}
	out := u.EscapedPath()
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}
	return out
}

// ResolveReturnTo returns the sanitized raw, or the console when raw is unsafe or empty.
func ResolveReturnTo(basePath, raw string) string {
	if s := SanitizeReturnTo(basePath, raw); s != "" {
		return s
	}
	return Path(basePath, "/console") + "/"
}
