package layout

import "strings"

// Path joins basePath and sub with a single leading `/` and no trailing `/`.
func Path(basePath, sub string) string {
	bp := strings.TrimRight(basePath, "/")
	s := sub
	if s == "" {
		if bp == "" {
			return "/"
		}
		return bp
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return bp + s
}

// UnderAny reports whether p is one of prefixes or below one of them.
func UnderAny(p string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}
