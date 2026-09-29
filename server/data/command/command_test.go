package command

import (
	"testing"

	"github.com/go-idp/agent/entities"
)

// newCommandWithoutRun creates a command which is not run yet, which happens
// when its config fails to be created, e.g. the metadata dir is not writable.
func newCommandWithoutRun(t *testing.T) *Command {
	t.Helper()

	dc, err := New(func(c *Config) {
		c.Command = &entities.Command{}
	})
	if err != nil {
		t.Fatalf("failed to create command: %v", err)
	}

	if dc.State != nil {
		t.Fatalf("expected nil state before run, got: %+v", dc.State)
	}

	return dc
}

func TestIsRunningWithoutRun(t *testing.T) {
	dc := newCommandWithoutRun(t)

	if dc.IsRunning() {
		t.Fatal("expected a command which never runs to be not running")
	}
}

func TestMarkKilledByCloseWithoutRun(t *testing.T) {
	dc := newCommandWithoutRun(t)

	dc.MarkKilledByClose()

	if dc.State != nil {
		t.Fatalf("expected state to stay nil, got: %+v", dc.State)
	}
}

func TestMarkCancelledWithoutRun(t *testing.T) {
	dc := newCommandWithoutRun(t)

	dc.MarkCancelled()

	if dc.State != nil {
		t.Fatalf("expected state to stay nil, got: %+v", dc.State)
	}
}

func TestCancelWithoutRun(t *testing.T) {
	dc := newCommandWithoutRun(t)

	if err := dc.Cancel(); err == nil {
		t.Fatal("expected error when cancelling a command which never runs")
	}

	if dc.State != nil {
		t.Fatalf("expected state to stay nil, got: %+v", dc.State)
	}
}
