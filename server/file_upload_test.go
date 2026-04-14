package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-zoox/zoox/defaults"
)

func TestAppendFileAPI_TruncateAndAppend(t *testing.T) {
	app := defaults.Application()
	app.Post("/files/append", appendFileAPI())

	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "demo.txt")

	// truncate write first chunk
	req1 := httptest.NewRequest(
		"POST",
		"/files/append?path="+target+"&truncate=true",
		strings.NewReader("hello "),
	)
	resp1 := httptest.NewRecorder()
	app.ServeHTTP(resp1, req1)
	if resp1.Code != 200 {
		t.Fatalf("expected status 200, got %d, body=%s", resp1.Code, resp1.Body.String())
	}

	// append second chunk
	req2 := httptest.NewRequest(
		"POST",
		"/files/append?path="+target+"&truncate=false",
		strings.NewReader("world"),
	)
	resp2 := httptest.NewRecorder()
	app.ServeHTTP(resp2, req2)
	if resp2.Code != 200 {
		t.Fatalf("expected status 200, got %d, body=%s", resp2.Code, resp2.Body.String())
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	if string(raw) != "hello world" {
		t.Fatalf("unexpected file content: %q", string(raw))
	}
}

func TestAppendFileAPI_PathRequired(t *testing.T) {
	app := defaults.Application()
	app.Post("/files/append", appendFileAPI())

	req := httptest.NewRequest("POST", "/files/append", strings.NewReader("x"))
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != 400 {
		t.Fatalf("expected status 400, got %d, body=%s", resp.Code, resp.Body.String())
	}
}

func TestAppendFileAPI_PathMustBeAbsolute(t *testing.T) {
	app := defaults.Application()
	app.Post("/files/append", appendFileAPI())

	req := httptest.NewRequest(
		"POST",
		"/files/append?path=relative/file.txt&truncate=true",
		strings.NewReader("x"),
	)
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code != 400 {
		t.Fatalf("expected status 400, got %d, body=%s", resp.Code, resp.Body.String())
	}
}

