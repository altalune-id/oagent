package storage

import (
	"errors"
	"strings"
)

// Prefix is an object key prefix at or below one owner's folder; only OwnerPrefix and Sub build one.
type Prefix struct{ value string }

// String returns the prefix with its trailing "/".
func (p Prefix) String() string { return p.value }

// IsZero reports whether p was never built; drivers refuse it.
func (p Prefix) IsZero() bool { return p.value == "" }

// Sub returns the folder seg inside p.
func (p Prefix) Sub(seg string) (Prefix, error) {
	if p.IsZero() || !validSegment(seg) {
		return Prefix{}, &InvalidPrefixError{Prefix: p.value + seg}
	}
	return Prefix{value: p.value + seg + "/"}, nil
}

// InvalidPrefixError signals a prefix that could reach beyond one owner's folder.
type InvalidPrefixError struct{ Prefix string }

func (e *InvalidPrefixError) Error() string { return "storage: invalid key prefix " + e.Prefix }

// IsInvalidPrefixError reports whether err's tree contains an *InvalidPrefixError.
func IsInvalidPrefixError(err error) bool {
	_, ok := errors.AsType[*InvalidPrefixError](err)
	return ok
}

// OwnerPrefix is the folder {visibility}/{app}/{owner kind}/{owner public id}/{purpose}/ that Key puts files in.
func OwnerPrefix(app string, p Purpose, ownerPublicID string) (Prefix, error) {
	segs := []string{string(p.Visibility), app, string(p.OwnerKind), ownerPublicID, p.Name}
	raw := strings.Join(segs, "/") + "/"
	for _, seg := range segs {
		if !validSegment(seg) {
			return Prefix{}, &InvalidPrefixError{Prefix: raw}
		}
	}
	return Prefix{value: raw}, nil
}

func validSegment(seg string) bool {
	return seg != "" && seg != "." && seg != ".." && !strings.Contains(seg, "/")
}
