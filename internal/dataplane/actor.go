package dataplane

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
)

// UserHeader names the integrator's own end user; only project and org keys may send it.
const UserHeader = "Oagent-User"

var errPersonalKeyUser = apperror.New(apperror.CodeForbidden, "A personal key always speaks as its owner").
	WithMeta(map[string]string{"permission": UserHeader})

// actorValue is the api_user identity the caller speaks as. SECURITY: never taken from the body.
func (h *Handler) actorValue(ctx context.Context, r *http.Request, p session.Principal) (string, error) {
	named := r.Header.Get(UserHeader)
	personal := p.OwnerID != uuid.Nil
	if named != "" {
		if personal {
			return "", errPersonalKeyUser
		}
		if !idPattern.MatchString(named) || reserved(named) {
			return "", invalid(UserHeader, apperror.FieldInvalidFormat)
		}
		return named, nil
	}
	if personal {
		id, err := h.d.Names.UserPublicID(ctx, p.OwnerID)
		if err != nil {
			return "", err
		}
		return "person:" + id, nil
	}
	// SECURITY: the key's id, never its hint: a hint can come back on a new key after a revoke. It stays server-side; labels show only a tail.
	return "key:" + p.KeyID.String(), nil
}

// SECURITY: key: and person: name oagent's own actors; an integrator naming one would speak as that key or member.
func reserved(named string) bool {
	lower := strings.ToLower(named)
	return strings.HasPrefix(lower, "key:") || strings.HasPrefix(lower, "person:")
}
