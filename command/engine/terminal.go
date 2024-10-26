package idp

import (
	"errors"

	"github.com/go-zoox/command/terminal"
)

// Terminal returns a terminal.
func (c *caas) Terminal() (terminal.Terminal, error) {
	return nil, errors.New("not supported")
}
