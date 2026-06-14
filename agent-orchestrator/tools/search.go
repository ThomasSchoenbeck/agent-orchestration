package tools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxSearchFileSize bounds which files search_files will scan for content
// (skips large/generated files). 1 MiB.
const maxSearchFileSize = 1 << 20

// maxSearchLineLen truncates very long matching lines in the result.
const maxSearchLineLen = 400

// searchSkipDirs are directory names search_files never descends into.
var searchSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
}

func searchFilesTool() Definition {
	return Definition{
		Name: "search_files",
		Description: "Search the workspace for files by name or by content (grep-like). Use mode=content " +
			"(default) to find lines matching a regular expression inside files, or mode=name to find files " +
			"whose path matches the expression. Optionally limit by subdirectory and filename glob.",
		Parameters: map[string]Param{
			"query":       {Type: "string", Description: "Regular expression to search for."},
			"mode":        {Type: "string", Description: "'content' (search inside files, default) or 'name' (match file paths)."},
			"path":        {Type: "string", Description: "Subdirectory to search under, relative to workspace root (default '.')."},
			"file_glob":   {Type: "string", Description: "Optional filename glob to limit which files are searched, e.g. '*.go'."},
			"ignore_case": {Type: "boolean", Description: "Case-insensitive match (default false)."},
			"max_results": {Type: "number", Description: "Maximum matches to return (default 100)."},
		},
		Required: []string{"query"}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			query, err := strArg(args, "query")
			if err != nil {
				return nil, err
			}
			mode := strArgOpt(args, "mode")
			if mode == "" {
				mode = "content"
			}
			if mode != "content" && mode != "name" {
				return nil, fmt.Errorf("search_files: mode must be 'content' or 'name', got %q", mode)
			}
			sub := strArgOpt(args, "path")
			if sub == "" {
				sub = "."
			}
			glob := strArgOpt(args, "file_glob")
			maxResults := intArgOpt(args, "max_results", 100)
			if maxResults <= 0 {
				maxResults = 100
			}

			pattern := query
			if v, ok := args["ignore_case"].(bool); ok && v {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("search_files: invalid query regex: %w", err)
			}

			root := safeJoin(repoPath, sub)
			matches := make([]map[string]interface{}, 0, maxResults)
			truncated := false

			walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // skip unreadable entries
				}
				if d.IsDir() {
					if searchSkipDirs[d.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				if glob != "" {
					if ok, _ := filepath.Match(glob, d.Name()); !ok {
						return nil
					}
				}
				rel, rerr := filepath.Rel(repoPath, p)
				if rerr != nil {
					rel = p
				}
				rel = filepath.ToSlash(rel)

				if mode == "name" {
					if re.MatchString(rel) || re.MatchString(d.Name()) {
						matches = append(matches, map[string]interface{}{"file": rel})
						if len(matches) >= maxResults {
							truncated = true
							return filepath.SkipAll
						}
					}
					return nil
				}

				// content mode
				info, ierr := d.Info()
				if ierr != nil || info.Size() > maxSearchFileSize {
					return nil
				}
				data, derr := os.ReadFile(p)
				if derr != nil || isBinary(data) {
					return nil
				}
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						text := strings.TrimRight(line, "\r")
						if len(text) > maxSearchLineLen {
							text = text[:maxSearchLineLen] + "…"
						}
						matches = append(matches, map[string]interface{}{
							"file": rel, "line": i + 1, "text": text,
						})
						if len(matches) >= maxResults {
							truncated = true
							return filepath.SkipAll
						}
					}
				}
				return nil
			})
			if walkErr != nil {
				return nil, fmt.Errorf("search_files: %w", walkErr)
			}

			return map[string]interface{}{
				"matches":   matches,
				"count":     len(matches),
				"truncated": truncated,
				"mode":      mode,
			}, nil
		},
	}
}

// isBinary reports whether data looks like a binary file (contains a NUL byte in
// the first chunk), so content search can skip it.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}
