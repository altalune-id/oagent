// Package mcp wraps the Model Context Protocol Go SDK's streamable-HTTP server with a scope-checked tool registry.
package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultName    = "oagent"
	defaultVersion = "0.0.0"
)

// NOTE: McpUiToolMeta is additionalProperties:false; a sibling key breaks strict hosts.
const metaKeyUI = "ui"

// Server is a streamable-HTTP MCP server.
type Server struct {
	name       string
	version    string
	ui         bool
	scopesFrom ScopesFunc
	mapErr     ErrorMapper
	logger     *slog.Logger
	registry   *Registry

	unmappedCode string

	mu        sync.Mutex
	sealed    bool
	resources map[string]UIResource

	once sync.Once
	sdk  *sdkmcp.Server
}

// NewServer builds a Server from opts.
func NewServer(opts ...Option) *Server {
	s := &Server{
		name:         defaultName,
		version:      defaultVersion,
		registry:     NewRegistry(),
		resources:    map[string]UIResource{},
		logger:       slog.New(slog.DiscardHandler),
		unmappedCode: DefaultUnmappedCode,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Registry returns the server's tool registry.
func (s *Server) Registry() *Registry { return s.registry }

// SDK returns the SDK server, sealing the registry.
func (s *Server) SDK() *sdkmcp.Server {
	s.once.Do(s.build)
	return s.sdk
}

// Handler returns the stateless streamable-HTTP handler.
func (s *Server) Handler() http.Handler {
	sdk := s.SDK()
	return sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return sdk },
		&sdkmcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: s.logger},
	)
}

func (s *Server) build() {
	s.sdk = sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: s.name, Version: s.version},
		&sdkmcp.ServerOptions{Logger: s.logger},
	)
	s.sdk.AddReceivingMiddleware(s.trace, s.listCallable)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealed = true
	specs := s.registry.seal()
	s.mustResolved(specs)
	for _, spec := range specs {
		// SECURITY: with no MCP view, only the model could ever call an app-only tool.
		if spec.AppOnly && !s.ui {
			continue
		}
		s.sdk.AddTool(s.tool(spec), s.dispatch(spec))
	}
	s.addResources()
}

func (s *Server) tool(spec ToolSpec) *sdkmcp.Tool {
	schema := spec.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	title := spec.Title
	if title == "" {
		title = titleOf(spec.Name)
	}
	// NOTE: both hints default to true, so both are always sent.
	destructive := spec.Destructive
	openWorld := false
	tool := &sdkmcp.Tool{
		Name:        spec.Name,
		Title:       title,
		Description: spec.Description,
		InputSchema: schema,
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    !spec.Mutation,
			DestructiveHint: &destructive,
			OpenWorldHint:   &openWorld,
		},
	}
	if s.ui && spec.UI != "" {
		// NOTE: exactly one _meta key; the deprecated flat sibling is rejected by strict hosts.
		ui := map[string]any{"resourceUri": spec.UI}
		if spec.AppOnly {
			ui["visibility"] = []string{"app"}
		}
		tool.Meta = sdkmcp.Meta{metaKeyUI: ui}
	}
	return tool
}

// SECURITY: failures answer in the result with IsError, never a bare error.
func (s *Server) dispatch(spec ToolSpec) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if err := s.authorize(ctx, spec); err != nil {
			return s.failure(ctx, spec.Name, err), nil
		}
		out, err := spec.Handler(ctx, normalize(req.Params.Arguments))
		if err != nil {
			return s.failure(ctx, spec.Name, err), nil
		}
		return s.success(ctx, spec.Name, out), nil
	}
}

// SECURITY: tools/list answers only the tools the same authorize check lets the caller call.
func (s *Server) listCallable(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
	return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
		res, err := next(ctx, method, req)
		list, ok := res.(*sdkmcp.ListToolsResult)
		if err != nil || method != methodToolsList || !ok {
			return res, err
		}
		list.Tools = slices.DeleteFunc(list.Tools, func(tool *sdkmcp.Tool) bool {
			spec, found := s.registry.Spec(tool.Name)
			return !found || s.authorize(ctx, spec) != nil
		})
		return list, nil
	}
}

func (s *Server) authorize(ctx context.Context, spec ToolSpec) error {
	if len(spec.Scopes) == 0 {
		return &ScopeUndeclaredError{Tool: spec.Name}
	}
	var held []string
	if s.scopesFrom != nil {
		held = s.scopesFrom(ctx)
	}
	for _, scope := range spec.Scopes {
		if !slices.Contains(held, scope) {
			return &ScopeDeniedError{Tool: spec.Name, Scope: scope}
		}
	}
	return nil
}

func (s *Server) success(ctx context.Context, tool string, out json.RawMessage) *sdkmcp.CallToolResult {
	raw := normalize(out)
	var structured map[string]any
	if err := json.Unmarshal(raw, &structured); err != nil {
		s.logger.ErrorContext(ctx, "mcp: tool returned JSON that is not an object", "tool", tool, "error", err)
		return s.unmappedResult()
	}
	if structured == nil {
		s.logger.ErrorContext(ctx, "mcp: tool returned a null result", "tool", tool)
		return s.unmappedResult()
	}
	return &sdkmcp.CallToolResult{
		StructuredContent: structured,
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(raw)}},
	}
}

// SECURITY: an unmapped cause is logged, never returned.
func (s *Server) failure(ctx context.Context, tool string, cause error) *sdkmcp.CallToolResult {
	payload := s.mapped(ctx, cause)
	if payload.Code == "" {
		s.logger.ErrorContext(ctx, "mcp: unmapped tool failure", "tool", tool, "error", cause)
		return s.unmappedResult()
	}
	return errorResult(payload)
}

func (s *Server) mapped(ctx context.Context, cause error) ErrorPayload {
	if s.mapErr == nil {
		return ErrorPayload{}
	}
	return s.mapErr(ctx, cause)
}

func (s *Server) unmappedResult() *sdkmcp.CallToolResult {
	return errorResult(ErrorPayload{Code: s.unmappedCode, Message: unmappedMessage})
}

func titleOf(name string) string {
	title := strings.ReplaceAll(name, "_", " ")
	if title == "" {
		return title
	}
	return strings.ToUpper(title[:1]) + title[1:]
}

func errorResult(payload ErrorPayload) *sdkmcp.CallToolResult {
	raw, _ := json.Marshal(payload)
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(raw)}},
	}
}

func normalize(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}
