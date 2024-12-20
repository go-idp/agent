package commands

import (
	"strings"

	"github.com/go-idp/agent/server"
	"github.com/go-zoox/cli"
	"github.com/go-zoox/core-utils/fmt"
	"github.com/go-zoox/debug"
)

func RegistryServer(app *cli.MultipleProgram) {
	app.Register("server", &cli.Command{
		Name:  "server",
		Usage: "idp agent server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Usage:   "server port",
				Aliases: []string{"p"},
				EnvVars: []string{"PORT"},
				// Value:   8838,
			},
			&cli.StringFlag{
				Name:    "shell",
				Usage:   "specify command shell",
				Aliases: []string{"s"},
				EnvVars: []string{"CAAS_SHELL"},
				Value:   "sh",
			},
			&cli.StringFlag{
				Name:    "metadata-dir",
				Usage:   "specify command metadata dir",
				EnvVars: []string{"CAAS_METADATA_DIR"},
				Value:   "/tmp/agent/metadata",
			},
			&cli.StringFlag{
				Name:    "workdir",
				Usage:   "specify command workdir",
				Aliases: []string{"w"},
				EnvVars: []string{"CAAS_WORKDIR"},
				Value:   "/tmp/agent/workdir",
			},
			&cli.StringSliceFlag{
				Name:    "environment",
				Usage:   "specify command environment",
				Aliases: []string{"e"},
				EnvVars: []string{"CAAS_ENVIRONMENT"},
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
			&cli.Int64Flag{
				Name:    "timeout",
				Usage:   "specify command timeout, in seconds, default: 86400 (1d)",
				Aliases: []string{"t"},
				EnvVars: []string{"CAAS_TIMEOUT"},
				Value:   86400,
			},
			&cli.BoolFlag{
				Name:    "daemon",
				Usage:   "Run as a daemon",
				Aliases: []string{"d"},
				EnvVars: []string{"CAAS_DAEMON"},
			},
			&cli.BoolFlag{
				Name:    "disable-clean-workdir",
				Usage:   "Disable clean user workdir, default: false",
				EnvVars: []string{"CAAS_DISABLE_CLEAN_USER_WORKDIR"},
			},
			&cli.BoolFlag{
				Name:    "disable-clean-metadatadir",
				Usage:   "Disable clean metadata dir, default: false",
				EnvVars: []string{"CAAS_DISABLE_CLEAN_METADATADIR"},
			},
			&cli.BoolFlag{
				Name:    "disable-command-cancel-on-close",
				Usage:   "Disable command cancel on close, default: false",
				EnvVars: []string{"CAAS_DISABLE_COMMAND_CANCEL_ON_CLOSE"},
			},
			&cli.StringFlag{
				Name:    "terminal-path",
				Usage:   "specify terminal path",
				EnvVars: []string{"CAAS_TERMINAL_PATH"},
				Value:   "/terminal",
			},
			&cli.StringFlag{
				Name:    "terminal-shell",
				Usage:   "specify terminal shell",
				EnvVars: []string{"CAAS_TERMINAL_SHELL", "SHELL"},
			},
			&cli.StringFlag{
				Name:    "terminal-driver",
				Usage:   "specify terminal container",
				EnvVars: []string{"CAAS_TERMINAL_CONTAINER"},
			},
			&cli.StringFlag{
				Name:    "terminal-driver-image",
				Usage:   "specify terminal container image",
				EnvVars: []string{"CAAS_TERMINAL_CONTAINER_IMAGE"},
			},
			&cli.StringFlag{
				Name:    "terminal-init-command",
				Usage:   "specify terminal init command",
				EnvVars: []string{"CAAS_TERMINAL_INIT_COMMAND"},
			},
			&cli.StringFlag{
				Name:    "terminal-relay",
				Usage:   "specify terminal relay",
				EnvVars: []string{"CAAS_TERMINAL_RELAY"},
			},
			&cli.BoolFlag{
				Name:    "auto-report",
				Usage:   "Auto report command status",
				EnvVars: []string{"CAAS_AUTO_REPORT"},
			},
		},
		Action: func(ctx *cli.Context) (err error) {
			cfg := &server.Config{}
			if err := cli.LoadConfig(ctx, cfg); err != nil {
				return fmt.Errorf("failed to load config file: %v", err)
			}

			if ctx.Int64("port") != 0 {
				cfg.Port = ctx.Int64("port")
			}

			if ctx.String("shell") != "" {
				cfg.Shell = ctx.String("shell")
			}

			if ctx.String("metadata-dir") != "" {
				cfg.MetadataDir = ctx.String("metadata-dir")
			}

			if ctx.String("workdir") != "" {
				cfg.WorkDir = ctx.String("workdir")
			}

			if ctx.String("environment") != "" {
				for _, env := range ctx.StringSlice("environment") {
					if env == "" {
						continue
					}

					cfg.Environment = map[string]string{}

					kv := strings.SplitN(env, "=", 2)
					if len(kv) == 2 {
						cfg.Environment[kv[0]] = kv[1]
					} else {
						cfg.Environment[kv[0]] = ""
					}
				}
			}

			if ctx.Int64("timeout") != 0 {
				cfg.Timeout = ctx.Int64("timeout")
			}

			if ctx.String("client-id") != "" {
				cfg.ClientID = ctx.String("client-id")
			}

			if ctx.String("client-secret") != "" {
				cfg.ClientSecret = ctx.String("client-secret")
			}

			if ctx.Bool("disable-clean-workdir") {
				cfg.IsCleanWorkDirDisabled = ctx.Bool("disable-clean-workdir")
			}

			if ctx.Bool("disable-clean-metadatadir") {
				cfg.IsCleanMetadataDirDisabled = ctx.Bool("disable-clean-metadatadir")
			}

			if ctx.Bool("disable-command-cancel-on-close") {
				cfg.IsCommandCancelOnCloseDisabled = ctx.Bool("disable-command-cancel-on-close")
			}

			if ctx.String("terminal-path") != "" {
				cfg.TerminalPath = ctx.String("terminal-path")
			}

			if ctx.String("terminal-shell") != "" {
				cfg.TerminalShell = ctx.String("terminal-shell")
			}

			if ctx.String("terminal-driver") != "" {
				cfg.TerminalDriver = ctx.String("terminal-driver")
			}

			if ctx.String("terminal-driver-image") != "" {
				cfg.TerminalDriverImage = ctx.String("terminal-driver-image")
			}

			if ctx.String("terminal-init-command") != "" {
				cfg.TerminalInitCommand = ctx.String("terminal-init-command")
			}

			if ctx.String("terminal-relay") != "" {
				cfg.TerminalRelay = ctx.String("terminal-relay")
			}

			if v := ctx.Bool("auto-report"); v {
				cfg.IsAutoReport = true
			}

			if cfg.Port == 0 {
				cfg.Port = 8838
			}

			if debug.IsDebugMode() {
				fmt.PrintJSON("config", cfg)
			}

			if ctx.Bool("daemon") {
				return cli.Daemon(ctx, func() error {
					return server.
						New(cfg).
						Run()
				})
			}

			return server.
				New(cfg).
				Run()
		},
	})
}
