package server

import (
	"fmt"
	"os"
	"time"

	"github.com/go-idp/agent/entities"
	dcommand "github.com/go-idp/agent/server/data/command"
	"github.com/go-zoox/uuid"
	"github.com/go-zoox/zoox"
)

func createCommandAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		commandRequest := &entities.Command{}
		if err := ctx.BindJSON(commandRequest); err != nil {
			ctx.JSON(400, zoox.H{"error": err.Error()})
			return
		}

		if commandRequest.Engine == "" {
			ctx.JSON(400, zoox.H{"error": "engine is required"})
			return
		}

		if commandRequest.Script == "" {
			ctx.JSON(400, zoox.H{"error": "script is required"})
			return
		}

		if commandRequest.ID == "" {
			commandRequest.ID = uuid.V4()
		}

		dc, err := dcommand.New(func(c *dcommand.Config) {
			c.ID = commandRequest.ID

			c.Command = commandRequest

			// cfg.Timeout is seconds, but command.Timeout is milliseconds
			if cfg.Timeout != 0 {
				c.Command.Timeout = cfg.Timeout * 1000
			}

			// fix workdir
			if c.Command.WorkDirBase == "" {
				c.Command.WorkDirBase = cfg.WorkDir
			}

			if cfg.Environment != nil {
				for k, v := range cfg.Environment {
					c.Command.Environment[k] = v
				}
			}
		})
		if err != nil {
			ctx.Fail(fmt.Errorf("failed to create data command: %s", err), 500, "failed to create data command")
			return
		}

		// set listener
		dc.On("error", func(payload any) {
			state.Command.Running.Dec(1)
			state.Command.Error.Inc(1)
		})
		dc.On("run", func(payload any) {
			state.Command.Running.Inc(1)
		})
		dc.On("cancel", func(payload any) {
			state.Command.Running.Dec(1)
			state.Command.Cancelled.Inc(1)
		})
		dc.On("finish", func(payload any) {
			state.Command.Running.Dec(1)
			state.Command.Finished.Inc(1)
		})

		commandsMap.Set(dc.ID, dc)
		commandsIDList.LPush(dc.ID)
		state.Command.Total.Inc(1)

		dc.SetStdout(os.Stdout)
		dc.SetStderr(os.Stderr)

		go func() {
			err = dc.Run()
			if err != nil {
				fmt.Printf("[createCommandAPI] failed to run command: %s\n", err)
			}
		}()

		ctx.Success(zoox.H{
			"id": commandRequest.ID,
		})
	}
}

func listCommandsAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		commands := []any{}
		for _, id := range commandsIDList.Iterator() {
			commands = append(commands, commandsMap.Get(id))
		}

		ctx.Success(zoox.H{
			"total": len(commands),
			"data":  commands,
		})
	}
}

func retvieveCommandAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		id := ctx.Param().Get("id").String()
		if id == "" {
			ctx.Fail(fmt.Errorf("id is required"), 400, "id is required")
			return
		}

		commandX := commandsMap.Get(id)
		if commandX == nil {
			ctx.Fail(nil, 404, "command not found")
			return
		}

		ctx.Success(commandX)
	}
}

func retrieveCommandLogAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		id := ctx.Param().Get("id").String()
		if id == "" {
			ctx.Fail(fmt.Errorf("id is required"), 400, "id is required")
			return
		}

		command := commandsMap.Get(id)
		if command == nil {
			ctx.Fail(nil, 404, "command not found")
			return
		}

		ctx.Success(zoox.H{
			"log": command.Log,
		})
	}
}

func retrieveCommandLogSSEAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		id := ctx.Param().Get("id").String()
		if id == "" {
			ctx.Fail(fmt.Errorf("id is required"), 400, "id is required")
			return
		}

		for {
			select {
			case <-ctx.Request.Context().Done():
				return
			case <-time.After(10 * time.Minute):
				// max 10 minutes, avoid memory leak
				return
			default:
				command := commandsMap.Get(id)
				if command == nil {
					ctx.Fail(nil, 404, "command not found")
					return
				}

				ctx.SSE().Event("message", command.Log.Pop().String())

				time.Sleep(1 * time.Second)
			}
		}
	}
}

func cancelCommandAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		id := ctx.Param().Get("id").String()
		if id == "" {
			ctx.Fail(fmt.Errorf("id is required"), 400, "id is required")
			return
		}

		command := commandsMap.Get(id)
		if command == nil {
			ctx.Fail(nil, 404, "command not found")
			return
		}

		if err := command.Cancel(); err != nil {
			ctx.Fail(err, 500, fmt.Sprintf("failed to cancel command: %s", err))
			return
		}

		ctx.Success(nil)
	}
}

func getLatestCommandAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		if state.Command.Running.Get() == 0 {
			ctx.Fail(nil, 200, "no commands running")
			return
		}

		for _, id := range commandsIDList.Iterator() {
			if command := commandsMap.Get(id); command != nil {
				if command.IsRunning() {
					ctx.Success(command)
					return
				}
			}
		}
	}
}

func getLatestCommandLogAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		if state.Command.Running.Get() == 0 {
			ctx.Fail(nil, 200, "no commands running")
			return
		}

		for _, id := range commandsIDList.Iterator() {
			if command := commandsMap.Get(id); command != nil {
				if command.IsRunning() {
					ctx.Success(zoox.H{
						"log": command.Log,
					})
					return
				}
			}
		}
	}
}

func getLatestCommandLogSSEAPI(cfg *Config) func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		if state.Command.Running.Get() == 0 {
			ctx.Fail(nil, 200, "no commands running")
			return
		}

		commandID := ""
		for _, id := range commandsIDList.Iterator() {
			if command := commandsMap.Get(id); command != nil {
				if command.IsRunning() {
					commandID = id
					break
				}
			}
		}

		if commandID == "" {
			ctx.Fail(nil, 404, "command current is not running")
			return
		}

		for {
			select {
			case <-ctx.Request.Context().Done():
				return
			case <-time.After(10 * time.Minute):
				// max 10 minutes, avoid memory leak
				return
			default:
				command := commandsMap.Get(commandID)
				if command == nil {
					ctx.Fail(nil, 404, "command not found")
					return
				}

				if running := command.IsRunning(); !running {
					return
				}

				ctx.SSE().Event("message", command.Log.Pop().String())

				time.Sleep(1 * time.Second)
			}
		}
	}
}
