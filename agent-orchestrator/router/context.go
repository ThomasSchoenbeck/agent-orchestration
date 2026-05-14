package router

import (
	"fmt"
	"strings"

	"agent-orchestrator/config"
)

// ContextEntry is a single piece of stored context (summary, snippet, note, etc.).
type ContextEntry struct {
	Type    string // "summary" | "snippet" | "note" | "diff" | "test_results" | ...
	Content string
	Source  string // optional: file path, task ID, etc.
}

// ContextBuilder assembles a context string for an agent based on the
// context_rules defined in config for its role.
type ContextBuilder struct {
	cfg *config.Config
}

// NewContextBuilder creates a ContextBuilder.
func NewContextBuilder(cfg *config.Config) *ContextBuilder {
	return &ContextBuilder{cfg: cfg}
}

// Build filters entries by the include/exclude rules for role and formats
// them into a single context block ready to inject into a prompt.
// Deprecated: use BuildWithRules instead for DB-backed rules.
func (b *ContextBuilder) Build(role string, entries []ContextEntry) string {
	rule, hasRule := b.cfg.ContextRules[role]
	var include, exclude []string
	if hasRule {
		include = rule.Include
		exclude = rule.Exclude
	}
	return b.BuildWithRules(entries, include, exclude)
}

// BuildWithRules filters entries by explicit include/exclude rules and formats
// them into a single context block. This is used by DB-backed role definitions.
func (b *ContextBuilder) BuildWithRules(entries []ContextEntry, include, exclude []string) string {
	var kept []ContextEntry
	for _, e := range entries {
		if len(include) > 0 && !containsStr(include, e.Type) {
			continue // not in include list
		}
		if containsStr(exclude, e.Type) {
			continue // explicitly excluded
		}
		kept = append(kept, e)
	}

	if len(kept) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== Context ===\n")
	for i, e := range kept {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		if e.Source != "" {
			sb.WriteString(fmt.Sprintf("[%s | %s]\n", e.Type, e.Source))
		} else {
			sb.WriteString(fmt.Sprintf("[%s]\n", e.Type))
		}
		sb.WriteString(e.Content)
		sb.WriteString("\n")
	}
	sb.WriteString("=== End Context ===")
	return sb.String()
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
