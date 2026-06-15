package tools

import (
	"context"
	"strings"
	"testing"
)

// --- capabilities.go ---

func TestCapabilities(t *testing.T) {
	ctx := WithCapabilities(context.Background(), []string{"creates_tasks", "handles_review"})
	if !contextHasCapability(ctx, "creates_tasks") {
		t.Error("expected creates_tasks to be granted")
	}
	if contextHasCapability(ctx, "handles_merge") {
		t.Error("handles_merge should not be granted")
	}
	// Unscoped context ⇒ gate is open.
	if !contextHasCapability(context.Background(), "anything") {
		t.Error("unscoped context should allow any capability")
	}
	if caps, ok := capabilitiesFromContext(ctx); !ok || len(caps) != 2 {
		t.Errorf("capabilitiesFromContext = %v, %v", caps, ok)
	}
	if _, ok := capabilitiesFromContext(context.Background()); ok {
		t.Error("capabilitiesFromContext on unscoped context should be !ok")
	}
}

// --- executor.go arg helpers ---

func TestArgHelpers(t *testing.T) {
	args := map[string]interface{}{
		"s":   "hello",
		"n":   float64(7),
		"ni":  3,
		"bad": 42,
		"obj": map[string]interface{}{"a": 1},
		"str": `{"already":"json"}`,
	}

	if v, err := strArg(args, "s"); err != nil || v != "hello" {
		t.Errorf("strArg(s) = %q, %v", v, err)
	}
	if _, err := strArg(args, "missing"); err == nil {
		t.Error("strArg(missing) should error")
	}
	if _, err := strArg(args, "bad"); err == nil {
		t.Error("strArg(bad type) should error")
	}

	if strArgOpt(args, "s") != "hello" || strArgOpt(args, "missing") != "" || strArgOpt(args, "bad") != "" {
		t.Error("strArgOpt behavior wrong")
	}

	if intArgOpt(args, "n", 0) != 7 || intArgOpt(args, "ni", 0) != 3 || intArgOpt(args, "missing", 9) != 9 {
		t.Error("intArgOpt behavior wrong")
	}

	if jsonArg(args, "missing") != "" {
		t.Error("jsonArg(missing) should be empty")
	}
	if jsonArg(args, "str") != `{"already":"json"}` {
		t.Error("jsonArg(string) should pass through")
	}
	if got := jsonArg(args, "obj"); !strings.Contains(got, `"a":1`) {
		t.Errorf("jsonArg(obj) = %q", got)
	}
}

// --- executor.go Registry ---

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	def := Definition{
		Name:        "echo",
		Description: "echoes x",
		Parameters:  map[string]Param{"x": {Type: "string", Description: "value"}},
		Required:    []string{"x"},
		Handler: func(_ context.Context, args map[string]interface{}) (interface{}, error) {
			s, err := strArg(args, "x")
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"echo": s}, nil
		},
	}
	if err := reg.Register(def); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(def); err == nil {
		t.Error("duplicate Register should error")
	}

	// Execute success.
	res, err := reg.Execute(context.Background(), "echo", map[string]interface{}{"x": "hi"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if m, _ := res.(map[string]interface{}); m["echo"] != "hi" {
		t.Errorf("Execute result = %v", res)
	}
	// Execute unknown tool.
	if _, err := reg.Execute(context.Background(), "nope", nil); err == nil {
		t.Error("Execute(unknown) should error")
	}

	// ExecuteJSON success + handler-error-as-JSON.
	if js, _ := reg.ExecuteJSON(context.Background(), "echo", map[string]interface{}{"x": "hi"}); !strings.Contains(js, `"echo":"hi"`) {
		t.Errorf("ExecuteJSON = %q", js)
	}
	if js, _ := reg.ExecuteJSON(context.Background(), "echo", map[string]interface{}{}); !strings.Contains(js, "error") {
		t.Errorf("ExecuteJSON(handler error) = %q", js)
	}

	// Get + List + ToLLMDef.
	if _, err := reg.Get("echo"); err != nil {
		t.Errorf("Get(echo): %v", err)
	}
	if _, err := reg.Get("nope"); err == nil {
		t.Error("Get(unknown) should error")
	}
	list := reg.List()
	if len(list) != 1 || list[0].Name != "echo" {
		t.Errorf("List = %+v", list)
	}
}
