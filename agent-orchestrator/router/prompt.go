package router

import (
	"fmt"
	"strings"

	"agent-orchestrator/config"
)

// Prompter fills prompt templates from config.prompts using simple
// {variable} substitution.
type Prompter struct {
	cfg *config.Config
}

// NewPrompter creates a Prompter backed by the given config.
func NewPrompter(cfg *config.Config) *Prompter {
	return &Prompter{cfg: cfg}
}

// Render returns the prompt for taskType with all {key} placeholders
// replaced by values from vars. If no template exists for taskType, a
// generic fallback is returned.
func (p *Prompter) Render(taskType string, vars map[string]interface{}) string {
	tmpl, ok := p.cfg.Prompts[taskType]
	if !ok {
		// Generic fallback.
		tmpl = "Execute the following task:\n\n{payload}"
	}
	return fillTemplate(tmpl, vars)
}

// fillTemplate replaces {key} occurrences in tmpl with string-formatted
// values from vars. Missing keys are left as-is.
func fillTemplate(tmpl string, vars map[string]interface{}) string {
	result := tmpl
	for k, v := range vars {
		placeholder := "{" + k + "}"
		var str string
		switch val := v.(type) {
		case string:
			str = val
		case nil:
			str = ""
		default:
			str = fmt.Sprintf("%v", val)
		}
		result = strings.ReplaceAll(result, placeholder, str)
	}
	return result
}
