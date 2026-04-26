package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeReadWriteAndEditFileTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sample.txt")

	writeArgs, _ := json.Marshal(map[string]string{
		"path":    path,
		"content": "alpha\nbeta\ngamma\n",
	})
	if result, err := (SafeWriteFileTool{}).Execute(context.Background(), writeArgs); err != nil {
		t.Fatalf("write_file failed: %v", err)
	} else if !strings.Contains(result, "wrote") {
		t.Fatalf("unexpected write result: %s", result)
	}

	readArgs, _ := json.Marshal(map[string]any{
		"path":       path,
		"start_line": 2,
		"end_line":   2,
	})
	content, err := (SafeReadFileTool{}).Execute(context.Background(), readArgs)
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if content != "beta" {
		t.Fatalf("expected selected line, got %q", content)
	}

	editArgs, _ := json.Marshal(map[string]string{
		"path": path,
		"old":  "beta",
		"new":  "BETA",
	})
	if _, err := (EditFileTool{}).Execute(context.Background(), editArgs); err != nil {
		t.Fatalf("edit_file failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if !strings.Contains(string(data), "BETA") {
		t.Fatalf("edit was not applied: %s", string(data))
	}
}

func TestListFilesAndSearchTools(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package demo\nfunc Alpha() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("plain text\n"), 0644); err != nil {
		t.Fatal(err)
	}

	listArgs, _ := json.Marshal(map[string]any{"path": dir})
	listing, err := (ListFilesTool{}).Execute(context.Background(), listArgs)
	if err != nil {
		t.Fatalf("list_files failed: %v", err)
	}
	if !strings.Contains(listing, "a.go") || !strings.Contains(listing, "b.txt") {
		t.Fatalf("listing missed files: %s", listing)
	}

	searchArgs, _ := json.Marshal(map[string]any{
		"path":  dir,
		"query": "Alpha",
		"glob":  "*.go",
	})
	hits, err := (SearchTool{}).Execute(context.Background(), searchArgs)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.Contains(hits, "a.go:2") {
		t.Fatalf("unexpected search hits: %s", hits)
	}
}
