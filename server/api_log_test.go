package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dcommand "github.com/go-idp/agent/server/data/command"
	"github.com/go-zoox/zoox/defaults"
)

func TestReadCommandLog_ReturnsFileContent(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{MetadataDir: tmpDir}
	commandID := "cmd-read-full"

	logDir := filepath.Join(tmpDir, commandID)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	logPath := filepath.Join(logDir, "log")
	if err := os.WriteFile(logPath, []byte("line-1\nline-2\n"), 0o644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	got, err := readCommandLog(cfg, commandID)
	if err != nil {
		t.Fatalf("readCommandLog returned error: %v", err)
	}
	if got != "line-1\nline-2\n" {
		t.Fatalf("unexpected log content: %q", got)
	}
}

func TestReadCommandLogChunk_ReadsIncrementally(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{MetadataDir: tmpDir}
	commandID := "cmd-read-chunk"

	logDir := filepath.Join(tmpDir, commandID)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	logPath := filepath.Join(logDir, "log")
	if err := os.WriteFile(logPath, []byte("abc"), 0o644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	firstChunk, offset, err := readCommandLogChunk(cfg, commandID, 0)
	if err != nil {
		t.Fatalf("readCommandLogChunk(first) returned error: %v", err)
	}
	if firstChunk != "abc" {
		t.Fatalf("unexpected first chunk: %q", firstChunk)
	}
	if offset != int64(len("abc")) {
		t.Fatalf("unexpected first offset: %d", offset)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open log file for append: %v", err)
	}
	if _, err := f.WriteString("def"); err != nil {
		_ = f.Close()
		t.Fatalf("failed to append to log file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close log file: %v", err)
	}

	secondChunk, nextOffset, err := readCommandLogChunk(cfg, commandID, offset)
	if err != nil {
		t.Fatalf("readCommandLogChunk(second) returned error: %v", err)
	}
	if secondChunk != "def" {
		t.Fatalf("unexpected second chunk: %q", secondChunk)
	}
	if nextOffset != offset+int64(len("def")) {
		t.Fatalf("unexpected second offset: %d", nextOffset)
	}
}

func TestRetrieveCommandLogAPI_ReadsFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{MetadataDir: tmpDir}
	commandID := "cmd-api-log"

	logDir := filepath.Join(tmpDir, commandID)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "log"), []byte("from-file-log"), 0o644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	// Keep command in global registry so API can locate it.
	_ = commandsMap.Set(commandID, &dcommand.Command{ID: commandID})
	defer func() {
		_ = commandsMap.Del(commandID)
	}()

	app := defaults.Application()
	app.Get("/commands/:id/log", retrieveCommandLogAPI(cfg))

	req := httptest.NewRequest("GET", "/commands/"+commandID+"/log", nil)
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)

	if resp.Code != 200 {
		t.Fatalf("expected status 200, got %d, body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "from-file-log") {
		t.Fatalf("expected response to include file log, body=%s", resp.Body.String())
	}
}
