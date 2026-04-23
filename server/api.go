package server

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-idp/agent/entities"
	dcommand "github.com/go-idp/agent/server/data/command"
	"github.com/go-zoox/datetime"
	"github.com/go-zoox/fs"
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
		dc.On("complete", func(payload any) {
			state.Command.Running.Dec(1)
			state.Command.Completed.Inc(1)
		})

		commandsMap.Set(dc.ID, dc)
		commandsIDList.LPush(dc.ID)
		state.Command.Total.Inc(1)

		cmdCfg, err := cfg.GetCommandConfig(dc.ID, commandRequest)
		if err != nil {
			ctx.Fail(fmt.Errorf("failed to get command config: %s", err), 500, "failed to get command config")
			return
		}

		dc.SetStdout(cmdCfg.Log)
		dc.SetStderr(cmdCfg.Log)

		cmdCfg.Script.WriteString(commandRequest.Script)
		cmdCfg.StartAt.WriteString(datetime.Now().Format("YYYY-MM-DD HH:mm:ss"))
		if len(commandRequest.Environment) > 0 {
			env := []string{}
			for k, v := range commandRequest.Environment {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			cmdCfg.Env.WriteString(strings.Join(env, "\n"))
		}

		go func() {
			defer cmdCfg.Log.Close()

			err = dc.Run()
			if err != nil {
				cmdCfg.Error.WriteString(err.Error())
				cmdCfg.FailedAt.WriteString(datetime.Now().Format("YYYY-MM-DD HH:mm:ss"))
				cmdCfg.Status.WriteString("failure")
				fmt.Printf("[createCommandAPI] failed to run command: %s\n", err)
				return
			}

			cmdCfg.SucceedAt.WriteString(datetime.Now().Format("YYYY-MM-DD HH:mm:ss"))
			cmdCfg.Status.WriteString("success")
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

		logContent, err := readCommandLog(cfg, id)
		if err != nil {
			ctx.Fail(err, 500, fmt.Sprintf("failed to read command log: %s", err))
			return
		}

		ctx.Success(zoox.H{
			"log": logContent,
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
		var offset int64
		logEventID := 0

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

				chunk, nextOffset, err := readCommandLogChunk(cfg, id, offset)
				if err != nil {
					ctx.Fail(err, 500, fmt.Sprintf("failed to stream command log: %s", err))
					return
				}
				offset = nextOffset
				if chunk != "" {
					logEventID++
					logEvent := dcommand.Log{
						ID:            logEventID,
						Log:           chunk,
						TimestampInMS: datetime.Now().UnixMilli(),
					}
					ctx.SSE().Event("message", logEvent.String())
				}

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
					logContent, err := readCommandLog(cfg, id)
					if err != nil {
						ctx.Fail(err, 500, fmt.Sprintf("failed to read command log: %s", err))
						return
					}

					ctx.Success(zoox.H{
						"log": logContent,
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
		var offset int64
		logEventID := 0

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

				chunk, nextOffset, err := readCommandLogChunk(cfg, commandID, offset)
				if err != nil {
					ctx.Fail(err, 500, fmt.Sprintf("failed to stream command log: %s", err))
					return
				}
				offset = nextOffset
				if chunk != "" {
					logEventID++
					logEvent := dcommand.Log{
						ID:            logEventID,
						Log:           chunk,
						TimestampInMS: datetime.Now().UnixMilli(),
					}
					ctx.SSE().Event("message", logEvent.String())
				}

				time.Sleep(1 * time.Second)
			}
		}
	}
}

func readCommandLog(cfg *Config, id string) (string, error) {
	logPath := getCommandLogPath(cfg, id)
	if !fs.IsExist(logPath) {
		return "", nil
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func readCommandLogChunk(cfg *Config, id string, offset int64) (string, int64, error) {
	logPath := getCommandLogPath(cfg, id)
	if !fs.IsExist(logPath) {
		return "", offset, nil
	}

	f, err := os.Open(logPath)
	if err != nil {
		return "", offset, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", offset, err
	}

	var chunk strings.Builder
	buf := make([]byte, 32*1024)
	nextOffset := offset
	for {
		n, err := f.Read(buf)
		if n > 0 {
			chunk.Write(buf[:n])
			nextOffset += int64(n)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", offset, err
		}
	}

	return chunk.String(), nextOffset, nil
}

func getCommandLogPath(cfg *Config, id string) string {
	metadataDir := cfg.MetadataDir
	if metadataDir == "" {
		metadataDir = "/tmp/agent/metadata"
	}

	return fmt.Sprintf("%s/%s/log", metadataDir, id)
}
