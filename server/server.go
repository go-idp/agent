package server

import (
	"fmt"
	"os"

	"github.com/go-idp/agent"
	"github.com/go-idp/agent/constants"
	"github.com/go-idp/agent/entities"

	// pipeline "github.com/go-idp/pipeline/svc/server"
	"github.com/go-zoox/chalk"
	"github.com/go-zoox/datetime"
	"github.com/go-zoox/fs"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/proxy/utils/rewriter"
	terminal "github.com/go-zoox/terminal/server"
	"github.com/go-zoox/websocket"
	"github.com/go-zoox/zoox"
	"github.com/go-zoox/zoox/defaults"
	"github.com/go-zoox/zoox/middleware"
)

const DefaultShell = "sh"

// Server is the server interface of caas
type Server interface {
	Run() error
}

// CommandConfig is the configuration of caas command
type CommandConfig struct {
	WorkDir     string
	MetadataDir string

	Script    *WriterFile
	Log       *WriterFile
	Env       *WriterFile
	StartAt   *WriterFile
	SucceedAt *WriterFile
	FailedAt  *WriterFile
	Status    *WriterFile
	Error     *WriterFile
}

func (c *Config) GetCommandConfig(id string, command *entities.Command) (*CommandConfig, error) {
	var isNeedWrite bool
	var oneMetadataDir string

	if c.MetadataDir == "" {
		c.MetadataDir = "/tmp/agent/metadata"
	}

	if c.WorkDir == "" {
		c.WorkDir = "/tmp/agent/workdir"
	}

	oneMetadataDir = fmt.Sprintf("%s/%s", c.MetadataDir, id)
	oneWorkDir := fmt.Sprintf("%s/%s", c.WorkDir, id)
	isNeedWrite = true

	if command.WorkDirBase != "" {
		oneWorkDir = fmt.Sprintf("%s/%s", command.WorkDirBase, id)
	}

	if err := fs.Mkdirp(oneMetadataDir); err != nil {
		return nil, fmt.Errorf("failed to create metadata dir: %s", err)
	}
	if err := fs.Mkdirp(oneWorkDir); err != nil {
		return nil, fmt.Errorf("failed to create work dir: %s", err)
	}

	return &CommandConfig{
		WorkDir:     oneWorkDir,
		MetadataDir: oneMetadataDir,
		Script:      &WriterFile{Path: fmt.Sprintf("%s/script", oneMetadataDir), IsNeedWrite: isNeedWrite},
		Log:         &WriterFile{Path: fmt.Sprintf("%s/log", oneMetadataDir), IsNeedWrite: isNeedWrite},
		Env:         &WriterFile{Path: fmt.Sprintf("%s/env", oneMetadataDir), IsNeedWrite: isNeedWrite},
		StartAt:     &WriterFile{Path: fmt.Sprintf("%s/start_at", oneMetadataDir), IsNeedWrite: isNeedWrite},
		SucceedAt:   &WriterFile{Path: fmt.Sprintf("%s/succeed_at", oneMetadataDir), IsNeedWrite: isNeedWrite},
		FailedAt:    &WriterFile{Path: fmt.Sprintf("%s/failed_at", oneMetadataDir), IsNeedWrite: isNeedWrite},
		Status:      &WriterFile{Path: fmt.Sprintf("%s/status", oneMetadataDir), IsNeedWrite: isNeedWrite},
		Error:       &WriterFile{Path: fmt.Sprintf("%s/error", oneMetadataDir), IsNeedWrite: isNeedWrite},
	}, nil
}

type WriterFile struct {
	Path        string
	IsNeedWrite bool
	//
	file *os.File
}

func (w *WriterFile) Write(p []byte) (n int, err error) {
	if !w.IsNeedWrite {
		return len(p), nil
	}

	if w.file == nil {
		if f, err := os.OpenFile(w.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err != nil {
			logger.Errorf("failed to open file: %s", err)
		} else {
			w.file = f
		}
	}

	if w.file != nil {
		return w.file.Write(p)
	}

	return len(p), nil
}

func (w *WriterFile) Close() error {
	if w.file != nil {
		return w.file.Close()
	}

	return nil
}

func (w *WriterFile) WriteString(content string) {
	if !w.IsNeedWrite {
		return
	}

	if err := fs.WriteFile(w.Path, []byte(content)); err != nil {
		logger.Errorf("failed to write file(%s): %s", w.Path, err)
	}
}

type server struct {
	cfg *Config
}

// New creates a new caas server
func New(cfg *Config) Server {
	if cfg.Port == 0 {
		cfg.Port = 8838
	}

	if cfg.Path == "" {
		cfg.Path = "/"
	}

	if cfg.Shell == "" {
		cfg.Shell = DefaultShell
	}

	return &server{
		cfg: cfg,
	}
}

func (s *server) Run() error {
	app := defaults.Application()

	app.Use(middleware.Prometheus())

	// clean metadata dir at 3:00 every month
	app.Cron().AddJob("clean-metadata", "0 3 * 1 *", func() error {
		if s.cfg.IsCleanMetadataDirDisabled {
			return nil
		}

		if s.cfg.MetadataDir == "" {
			return nil
		}

		logger.Infof("[cronjob] clean metadata dir: %s", s.cfg.MetadataDir)
		if err := fs.RemoveDir(s.cfg.MetadataDir); err != nil {
			return fmt.Errorf("failed to clean metadata dir: %s", err)
		}

		return nil
	})

	// clean work dir at 3:00 every week
	app.Cron().AddJob("clean-workdir", "0 3 * * 6", func() error {
		if s.cfg.IsCleanWorkDirDisabled {
			return nil
		}

		if s.cfg.WorkDir == "" {
			return nil
		}

		logger.Infof("[cronjob] clean work dir: %s", s.cfg.WorkDir)
		if err := fs.RemoveDir(s.cfg.WorkDir); err != nil {
			return fmt.Errorf("failed to clean work dir: %s", err)
		}

		return nil
	})

	wsServer, err := websocket.NewServer()
	if err != nil {
		return err
	}

	createWsService(s.cfg)(wsServer)

	app.WebSocket(s.cfg.Path, func(opt *zoox.WebSocketOption) {
		opt.Server = wsServer
	})

	{ // Web Terminal

		app.Get("/wt", func(ctx *zoox.Context) {
			ctx.HTML(200, terminal.RenderXTerm(zoox.H{
				"wsPath": constants.DefaultTerminalPath,
			}))
		})

		if s.cfg.TerminalRelay != "" {
			app.Proxy(constants.DefaultTerminalPath, s.cfg.TerminalRelay, func(cfg *zoox.ProxyConfig) {
				cfg.Rewrites = []rewriter.Rewriter{
					{
						From: constants.DefaultTerminalPath,
						To:   "/",
					},
				}
			})
		} else {
			server, err := terminal.Serve(&terminal.Config{
				Shell:       s.cfg.TerminalShell,
				Driver:      s.cfg.TerminalDriver,
				DriverImage: s.cfg.TerminalDriverImage,
				InitCommand: s.cfg.TerminalInitCommand,
				Username:    s.cfg.ClientID,
				Password:    s.cfg.ClientSecret,
			})
			if err != nil {
				return fmt.Errorf("failed to create terminal server: %s", err)
			}

			app.WebSocket("/terminal", func(opt *zoox.WebSocketOption) {
				opt.Server = server

				opt.Middlewares = append(opt.Middlewares, func(ctx *zoox.Context) {
					if s.cfg.ClientID == "" && s.cfg.ClientSecret == "" {
						ctx.Next()
						return
					}

					user, pass, ok := ctx.Request.BasicAuth()
					if !ok {
						ctx.Set("WWW-Authenticate", `Basic realm="go-zoox"`)
						ctx.Status(401)
						return
					}

					if !(user == s.cfg.ClientID && pass == s.cfg.ClientSecret) {
						ctx.Status(401)
						return
					}

					ctx.Next()
				})
			})
		}
	}

	// { // Pipeline
	// 	pipeline.Mount(app, func(cfg *pipeline.MountConfig) {
	// 		cfg.Path = constants.DefaultPipelinePath
	// 		cfg.Workdir = s.cfg.WorkDir
	// 		cfg.Environment = s.cfg.Environment
	// 	})
	// }

	app.Post("/exec", func(ctx *zoox.Context) {
		if s.cfg.ClientID == "" && s.cfg.ClientSecret == "" {
			ctx.Next()
			return
		}

		user, pass, ok := ctx.Request.BasicAuth()
		if !ok {
			ctx.Set("WWW-Authenticate", `Basic realm="go-zoox"`)
			ctx.Status(401)
			return
		}

		if !(user == s.cfg.ClientID && pass == s.cfg.ClientSecret) {
			ctx.Status(401)
			return
		}

		ctx.Next()
	}, createCommandAPI(s.cfg))

	app.Group("/commands", func(group *zoox.RouterGroup) {
		group.Use(func(ctx *zoox.Context) {
			if s.cfg.ClientID == "" && s.cfg.ClientSecret == "" {
				ctx.Next()
				return
			}

			user, pass, ok := ctx.Request.BasicAuth()
			if !ok {
				ctx.Set("WWW-Authenticate", `Basic realm="go-zoox"`)
				ctx.Status(401)
				return
			}

			if !(user == s.cfg.ClientID && pass == s.cfg.ClientSecret) {
				ctx.Status(401)
				return
			}

			ctx.Next()
		})

		// latest command
		group.Get("/latest", getLatestCommandAPI(s.cfg))
		group.Get("/latest/log", getLatestCommandLogAPI(s.cfg))
		group.Get("/latest/log/sse", getLatestCommandLogSSEAPI(s.cfg))

		group.Get("/", listCommandsAPI(s.cfg))
		group.Post("/", createCommandAPI(s.cfg))
		group.Get("/:id", retvieveCommandAPI(s.cfg))

		group.Get("/:id/log", retrieveCommandLogAPI(s.cfg))
		group.Get("/:id/log/sse", retrieveCommandLogSSEAPI(s.cfg))

		group.Post("/:id/create", createCommandAPI(s.cfg))
		group.Post("/:id/cancel", cancelCommandAPI(s.cfg))

		// group.Post("/:id/pause", pauseCommandAPI(s.cfg))
		// group.Post("/:id/resume", resumeCommandAPI(s.cfg))

		// group.Post("/:id/start", startCommandAPI(s.cfg))
	})

	runningAt := datetime.Now().Format("YYYY-MM-DD HH:mm:ss")
	app.Get("/", func(ctx *zoox.Context) {
		ctx.JSON(200, zoox.H{
			"title":       "idp agent",
			"description": "the agent of idp",
			"version":     agent.Version,
			"state":       state,
			"running_at":  runningAt,
		})
	})

	app.SetBanner(fmt.Sprintf(`
   _______  ___    ___                __ 
  /  _/ _ \/ _ \  / _ |___ ____ ___  / /_
 _/ // // / ___/ / __ / _ '/ -_) _ \/ __/
/___/____/_/    /_/ |_\_, /\__/_//_/\__/ 
                     /___/                  %s

____________________________________O/_______
                                    O\
`, chalk.Green("v"+agent.Version)))

	return app.Run(fmt.Sprintf("0.0.0.0:%d", s.cfg.Port))
}
