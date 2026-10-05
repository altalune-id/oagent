package controlplane

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"

	orgv1connect "altalune.id/auth-server/gen/go/org/v1/orgv1connect"
	"altalune.id/auth-server/httpclient"
)

// DefaultClientTimeout bounds every remote CLI request.
const DefaultClientTimeout = 30 * time.Second

// Client bundles the Connect clients the CLI uses to reach a remote auth-server.
type Client struct {
	Org orgv1connect.OrgServiceClient
}

// NewClient points every client at baseURL+"/api"; a non-empty token becomes the Bearer header.
func NewClient(baseURL, token string) *Client {
	httpClient := httpclient.New(
		httpclient.WithAllowPrivateHosts(true), // the CLI may legitimately target localhost/private hosts
		httpclient.WithTimeout(DefaultClientTimeout),
	)
	base := strings.TrimRight(baseURL, "/") + "/api"
	opts := []connect.ClientOption{connect.WithInterceptors(bearerInterceptor(token))}
	return &Client{Org: orgv1connect.NewOrgServiceClient(httpClient, base, opts...)}
}

func bearerInterceptor(token string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if token != "" {
				req.Header().Set("Authorization", "Bearer "+token)
			}
			return next(ctx, req)
		}
	}
}
