package server

import (
	"github.com/go-zoox/core-utils/safe"

	dcommand "github.com/go-idp/agent/server/data/command"
)

// Commands
var commandsCapacity = 100
var commandsMap = safe.NewMap[string, *dcommand.Command](func(mc *safe.MapConfig) {
	mc.Capacity = commandsCapacity
})
var commandsIDList = safe.NewList[string](func(lc *safe.ListConfig) {
	lc.Capacity = commandsCapacity
})

// type Command struct {
// 	ID      string            `json:"id"`
// 	Command *entities.Command `json:"command"`
// 	State   *CommandState     `json:"state"`
// 	//
// 	connData *ConnData
// }

// type CommandState struct {
// 	Stopped         bool `json:"stopped"`
// 	IsKilledByClose bool `json:"is_killed_by_close"`
// 	IsCancelled     bool `json:"is_cancelled"`
// 	IsCompleted     bool `json:"is_completed"`
// 	IsError         bool `json:"is_error"`
// 	//
// 	Error error `json:"error"`
// }

// func (c *Command) Cancel() error {
// 	if c.connData == nil {
// 		return fmt.Errorf("connData is nil")
// 	}

// 	return c.connData.Cmd.Cancel()
// }
