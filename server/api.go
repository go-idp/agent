package server

import (
	"fmt"
	"net/http"

	"github.com/go-idp/agent/entities"
	"github.com/go-zoox/command"
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

		cmdCfg, err := cfg.GetCommandConfig(commandRequest.ID, commandRequest)
		if err != nil {
			ctx.Fail(err, http.StatusInternalServerError, fmt.Sprintf("failed to get command config: %s", err))
			return
		}

		cmd, err := command.New(&command.Config{
			Command:     commandRequest.Script,
			Shell:       cfg.Shell,
			WorkDir:     cmdCfg.WorkDir,
			Environment: commandRequest.Environment,
			User:        commandRequest.User,
			Engine:      commandRequest.Engine,
			Image:       commandRequest.Image,
			Memory:      commandRequest.Memory,
			CPU:         commandRequest.CPU,
			Platform:    commandRequest.Platform,
			Network:     commandRequest.Network,
			Privileged:  commandRequest.Privileged,
		})
		if err != nil {
			ctx.Fail(err, http.StatusInternalServerError, fmt.Sprintf("failed to create command (1): %s", err))
			return
		}

		cmd.SetStdout(cmdCfg.Log)
		cmd.SetStderr(cmdCfg.Log)

		if err := cmd.Run(); err != nil {
			ctx.Fail(err, http.StatusInternalServerError, fmt.Sprintf("failed to run command: %s", err))
			return
		}

		log, err := fs.ReadFileAsString(cmdCfg.Log.Path)
		if err != nil {
			ctx.Fail(err, http.StatusInternalServerError, fmt.Sprintf("failed to read log: %s", err))
			return
		}

		// @TODO
		if log[len(log)-1] == '\n' {
			log = log[:len(log)-1]
		}

		ctx.Success(zoox.H{
			"id":  commandRequest.ID,
			"log": log,
		})
	}
}
