package mcp

import (
	"context"
	"maps"
	"slices"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MIMEApp is the MCP Apps media type for a single-file HTML UI resource.
const MIMEApp = "text/html;profile=mcp-app"

// UIResource is one static MCP Apps bundle a tool's result can be rendered by.
type UIResource struct {
	URI      string
	Name     string
	MIMEType string
	Body     string
	// PrefersBorder asks the host to draw a border; nil leaves the host default.
	PrefersBorder *bool
}

// NOTE: McpUiResourceMeta is additionalProperties:false; the deprecated flat sibling is not emitted.
func (r UIResource) meta() sdkmcp.Meta {
	ui := map[string]any{}
	if r.PrefersBorder != nil {
		ui["prefersBorder"] = *r.PrefersBorder
	}
	if len(ui) == 0 {
		return nil
	}
	return sdkmcp.Meta{metaKeyUI: ui}
}

// AddUIResource publishes r, panicking on invalid, duplicate, or post-build registrations.
func (s *Server) AddUIResource(r UIResource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URI == "" {
		panic("mcp: UIResource.URI must not be empty")
	}
	if !strings.HasPrefix(r.URI, "ui://") {
		panic("mcp: UIResource.URI scheme must be ui, got " + r.URI)
	}
	if r.Body == "" {
		panic("mcp: UIResource.Body must not be empty for " + r.URI)
	}
	if _, dup := s.resources[r.URI]; dup {
		panic("mcp: duplicate UI resource " + r.URI)
	}
	if s.sealed {
		panic("mcp: UI resource " + r.URI + " published after the server was built")
	}
	if r.MIMEType == "" {
		r.MIMEType = MIMEApp
	}
	s.resources[r.URI] = r
}

func (s *Server) addResources() {
	for _, uri := range slices.Sorted(maps.Keys(s.resources)) {
		r := s.resources[uri]
		s.sdk.AddResource(&sdkmcp.Resource{
			Meta:     r.meta(),
			URI:      r.URI,
			Name:     r.Name,
			MIMEType: r.MIMEType,
		}, s.readResource(r))
	}
}

func (s *Server) readResource(r UIResource) sdkmcp.ResourceHandler {
	return func(context.Context, *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{{
				URI:      r.URI,
				MIMEType: r.MIMEType,
				Text:     r.Body,
				Meta:     r.meta(),
			}},
		}, nil
	}
}

// SECURITY: an unpublished UI reference fails the boot rather than rendering a blank panel.
func (s *Server) mustResolved(specs []ToolSpec) {
	if !s.ui {
		return
	}
	for _, spec := range specs {
		if spec.UI == "" {
			continue
		}
		if _, ok := s.resources[spec.UI]; !ok {
			panic("mcp: tool " + spec.Name + " references unpublished UI resource " + spec.UI)
		}
	}
}
