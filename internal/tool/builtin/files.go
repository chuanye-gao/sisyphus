package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxReadFileBytes = 256 * 1024
	defaultMaxItems  = 500
	defaultMaxHits   = 100
	maxSearchBytes   = 2 * 1024 * 1024
)

type SafeReadFileTool struct{}

func (SafeReadFileTool) Name() string { return "read_file" }

func (SafeReadFileTool) Description() string {
	return "Read a text file. Optional start_line and end_line select a 1-based inclusive line range."
}

func (SafeReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to read"},
			"start_line": {"type": "integer", "description": "Optional 1-based start line"},
			"end_line": {"type": "integer", "description": "Optional 1-based end line"}
		},
		"required": ["path"]
	}`)
}

func (SafeReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("read_file: parse arguments: %w", err)
	}
	if strings.TrimSpace(params.Path) == "" {
		return "", fmt.Errorf("read_file: path is required")
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	content := string(data)
	if params.StartLine > 0 || params.EndLine > 0 {
		content, err = sliceLines(content, params.StartLine, params.EndLine)
		if err != nil {
			return "", err
		}
	}
	if len(content) > maxReadFileBytes {
		return content[:maxReadFileBytes] + "\n\n[truncated]", nil
	}
	return content, nil
}

type SafeWriteFileTool struct{}

func (SafeWriteFileTool) Name() string { return "write_file" }

func (SafeWriteFileTool) Description() string {
	return "Write a complete file, creating parent directories as needed. Prefer edit_file for small changes."
}

func (SafeWriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to write"},
			"content": {"type": "string", "description": "Full file content"}
		},
		"required": ["path", "content"]
	}`)
}

func (SafeWriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("write_file: parse arguments: %w", err)
	}
	if strings.TrimSpace(params.Path) == "" {
		return "", fmt.Errorf("write_file: path is required")
	}
	if err := ensureParentDir(params.Path); err != nil {
		return "", err
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return fmt.Sprintf("wrote %s (%d bytes)", params.Path, len(params.Content)), nil
}

type EditFileTool struct{}

func (EditFileTool) Name() string { return "edit_file" }

func (EditFileTool) Description() string {
	return "Edit an existing text file by replacing an exact old string with a new string."
}

func (EditFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to edit"},
			"old": {"type": "string", "description": "Exact text to replace"},
			"new": {"type": "string", "description": "Replacement text"},
			"replace_all": {"type": "boolean", "description": "Replace every occurrence instead of the first one"}
		},
		"required": ["path", "old", "new"]
	}`)
}

func (EditFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path       string `json:"path"`
		Old        string `json:"old"`
		New        string `json:"new"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("edit_file: parse arguments: %w", err)
	}
	if strings.TrimSpace(params.Path) == "" {
		return "", fmt.Errorf("edit_file: path is required")
	}
	if params.Old == "" {
		return "", fmt.Errorf("edit_file: old text is required")
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	content := string(data)
	count := strings.Count(content, params.Old)
	if count == 0 {
		return "", fmt.Errorf("edit_file: old text not found")
	}
	n := 1
	if params.ReplaceAll {
		n = -1
	}
	updated := strings.Replace(content, params.Old, params.New, n)
	info, err := os.Stat(params.Path)
	if err != nil {
		return "", fmt.Errorf("edit_file: stat: %w", err)
	}
	if err := os.WriteFile(params.Path, []byte(updated), info.Mode()); err != nil {
		return "", fmt.Errorf("edit_file: write: %w", err)
	}
	replaced := 1
	if params.ReplaceAll {
		replaced = count
	}
	return fmt.Sprintf("edited %s (%d replacement%s)", params.Path, replaced, plural(replaced)), nil
}

type ListFilesTool struct{}

func (ListFilesTool) Name() string { return "list_files" }

func (ListFilesTool) Description() string {
	return "List files and directories under a path. Use recursive=true for a tree-like listing."
}

func (ListFilesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path, defaults to current directory"},
			"recursive": {"type": "boolean", "description": "Whether to walk recursively"},
			"max_items": {"type": "integer", "description": "Maximum entries to return"}
		}
	}`)
}

func (ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
		MaxItems  int    `json:"max_items"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("list_files: parse arguments: %w", err)
		}
	}
	root := params.Path
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	maxItems := bounded(params.MaxItems, defaultMaxItems)

	var entries []string
	if params.Recursive {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if path == root {
				return nil
			}
			if shouldSkipDir(d) {
				return filepath.SkipDir
			}
			entries = append(entries, formatPath(path, d.IsDir()))
			if len(entries) >= maxItems {
				return errStopWalk
			}
			return nil
		})
		if err != nil && err != errStopWalk {
			return "", fmt.Errorf("list_files: %w", err)
		}
	} else {
		items, err := os.ReadDir(root)
		if err != nil {
			return "", fmt.Errorf("list_files: %w", err)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })
		for _, item := range items {
			entries = append(entries, formatPath(filepath.Join(root, item.Name()), item.IsDir()))
			if len(entries) >= maxItems {
				break
			}
		}
	}
	if len(entries) == 0 {
		return "[empty]", nil
	}
	if len(entries) >= maxItems {
		entries = append(entries, "[truncated]")
	}
	return strings.Join(entries, "\n"), nil
}

type SearchTool struct{}

func (SearchTool) Name() string { return "search" }

func (SearchTool) Description() string {
	return "Search text files under a path. Supports substring search or regex=true."
}

func (SearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File or directory path, defaults to current directory"},
			"query": {"type": "string", "description": "Substring or regular expression to find"},
			"regex": {"type": "boolean", "description": "Treat query as a regular expression"},
			"glob": {"type": "string", "description": "Optional filename glob, such as *.go"},
			"max_results": {"type": "integer", "description": "Maximum matching lines to return"}
		},
		"required": ["query"]
	}`)
}

func (SearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path       string `json:"path"`
		Query      string `json:"query"`
		Regex      bool   `json:"regex"`
		Glob       string `json:"glob"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("search: parse arguments: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("search: query is required")
	}
	root := params.Path
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	maxResults := bounded(params.MaxResults, defaultMaxHits)

	var re *regexp.Regexp
	if params.Regex {
		compiled, err := regexp.Compile(params.Query)
		if err != nil {
			return "", fmt.Errorf("search: compile regex: %w", err)
		}
		re = compiled
	}

	var hits []string
	searchFile := func(path string) {
		if len(hits) >= maxResults {
			return
		}
		if params.Glob != "" {
			ok, err := filepath.Match(params.Glob, filepath.Base(path))
			if err != nil || !ok {
				return
			}
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() > maxSearchBytes {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil || isProbablyBinary(data) {
			return
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if matches(line, params.Query, re) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
				if len(hits) >= maxResults {
					return
				}
			}
		}
	}

	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}
	if !info.IsDir() {
		searchFile(root)
	} else {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if shouldSkipDir(d) {
				return filepath.SkipDir
			}
			if d.Type().IsRegular() {
				searchFile(path)
			}
			if len(hits) >= maxResults {
				return errStopWalk
			}
			return nil
		})
		if err != nil && err != errStopWalk {
			return "", fmt.Errorf("search: %w", err)
		}
	}
	if len(hits) == 0 {
		return "no matches", nil
	}
	if len(hits) >= maxResults {
		hits = append(hits, "[truncated]")
	}
	return strings.Join(hits, "\n"), nil
}

type stopWalkError struct{}

func (stopWalkError) Error() string { return "stop walk" }

var errStopWalk error = stopWalkError{}

func sliceLines(content string, start, end int) (string, error) {
	lines := strings.Split(content, "\n")
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) || start > end {
		return "", fmt.Errorf("read_file: invalid line range %d-%d", start, end)
	}
	return strings.Join(lines[start-1:end], "\n"), nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	return nil
}

func bounded(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	if value > fallback {
		return fallback
	}
	return value
}

func shouldSkipDir(d os.DirEntry) bool {
	if !d.IsDir() {
		return false
	}
	switch d.Name() {
	case ".git", "node_modules", "vendor", "bin", "dist", "build":
		return true
	default:
		return false
	}
}

func formatPath(path string, isDir bool) string {
	if isDir {
		return path + string(os.PathSeparator)
	}
	return path
}

func matches(line, query string, re *regexp.Regexp) bool {
	if re != nil {
		return re.MatchString(line)
	}
	return strings.Contains(line, query)
}

func isProbablyBinary(data []byte) bool {
	limit := len(data)
	if limit > 8000 {
		limit = 8000
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
