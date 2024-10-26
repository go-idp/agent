package idp

import (
	"fmt"

	"github.com/go-zoox/logger"
)

func (c *idp) Start() error {
	if err := c.client.Connect(); err != nil {
		logger.Debugf("failed to connect to server: %s", err)
		return fmt.Errorf("failed to connect server(%s): %s", c.cfg.Server, err)
	}

	return nil
}
