package server

import "github.com/go-zoox/core-utils/safe"

type State struct {
	Command CommandState `json:"command"`
}

type CommandState struct {
	Total     *safe.Int `json:"total"`
	Running   *safe.Int `json:"running"`
	Cancelled *safe.Int `json:"cancelled"`
	Error     *safe.Int `json:"error"`
	Completed *safe.Int `json:"completed"`
}

var state = &State{
	Command: CommandState{
		Total:     safe.NewInt(),
		Running:   safe.NewInt(),
		Cancelled: safe.NewInt(),
		Error:     safe.NewInt(),
		Completed: safe.NewInt(),
	},
}
