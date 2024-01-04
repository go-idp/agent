package main

import (
	"github.com/go-idp/agent"
	"github.com/go-idp/agent/cmd/agent/commands"
	"github.com/go-zoox/cli"
)

func main() {
	app := cli.NewMultipleProgram(&cli.MultipleProgramConfig{
		Name:    "agent",
		Usage:   "IDP Agent",
		Version: agent.Version,
	})

	// server
	commands.RegistryServer(app)
	// client
	commands.RegistryClient(app)
	// shell
	commands.RegistryShell(app)

	app.Run()
}
