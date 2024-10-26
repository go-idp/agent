package idp

import (
	"os"

	"github.com/go-idp/agent/entities"
)

// Wait waits for the command to finish.
func (c *caas) Wait() error {
	if len(c.cfg.AllowedSystemEnvKeys) != 0 {
		for _, key := range c.cfg.AllowedSystemEnvKeys {
			if c.cfg.Environment[key] == "" {
				if value, ok := os.LookupEnv(key); ok {
					c.cfg.Environment[key] = value
				}
			}
		}
	}

	return c.client.Exec(&entities.Command{
		ID:          c.cfg.ID,
		Script:      c.cfg.Command,
		Environment: c.cfg.Environment,
		// WorkDir:     c.cfg.WorkDir,
		User:  c.cfg.User,
		Shell: c.cfg.Shell,
	})
}
