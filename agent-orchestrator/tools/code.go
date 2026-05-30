package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RegisterCodeTools registers all code-execution tools into reg.
// These tools allow an agent to read/write files, apply diffs, run tests,
// and manage git repositories.
func RegisterCodeTools(reg *Registry) error {
	defs := []Definition{
		readFileTool(),
		writeFileTool(),
		applyDiffTool(),
		runTestsTool(),
		gitCloneTool(),
		gitCheckoutTool(),
		listFilesTool(),
	}
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

func readFileTool() Definition {
	return Definition{
		Name:        "read_file",
		Description: "Read the contents of a file from the workspace.",
		Parameters: map[string]Param{
			"file_path": {Type: "string", Description: "Path to the file (relative to workspace root)"},
		},
		Required: []string{"file_path"}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			filePath, err := strArg(args, "file_path")
			if err != nil {
				return nil, err
			}

			full := safeJoin(repoPath, filePath)
			data, err := os.ReadFile(full)
			if err != nil {
				return nil, fmt.Errorf("read_file %q: %w", filePath, err)
			}
			return map[string]interface{}{
				"content": string(data),
				"path":    filePath,
				"size":    len(data),
			}, nil
		},
	}
}

func writeFileTool() Definition {
	return Definition{
		Name:        "write_file",
		Description: "Write content to a file in the workspace. Creates parent directories as needed. Overwrites if the file exists.",
		Parameters: map[string]Param{
			"file_path": {Type: "string", Description: "Path to the file (relative to workspace root), e.g. src/main.go"},
			"content":   {Type: "string", Description: "The exact text content to write into the file"},
		},
		Required: []string{"file_path", "content"}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			filePath, err := strArg(args, "file_path")
			if err != nil {
				return nil, err
			}
			content, err := strArg(args, "content")
			if err != nil {
				return nil, err
			}

			full := safeJoin(repoPath, filePath)
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				return nil, fmt.Errorf("write_file: create dirs for %q: %w", filePath, err)
			}
			if err := os.WriteFile(full, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("write_file %q: %w", filePath, err)
			}
			return map[string]interface{}{
				"success": true,
				"path":    filePath,
				"bytes":   len(content),
			}, nil
		},
	}
}

func applyDiffTool() Definition {
	return Definition{
		Name:        "apply_diff",
		Description: "Apply a unified diff to the workspace using the `patch` command.",
		Parameters: map[string]Param{
			"diff": {Type: "string", Description: "Unified diff content (output of git diff or similar)"},
		},
		Required: []string{"diff"}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			diff, err := strArg(args, "diff")
			if err != nil {
				return nil, err
			}

			cmd := exec.CommandContext(ctx, "patch", "-p1", "--forward")
			cmd.Dir = repoPath
			cmd.Stdin = strings.NewReader(diff)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return map[string]interface{}{
					"success": false,
					"output":  string(out),
					"error":   err.Error(),
				}, nil
			}
			return map[string]interface{}{
				"success": true,
				"output":  string(out),
			}, nil
		},
	}
}

func runTestsTool() Definition {
	return Definition{
		Name:        "run_tests",
		Description: "Run the test suite in the workspace. Returns stdout, stderr, and a pass/fail summary.",
		Parameters: map[string]Param{
			"test_command": {Type: "string", Description: "Shell command to run tests, e.g. 'go test ./...' or 'pytest'"},
			"timeout_sec":  {Type: "number", Description: "Timeout in seconds (default 120)"},
		},
		Required: []string{"test_command"}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			testCmd, err := strArg(args, "test_command")
			if err != nil {
				return nil, err
			}

			cmd := exec.CommandContext(ctx, "sh", "-c", testCmd)
			cmd.Dir = repoPath
			out, err := cmd.CombinedOutput()

			passed := err == nil
			result := map[string]interface{}{
				"passed":  passed,
				"output":  string(out),
				"command": testCmd,
			}
			if !passed {
				result["error"] = err.Error()
			}
			return result, nil
		},
	}
}

func gitCloneTool() Definition {
	return Definition{
		Name:        "git_clone",
		Description: "Clone a git repository to a local path.",
		Parameters: map[string]Param{
			"repo_url":    {Type: "string", Description: "URL of the git repository to clone"},
			"target_path": {Type: "string", Description: "Absolute local path to clone into"},
		},
		Required: []string{"repo_url", "target_path"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoURL, err := strArg(args, "repo_url")
			if err != nil {
				return nil, err
			}
			targetPath, err := strArg(args, "target_path")
			if err != nil {
				return nil, err
			}

			cmd := exec.CommandContext(ctx, "git", "clone", repoURL, targetPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return map[string]interface{}{
					"success": false,
					"output":  string(out),
					"error":   err.Error(),
				}, nil
			}
			return map[string]interface{}{
				"success":     true,
				"target_path": targetPath,
				"output":      string(out),
			}, nil
		},
	}
}

func gitCheckoutTool() Definition {
	return Definition{
		Name:        "git_checkout",
		Description: "Checkout a branch or commit in the workspace repository.",
		Parameters: map[string]Param{
			"branch": {Type: "string", Description: "Branch name or commit SHA to checkout"},
			"create": {Type: "string", Description: "Set to 'true' to create the branch if it does not exist"},
		},
		Required: []string{"branch"}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			branch, err := strArg(args, "branch")
			if err != nil {
				return nil, err
			}
			create := strArgOpt(args, "create") == "true"

			var gitArgs []string
			if create {
				gitArgs = []string{"checkout", "-b", branch}
			} else {
				gitArgs = []string{"checkout", branch}
			}
			cmd := exec.CommandContext(ctx, "git", gitArgs...)
			cmd.Dir = repoPath
			out, err := cmd.CombinedOutput()
			if err != nil {
				return map[string]interface{}{
					"success": false,
					"output":  string(out),
					"error":   err.Error(),
				}, nil
			}
			return map[string]interface{}{
				"success": true,
				"branch":  branch,
				"output":  string(out),
			}, nil
		},
	}
}

func listFilesTool() Definition {
	return Definition{
		Name:        "list_files",
		Description: "List files in a directory within the workspace.",
		Parameters: map[string]Param{
			"dir_path": {Type: "string", Description: "Directory path relative to workspace root (default: '.')"},
			"pattern":  {Type: "string", Description: "Optional glob pattern to filter results (e.g. '*.go')"},
		},
		Required: []string{}, // repo_path injected by executor
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			repoPath, err := strArg(args, "repo_path")
			if err != nil {
				return nil, err
			}
			dirPath := strArgOpt(args, "dir_path")
			if dirPath == "" {
				dirPath = "."
			}
			pattern := strArgOpt(args, "pattern")

			base := safeJoin(repoPath, dirPath)
			entries, err := os.ReadDir(base)
			if err != nil {
				return nil, fmt.Errorf("list_files %q: %w", dirPath, err)
			}

			var files []string
			for _, e := range entries {
				name := e.Name()
				if pattern != "" {
					matched, err := filepath.Match(pattern, name)
					if err != nil || !matched {
						continue
					}
				}
				if e.IsDir() {
					name += "/"
				}
				files = append(files, name)
			}
			return map[string]interface{}{
				"files": files,
				"count": len(files),
				"dir":   dirPath,
			}, nil
		},
	}
}

// safeJoin joins base and rel, cleaning the result to prevent path traversal.
func safeJoin(base, rel string) string {
	return filepath.Join(base, filepath.Clean("/"+rel))
}
