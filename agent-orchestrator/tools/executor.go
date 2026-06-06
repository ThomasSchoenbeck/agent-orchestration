// Package tools provides a registry and runner for agent tools —
// callable functions exposed to LLMs via the tool-use protocol.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"agent-orchestrator/llm"
)

// Handler is the function signature every tool must implement.
type Handler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// Definition ties together the metadata an LLM needs (name, description,
// JSON-schema) with the Go handler that executes the tool.
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]Param // keyed by parameter name
	Required    []string
	Handler     Handler
}

// Param describes a single tool parameter.
type Param struct {
	Type        string // "string" | "number" | "boolean" | "array" | "object"
	Description string
}

// ToLLMDef converts a Definition into the llm.ToolDef format understood by
// LLM providers.
func (d *Definition) ToLLMDef() llm.ToolDef {
	props := make(map[string]llm.Property, len(d.Parameters))
	for name, p := range d.Parameters {
		props[name] = llm.Property{Type: p.Type, Description: p.Description}
	}
	return llm.ToolDef{
		Name:        d.Name,
		Description: d.Description,
		InputSchema: llm.InputSchema{
			Type:       "object",
			Properties: props,
			Required:   d.Required,
		},
	}
}

// Registry holds all registered tools and executes them by name.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Definition
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Definition)}
}

// Register adds a tool to the registry. Returns an error if a tool with the
// same name already exists.
func (r *Registry) Register(def Definition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[def.Name]; ok {
		return fmt.Errorf("tool %q already registered", def.Name)
	}
	cp := def // copy so callers can't mutate after registration
	r.tools[def.Name] = &cp
	return nil
}

// Execute runs the named tool with the provided arguments.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	r.mu.RLock()
	def, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	return def.Handler(ctx, args)
}

// ExecuteJSON runs a tool and returns its result as a JSON string, as expected
// by the LLM tool-result message format.
func (r *Registry) ExecuteJSON(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := r.Execute(ctx, name, args)
	if err != nil {
		// Encode the error as a tool result so the LLM can see what went wrong.
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(b), nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("tool %q: failed to marshal result: %w", name, err)
	}
	return string(b), nil
}

// Get returns the named tool definition, or an error if not found.
func (r *Registry) Get(name string) (*Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	return def, nil
}

// List returns all registered tool definitions as llm.ToolDef values, ready to
// include in a ChatRequest.
func (r *Registry) List() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.ToolDef, 0, len(r.tools))
	for _, def := range r.tools {
		out = append(out, def.ToLLMDef())
	}
	return out
}

// strArg extracts a required string arg from the args map.
func strArg(args map[string]interface{}, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string, got %T", key, v)
	}
	return s, nil
}

// strArgOpt extracts an optional string arg; returns "" if absent.
func strArgOpt(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// intArgOpt extracts an optional int arg (accepts float64 from JSON decode).
func intArgOpt(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// jsonArg returns the argument as a JSON string, accepting either a string the
// model already stringified, or a native array/object the model passed directly
// (smaller models commonly emit structured JSON instead of a JSON-encoded
// string). Returns "" when the key is absent.
func jsonArg(args map[string]interface{}, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
