package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-idp/agent/entities"
	"github.com/go-zoox/logger"
	cst "github.com/go-zoox/logger/components/transport"
	"github.com/go-zoox/logger/transport/console"
	"github.com/go-zoox/websocket"
)

type logCapture struct {
	sync.Mutex
	//
	buf strings.Builder
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.Lock()
	defer c.Unlock()

	return c.buf.Write(p)
}

func (c *logCapture) String() string {
	c.Lock()
	defer c.Unlock()

	return c.buf.String()
}

type logTransport struct {
	capture *logCapture
}

func (t *logTransport) Write(p []byte) (int, error) {
	return t.capture.Write(p)
}

func (t *logTransport) WriteWithLevel(p []byte, level string) (int, error) {
	return t.capture.Write(p)
}

// captureLogs redirects the default logger to memory, so that tests can
// assert what the server logged, e.g. a recovered panic.
func captureLogs(t *testing.T) *logCapture {
	t.Helper()

	capture := &logCapture{}
	if err := logger.SetTransports(map[string]cst.Transport{
		"test": &logTransport{capture: capture},
	}); err != nil {
		t.Fatalf("failed to set log transports: %v", err)
	}

	t.Cleanup(func() {
		logger.SetTransports(map[string]cst.Transport{
			"console": console.New(),
		})
	})

	return capture
}

func waitForLog(t *testing.T, capture *logCapture, substr string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(capture.String(), substr) {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for log(%q), got:\n%s", substr, capture.String())
}

// TestWsCloseAfterCommandConfigFailure reproduces the reported incident:
// the command config fails to be created, e.g. mkdir the metadata dir is
// permission denied, so the command never runs and its state stays nil,
// and the connection close afterwards must not panic.
func TestWsCloseAfterCommandConfigFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("run as non-root to reproduce a permission denied metadata dir")
	}

	capture := captureLogs(t)

	baseDir := t.TempDir()
	readonlyDir := filepath.Join(baseDir, "readonly")
	if err := os.MkdirAll(readonlyDir, 0o500); err != nil {
		t.Fatalf("failed to create readonly dir: %v", err)
	}

	cfg := &Config{
		MetadataDir: filepath.Join(readonlyDir, "metadata"),
		WorkDir:     filepath.Join(baseDir, "workdir"),
	}

	wsServer, err := websocket.NewServer()
	if err != nil {
		t.Fatalf("failed to create websocket server: %v", err)
	}

	createWsService(cfg)(wsServer)

	httpServer := httptest.NewServer(wsServer)
	defer httpServer.Close()

	client, err := websocket.NewClient(func(opt *websocket.ClientOption) {
		opt.Addr = "ws" + strings.TrimPrefix(httpServer.URL, "http")
	})
	if err != nil {
		t.Fatalf("failed to create websocket client: %v", err)
	}

	if err := client.Connect(); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// wait for the connection to be established, so that the state is set
	time.Sleep(200 * time.Millisecond)

	payload, err := json.Marshal(&entities.Command{
		ID:     "ws-close-after-config-failure",
		Script: "echo hello",
	})
	if err != nil {
		t.Fatalf("failed to marshal command: %v", err)
	}

	if err := client.SendTextMessage(append([]byte{entities.MessageCommand}, payload...)); err != nil {
		t.Fatalf("failed to send command: %v", err)
	}

	// the command config fails to be created, so the command never runs
	waitForLog(t, capture, "failed to get command config")

	// client disconnect, then the server handles the close
	if err := client.Close(); err != nil {
		t.Fatalf("failed to close client: %v", err)
	}

	waitForLog(t, capture, "connection close")

	// the close handler is handled asynchronously
	time.Sleep(1 * time.Second)

	if got := capture.String(); strings.Contains(got, "nil pointer dereference") {
		t.Fatalf("connection close panicked:\n%s", got)
	}
}
