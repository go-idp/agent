package idp

import "io"

// SetStdin sets the stdin for the command.
func (c *idp) SetStdin(stdin io.Reader) error {
	c.stdin = stdin
	return nil
}

// SetStdout sets the stdout for the command.
func (c *idp) SetStdout(stdout io.Writer) error {
	c.stdout = stdout
	return nil
}

// SetStderr sets the stderr for the command.
func (c *idp) SetStderr(stderr io.Writer) error {
	c.stderr = stderr
	return nil
}
