package idp

import "io"

// SetStdin sets the stdin for the command.
func (c *caas) SetStdin(stdin io.Reader) error {
	c.stdin = stdin
	return nil
}

// SetStdout sets the stdout for the command.
func (c *caas) SetStdout(stdout io.Writer) error {
	c.stdout = stdout
	return nil
}

// SetStderr sets the stderr for the command.
func (c *caas) SetStderr(stderr io.Writer) error {
	c.stderr = stderr
	return nil
}
