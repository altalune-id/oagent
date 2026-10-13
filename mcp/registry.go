package mcp

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sync"
)

// ToolHandler executes one tool call over raw JSON in and raw JSON out.
type ToolHandler func(ctx context.Context, req json.RawMessage) (json.RawMessage, error)

// ToolSpec describes one MCP tool; all of its Scopes are checked at call time.
type ToolSpec struct {
	Name        string
	Title       string
	Description string
	Scopes      []string
	Mutation    bool
	Destructive bool
	UI          string
	AppOnly     bool
	InputSchema any
	Handler     ToolHandler
}

// Registry holds every registered tool and its backing instance.
type Registry struct {
	mu       sync.RWMutex
	sealed   bool
	tools    map[string]ToolSpec
	handlers map[string]any
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]ToolSpec{}, handlers: map[string]any{}}
}

// Register adds spec, panicking on an invalid or post-build registration.
func (r *Registry) Register(spec ToolSpec, instance any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if spec.Name == "" {
		panic("mcp: ToolSpec.Name must not be empty")
	}
	if spec.Handler == nil {
		panic("mcp: ToolSpec.Handler must not be nil for tool " + spec.Name)
	}
	if _, dup := r.tools[spec.Name]; dup {
		panic("mcp: duplicate tool name " + spec.Name)
	}
	if r.sealed {
		panic("mcp: tool " + spec.Name + " registered after the server was built")
	}
	r.tools[spec.Name] = spec
	r.handlers[spec.Name] = instance
}

// Names returns every registered tool name, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.tools))
}

// Spec returns the spec registered for name, and whether name is known.
func (r *Registry) Spec(name string) (ToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.tools[name]
	return spec, ok
}

// HandlerFor returns the instance registered for name. SECURITY: guard-test introspection only.
func (r *Registry) HandlerFor(name string) any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[name]
}

func (r *Registry) seal() []ToolSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sealed = true
	specs := make([]ToolSpec, 0, len(r.tools))
	for _, name := range slices.Sorted(maps.Keys(r.tools)) {
		specs = append(specs, r.tools[name])
	}
	return specs
}
