package idp

import (
	"io"
	"os"

	idp "github.com/go-idp/agent/client"
	"github.com/go-zoox/command/engine"
)

// Name is the name of the engine.
const Name = "caas"

type caas struct {
	//
	cfg *Config
	//
	client idp.Client

	//
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// New creates a new caas engine.
func New(cfg *Config) (engine.Engine, error) {
	c := &caas{
		cfg: cfg,
		//
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	if err := c.create(); err != nil {
		return nil, err
	}

	return c, nil
}
