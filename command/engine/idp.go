package idp

import (
	"io"
	"os"

	"github.com/go-idp/agent/client"
	"github.com/go-zoox/command/engine"
)

// Name is the name of the engine.
const Name = "idp"

type idp struct {
	//
	cfg *Config
	//
	client client.Client

	//
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// New creates a new caas engine.
func New(cfg *Config) (engine.Engine, error) {
	c := &idp{
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
