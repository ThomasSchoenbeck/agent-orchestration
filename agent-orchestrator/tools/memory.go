package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-orchestrator/db"
)

// Task memory (Multi-session orchestration, 2026-07-04) is a durable,
// agent-writable scratchpad kept in the worktree under .agent_context/. It
// survives context-window checkpoints (a fresh session re-reads it) but is
// gitignored, so it never lands in the project repo. Stored as memory.json
// (structured) plus memory.md (human-readable rendering).
const (
	memoryDir      = ".agent_context"
	memoryJSONName = "memory.json"
	memoryMDName   = "memory.md"
)

// memorySections are the writable sections of task memory. summary is a single
// text block; the rest are append-logs.
var memorySections = map[string]bool{
	"summary":        true,
	"progress":       true,
	"decisions":      true,
	"findings":       true,
	"open_questions": true,
}

// RegisterMemoryTools registers the read_memory / write_memory tools.
func RegisterMemoryTools(reg *Registry) error {
	for _, d := range []Definition{writeMemoryTool(), readMemoryTool()} {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

func writeMemoryTool() Definition {
	return Definition{
		Name: "write_memory",
		Description: "Record durable task memory in the worktree so work can continue across " +
			"context-window checkpoints and new sessions. Sections: summary (the current task summary), " +
			"progress (a log of steps taken), decisions, findings, open_questions. Use mode=append to add " +
			"an entry (default) or mode=replace to overwrite the section.",
		Parameters: map[string]Param{
			"section": {Type: "string", Description: "One of: summary, progress, decisions, findings, open_questions"},
			"content": {Type: "string", Description: "The text to record in the section"},
			"mode":    {Type: "string", Description: "append (default) or replace"},
		},
		Required: []string{"section", "content"}, // repo_path injected by executor
		Handler: func(_ context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			section, err := strArg(args, "section")
			if err != nil {
				return nil, err
			}
			section = strings.TrimSpace(strings.ToLower(section))
			if !memorySections[section] {
				return nil, fmt.Errorf("write_memory: unknown section %q (want summary|progress|decisions|findings|open_questions)", section)
			}
			content, err := strArg(args, "content")
			if err != nil {
				return nil, err
			}
			replace := strings.EqualFold(strArgOpt(args, "mode"), "replace")

			mem, err := loadTaskMemory(repoPath)
			if err != nil {
				return nil, fmt.Errorf("write_memory: %w", err)
			}
			applyMemoryUpdate(&mem, section, content, replace)
			if err := saveTaskMemory(repoPath, mem); err != nil {
				return nil, fmt.Errorf("write_memory: %w", err)
			}
			return map[string]interface{}{"success": true, "section": section, "mode": modeLabel(replace)}, nil
		},
	}
}

func readMemoryTool() Definition {
	return Definition{
		Name: "read_memory",
		Description: "Read the durable task memory recorded for this task (summary, progress, decisions, " +
			"findings, open_questions). Pass a section to read just that one; omit it to read all. Returns " +
			"empty memory (not an error) when nothing has been recorded yet.",
		Parameters: map[string]Param{
			"section": {Type: "string", Description: "Optional: one section to read (summary, progress, decisions, findings, open_questions)"},
		},
		Required: []string{}, // repo_path injected by executor
		Handler: func(_ context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			mem, err := loadTaskMemory(repoPath)
			if err != nil {
				return nil, fmt.Errorf("read_memory: %w", err)
			}
			section := strings.TrimSpace(strings.ToLower(strArgOpt(args, "section")))
			if section == "" {
				return map[string]interface{}{"memory": mem}, nil
			}
			if !memorySections[section] {
				return nil, fmt.Errorf("read_memory: unknown section %q", section)
			}
			return map[string]interface{}{"section": section, "value": memorySection(mem, section)}, nil
		},
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func memoryPaths(repoPath string) (jsonPath, mdPath string) {
	dir := filepath.Join(repoPath, memoryDir)
	return filepath.Join(dir, memoryJSONName), filepath.Join(dir, memoryMDName)
}

// loadTaskMemory reads the worktree memory.json, returning empty memory when the
// file does not exist yet.
func loadTaskMemory(repoPath string) (db.TaskMemoryContent, error) {
	var c db.TaskMemoryContent
	jsonPath, _ := memoryPaths(repoPath)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &c); err != nil {
			return c, fmt.Errorf("parse memory.json: %w", err)
		}
	}
	return c, nil
}

// saveTaskMemory writes both memory.json (structured) and memory.md (rendered).
func saveTaskMemory(repoPath string, c db.TaskMemoryContent) error {
	jsonPath, mdPath := memoryPaths(repoPath)
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(mdPath, []byte(renderMemoryMD(c)), 0o644)
}

// applyMemoryUpdate mutates the given section. summary is text (replace, or
// append-with-newline); the others are append-logs (append one entry, or replace
// the whole list with the single entry).
func applyMemoryUpdate(c *db.TaskMemoryContent, section, content string, replace bool) {
	switch section {
	case "summary":
		if replace || c.Summary == "" {
			c.Summary = content
		} else {
			c.Summary = c.Summary + "\n" + content
		}
	case "progress":
		c.Progress = appendOrReplace(c.Progress, content, replace)
	case "decisions":
		c.Decisions = appendOrReplace(c.Decisions, content, replace)
	case "findings":
		c.Findings = appendOrReplace(c.Findings, content, replace)
	case "open_questions":
		c.OpenQuestions = appendOrReplace(c.OpenQuestions, content, replace)
	}
}

func appendOrReplace(list []string, content string, replace bool) []string {
	if replace {
		return []string{content}
	}
	return append(list, content)
}

func memorySection(c db.TaskMemoryContent, section string) interface{} {
	switch section {
	case "summary":
		return c.Summary
	case "progress":
		return c.Progress
	case "decisions":
		return c.Decisions
	case "findings":
		return c.Findings
	case "open_questions":
		return c.OpenQuestions
	}
	return nil
}

func modeLabel(replace bool) string {
	if replace {
		return "replace"
	}
	return "append"
}

// renderMemoryMD produces a human-readable Markdown view of task memory.
func renderMemoryMD(c db.TaskMemoryContent) string {
	var b strings.Builder
	b.WriteString("# Task Memory\n\n## Summary\n\n")
	if c.Summary != "" {
		b.WriteString(c.Summary + "\n")
	} else {
		b.WriteString("_none_\n")
	}
	writeList := func(title string, items []string) {
		b.WriteString("\n## " + title + "\n\n")
		if len(items) == 0 {
			b.WriteString("_none_\n")
			return
		}
		for _, it := range items {
			b.WriteString("- " + it + "\n")
		}
	}
	writeList("Progress", c.Progress)
	writeList("Decisions", c.Decisions)
	writeList("Findings", c.Findings)
	writeList("Open Questions", c.OpenQuestions)
	return b.String()
}
