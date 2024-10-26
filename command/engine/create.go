package idp

import (
	"errors"

	idp "github.com/go-idp/agent/client"
)

// create creates the engine.
func (c *caas) create() error {
	if c.client != nil {
		return errors.New("command: already created")
	}

	if c.cfg.Server == "" {
		return errors.New("command: server is required")
	}

	c.client = idp.New(&idp.Config{
		Server:       c.cfg.Server,
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Stdout:       c.stdout,
		Stderr:       c.stderr,
	})

	return nil
}
