package router

import (
	"fmt"
	"strings"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
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

// Scope context entry types (Feature 5). A role receives these only if its
// include/exclude rules permit them, so implementation roles stay focused on
// their own task while planner-type roles see the whole-project scope.
const (
	ScopeTypeRequirements = "project_requirements"
	ScopeTypeFeatures     = "project_features"
)

// ScopeEntries builds the synthetic context entries for a project's
// requirements and features. Empty blocks are omitted.
func ScopeEntries(requirements, features string) []ContextEntry {
	var out []ContextEntry
	if strings.TrimSpace(requirements) != "" {
		out = append(out, ContextEntry{Type: ScopeTypeRequirements, Content: requirements})
	}
	if strings.TrimSpace(features) != "" {
		out = append(out, ContextEntry{Type: ScopeTypeFeatures, Content: features})
	}
	return out
}

// Persona is an agent's effective configuration after composing its role with
// its assigned skills (Feature 6).
type Persona struct {
	SystemPrompt   string
	ContextInclude []string
	ContextExclude []string
	AllowedTools   []string
}

// ResolveAgentPersona merges a role with its assigned skills:
//   - SystemPrompt   = role.SystemPrompt then each skill's PromptFragment (stable order)
//   - Context rules  = union of the role's and every skill's include/exclude
//   - AllowedTools   = union of the role's and the skills' tools
//
// Capabilities are intentionally NOT part of the persona — lifecycle authority
// comes only from roles, keeping the security model in the role layer.
func ResolveAgentPersona(role *db.RoleDefinition, skills []*db.SkillDefinition) Persona {
	var p Persona
	var prompts []string

	if role != nil {
		if strings.TrimSpace(role.SystemPrompt) != "" {
			prompts = append(prompts, role.SystemPrompt)
		}
		p.ContextInclude = appendUnique(p.ContextInclude, role.ContextInclude...)
		p.ContextExclude = appendUnique(p.ContextExclude, role.ContextExclude...)
		p.AllowedTools = appendUnique(p.AllowedTools, role.AllowedTools...)
	}
	for _, s := range skills {
		if s == nil {
			continue
		}
		if strings.TrimSpace(s.PromptFragment) != "" {
			prompts = append(prompts, s.PromptFragment)
		}
		p.ContextInclude = appendUnique(p.ContextInclude, s.ContextInclude...)
		p.ContextExclude = appendUnique(p.ContextExclude, s.ContextExclude...)
		p.AllowedTools = appendUnique(p.AllowedTools, s.AllowedTools...)
	}
	p.SystemPrompt = strings.Join(prompts, "\n\n")
	return p
}

// appendUnique appends items to dst, skipping any value already present.
func appendUnique(dst []string, items ...string) []string {
	for _, it := range items {
		if !containsStr(dst, it) {
			dst = append(dst, it)
		}
	}
	return dst
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
