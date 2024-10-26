package idp

import (
	"fmt"

	"github.com/go-zoox/logger"
)

func (c *caas) Start() error {
	if err := c.client.Connect(); err != nil {
		logger.Debugf("failed to connect to server: %s", err)
		return fmt.Errorf("failed to connect server(%s)", c.cfg.Server)
	}

	return nil
}
