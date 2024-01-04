package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/go-idp/agent/client"
	"github.com/go-idp/agent/entities"
	"github.com/go-zoox/cli"
	"github.com/go-zoox/core-utils/regexp"
	"github.com/go-zoox/fetch"
	"github.com/go-zoox/fs"
	"github.com/go-zoox/logger"
)

func RegistryShell(app *cli.MultipleProgram) {
	app.Register("shell", &cli.Command{
		Name:  "shell",
		Usage: "idp agent shell",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "server",
				Usage:   "server url",
				Aliases: []string{"s"},
				EnvVars: []string{"CAAS_SERVER"},
				// Required: true,
				Value: "127.0.0.1",
			},
			&cli.StringFlag{
				Name:    "script",
				Usage:   "specify command script",
				EnvVars: []string{"CAAS_SCRIPT"},
				// Required: true,
			},
			&cli.StringFlag{
				Name:    "client-id",
				Usage:   "Auth Client ID",
				EnvVars: []string{"CAAS_CLIENT_ID"},
			},
			&cli.StringFlag{
				Name:    "client-secret",
				Usage:   "Auth Client Secret",
				EnvVars: []string{"CAAS_CLIENT_SECRET"},
			},
			//
			&cli.StringFlag{
				Name:    "envfile",
				Usage:   "specify command envfile file path",
				EnvVars: []string{"CAAS_ENV_FILE"},
			},
		},
		Action: func(ctx *cli.Context) (err error) {
			cfg := &client.Config{}
			if err := cli.LoadConfig(ctx, cfg); err != nil {
				return fmt.Errorf("failed to load config file: %v", err)
			}

			if ctx.String("server") != "" {
				cfg.Server = ctx.String("server")
			}

			if ctx.String("client-id") != "" {
				cfg.ClientID = ctx.String("client-id")
			}

			if ctx.String("client-secret") != "" {
				cfg.ClientSecret = ctx.String("client-secret")
			}

			// add scheme
			if !regexp.Match("^wss?://", cfg.Server) {
				cfg.Server = fmt.Sprintf("ws://%s", cfg.Server)
			}

			// add port
			if !regexp.Match(":\\d+$", cfg.Server) {
				// host:port
				cfg.Server = fmt.Sprintf("%s:8838", cfg.Server)
			}

			if !regexp.Match("^ws://[^:]+:\\d+", cfg.Server) {
				return fmt.Errorf("invalid agent server: %s", cfg.Server)
			}

			environment := map[string]string{}

			if envfilePath := ctx.String("envfile"); envfilePath != "" {
				envText := ""
				if regexp.Match("^https?://", envfilePath) {
					response, err := fetch.Get(envfilePath)
					if err != nil {
						return fmt.Errorf("failed to fetch script file: %s", err)
					}

					envText = response.String()
				} else {
					if ok := fs.IsExist(envfilePath); !ok {
						return fmt.Errorf("envfile path not found: %s", envfilePath)
					}

					if envTextX, err := fs.ReadFileAsString(envfilePath); err != nil {
						return fmt.Errorf("failed to read script file: %s", err)
					} else {
						envText = envTextX
					}
				}

				lines := strings.Split(envText, "\n")

				for _, line := range lines {
					if line == "" {
						continue
					}

					if line[0] == '#' {
						continue
					}

					parts := strings.SplitN(line, "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid envfile line: %s", line)
					}

					environment[parts[0]] = parts[1]
				}
			}

			c := client.New(cfg)
			if err := c.Connect(); err != nil {
				logger.Debugf("failed to connect to server: %s", err)
				return fmt.Errorf("server(%s) is not running", ctx.String("server"))
			}

			runCommand := func(cmd string) error {
				return c.Exec(&entities.Command{
					Script:      cmd,
					Environment: environment,
				})
			}

			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("$ ")
				cmdString, err := reader.ReadString('\n')
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
				err = runCommand(cmdString)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}
		},
	})
}
