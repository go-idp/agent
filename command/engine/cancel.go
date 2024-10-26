package idp

func (c *idp) Cancel() error {
	return c.client.Close()
}
