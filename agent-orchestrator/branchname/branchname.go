// Package branchname builds human-readable git branch names from a per-task-type
// template. It is intentionally dependency-free so it is trivially testable and
// reusable by the server's claim path.
package branchname

import "strings"

const maxSlugLen = 50

// Slugify converts arbitrary text into a git-ref-safe slug: lowercase, with runs
// of non-alphanumeric characters collapsed to a single dash, trimmed, and capped
// at maxSlugLen. Returns "" for input with no alphanumeric content.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > maxSlugLen {
		out = strings.Trim(out[:maxSlugLen], "-")
	}
	return out
}

// shortID returns the first 8 characters of an id (or the whole id if shorter).
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Generate builds a branch name from a task type's template, substituting:
//
//	{slug}    → slugified title (falls back to {shortid} when the title is empty)
//	{id}      → full task id
//	{shortid} → first 8 chars of the task id
//	{type}    → task type key
//
// An empty template falls back to "{type}/{slug}". The result is trimmed of
// leading/trailing slashes; if substitution yields nothing usable it falls back
// to "task-<shortid>".
func Generate(template, title, taskID, typeKey string) string {
	if template == "" {
		template = "{type}/{slug}"
	}
	short := shortID(taskID)
	slug := Slugify(title)
	if slug == "" {
		slug = short
	}
	out := strings.NewReplacer(
		"{slug}", slug,
		"{shortid}", short,
		"{id}", taskID,
		"{type}", typeKey,
	).Replace(template)

	// Collapse accidental double slashes and trim.
	for strings.Contains(out, "//") {
		out = strings.ReplaceAll(out, "//", "/")
	}
	out = strings.Trim(out, "/")
	if out == "" {
		out = "task-" + short
	}
	return out
}
