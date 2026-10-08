package controlplane

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"

	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/httpclient"
)

// DefaultClientTimeout bounds every remote CLI request.
const DefaultClientTimeout = 30 * time.Second

// Client bundles the Connect clients the CLI uses to reach a remote oagent.
type Client struct {
	Org      orgv1connect.OrgServiceClient
	Projects projectv1connect.ProjectServiceClient
}

// NewClient points every client at baseURL+"/api"; a non-empty token becomes the Bearer header.
func NewClient(baseURL, token string) *Client {
	httpClient := httpclient.New(
		httpclient.WithAllowPrivateHosts(true), // the CLI may legitimately target localhost/private hosts
		httpclient.WithTimeout(DefaultClientTimeout),
	)
	base := strings.TrimRight(baseURL, "/") + "/api"
	opts := []connect.ClientOption{connect.WithInterceptors(bearerInterceptor(token))}
	return &Client{
		Org:      orgv1connect.NewOrgServiceClient(httpClient, base, opts...),
		Projects: projectv1connect.NewProjectServiceClient(httpClient, base, opts...),
	}
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
