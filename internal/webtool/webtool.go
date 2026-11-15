// Package webtool owns a project's web search provider and the HTTP secrets its agents' web tools use.
package webtool

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	webtoolv1 "altalune.id/oagent/gen/go/webtool/v1"
	"altalune.id/oagent/internal/kernel/webpolicy"
	"altalune.id/oagent/nanoid"
)

// Kind names a search provider's API.
type Kind string

// The search provider kinds; a kind missing from AvailableKinds is listed but refused.
const (
	KindBrave  Kind = "brave"
	KindExa    Kind = "exa"
	KindTavily Kind = "tavily"
)

// Kinds lists every search provider kind, offered or not.
func Kinds() []Kind { return []Kind{KindBrave, KindExa, KindTavily} }

// AvailableKinds lists the kinds this deployment can search with.
func AvailableKinds() []Kind { return []Kind{KindBrave, KindExa} }

// Available reports whether k is a kind this deployment can search with.
func (k Kind) Available() bool { return slices.Contains(AvailableKinds(), k) }

// Provider is a project's web search provider; the API key is sealed.
type Provider struct {
	OrgID, ProjectID   uuid.UUID
	Kind               Kind
	APIKeySealed       []byte
	PricePerCallMicros int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// HTTPSecret is a header value http_request attaches on calls to Host; the value is sealed and Header is lowercase.
type HTTPSecret struct {
	ID               uuid.UUID
	PublicID         string
	OrgID, ProjectID uuid.UUID
	Host, Header     string
	ValueSealed      []byte
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NewHTTPSecret checks host and header and mints a secret for them; the value is sealed by the service.
func NewHTTPSecret(orgID, projectID uuid.UUID, host, header string, now time.Time) (*HTTPSecret, error) {
	if !webpolicy.ValidHost(host) {
		return nil, apperror.Invalid("host", apperror.FieldInvalid)
	}
	if !webpolicy.ValidHeaderName(header) || webpolicy.Reserved(header) {
		return nil, apperror.Invalid("header", apperror.FieldInvalid)
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, err
	}
	now = now.UTC().Truncate(time.Microsecond)
	return &HTTPSecret{
		ID: uuid.New(), PublicID: publicID, OrgID: orgID, ProjectID: projectID,
		Host: host, Header: strings.ToLower(header), CreatedAt: now, UpdatedAt: now,
	}, nil
}

func kindToProto(k Kind) webtoolv1.WebSearchProviderKind {
	switch k {
	case KindBrave:
		return webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_BRAVE
	case KindExa:
		return webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_EXA
	case KindTavily:
		return webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_TAVILY
	default:
		return webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_UNSPECIFIED
	}
}

func kindOf(k webtoolv1.WebSearchProviderKind) Kind {
	switch k {
	case webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_BRAVE:
		return KindBrave
	case webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_EXA:
		return KindExa
	case webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_TAVILY:
		return KindTavily
	default:
		return ""
	}
}

func availableKindsToProto() []webtoolv1.WebSearchProviderKind {
	out := make([]webtoolv1.WebSearchProviderKind, 0, len(AvailableKinds()))
	for _, k := range AvailableKinds() {
		out = append(out, kindToProto(k))
	}
	return out
}

func providerToProto(p *Provider) *webtoolv1.WebSearchProvider {
	return &webtoolv1.WebSearchProvider{
		Kind: kindToProto(p.Kind), KeySet: len(p.APIKeySealed) > 0,
		PricePerCallMicros: p.PricePerCallMicros, UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
}

func secretToProto(s *HTTPSecret) *webtoolv1.HTTPSecret {
	return &webtoolv1.HTTPSecret{Id: s.PublicID, Host: s.Host, Header: s.Header, UpdatedAt: timestamppb.New(s.UpdatedAt)}
}
