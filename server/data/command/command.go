package command

import (
	"fmt"
	"io"
	"time"

	"github.com/go-idp/agent/entities"
	gzc "github.com/go-zoox/command"
	gzio "github.com/go-zoox/core-utils/io"
	"github.com/go-zoox/core-utils/safe"
	"github.com/go-zoox/datetime"
	"github.com/go-zoox/eventemitter"
	"github.com/go-zoox/fs"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/uuid"
)

type Command struct {
	event eventemitter.EventEmitter

	ID string `json:"id"`

	Cmd *entities.Command `json:"command"`

	State *State `json:"state"`

	Log *safe.List[Log] `json:"log"`

	//
	stdout io.Writer
	stderr io.Writer

	cmd gzc.Command
}

type State struct {
	StartedAt  *datetime.DateTime `json:"started_at"`
	FinishedAt *datetime.DateTime `json:"finished_at"`
	ErroredAt  *datetime.DateTime `json:"errored_at"`
	//
	Stopped         bool `json:"stopped"`
	IsKilledByClose bool `json:"is_killed_by_close"`
	IsCancelled     bool `json:"is_cancelled"`
	IsFinished      bool `json:"is_finished"`
	IsError         bool `json:"is_error"`
	IsTimeout       bool `json:"is_timeout"`
	//
	Error error `json:"error"`
	//
	Status string `json:"status"` // running, cancelled, finished, error
}

type Log struct {
	ID        int                `json:"id"`
	Message   string             `json:"message"`
	Timestamp *datetime.DateTime `json:"timestamp"`
}

type Config struct {
	ID string `json:"id"`

	Command *entities.Command `json:"command"`
}

type Option func(*Config)

func New(opts ...Option) (*Command, error) {
	opt := &Config{
		ID: uuid.V4(),
	}
	for _, o := range opts {
		o(opt)
	}

	if opt.Command.ID != "" {
		opt.ID = opt.Command.ID
	}

	return &Command{
		ID:  opt.ID,
		Cmd: opt.Command,
		//
		event: eventemitter.New(),
	}, nil
}

func (c *Command) Run() error {
	c.State.Status = "running"

	c.State = &State{
		StartedAt: datetime.Now(),
	}
	c.Log = safe.NewList[Log](func(lc *safe.ListConfig) {
		lc.Capacity = 10000
	})

	workdir := fmt.Sprintf("%s/%s", c.Cmd.WorkDirBase, c.ID)
	if err := fs.Mkdirp(workdir); err != nil {
		return fmt.Errorf("failed to create work dir: %s", err)
	}

	cmd, err := gzc.New(&gzc.Config{
		Command:     c.Cmd.Script,
		Shell:       c.Cmd.Shell,
		WorkDir:     workdir,
		Environment: c.Cmd.Environment,
		User:        c.Cmd.User,
		Engine:      c.Cmd.Engine,
		Image:       c.Cmd.Image,
		Memory:      c.Cmd.Memory,
		CPU:         c.Cmd.CPU,
		Platform:    c.Cmd.Platform,
		Network:     c.Cmd.Network,
		Privileged:  c.Cmd.Privileged,
		//
		Timeout: time.Duration(c.Cmd.Timeout) * time.Millisecond,
	})
	if err != nil {
		c.event.Emit("error", fmt.Errorf("failed to run command: %s", err))

		c.State.IsError = true
		c.State.Status = "error"
		c.State.Error = err
		c.State.ErroredAt = datetime.Now()

		return fmt.Errorf("failed to run command: %s", err)
	}

	// set cmd to context
	c.cmd = cmd

	if c.stdout == nil {
		return fmt.Errorf("you should call SetStdout(stdout) first")
	}
	if c.stderr == nil {
		return fmt.Errorf("you should call SetStderr(stderr) first")
	}

	line := safe.NewInt()
	logWriter := gzio.WriterWrapFunc(func(p []byte) (n int, err error) {
		// fmt.Println("logWriter:", string(p))
		c.Log.Push(Log{
			ID:        line.Get(),
			Message:   string(p),
			Timestamp: datetime.Now(),
		})

		line.Inc(1)

		// @TODO save to log file, and save to oss
		return len(p), nil
	})
	cmd.SetStdout(io.MultiWriter(c.stdout, logWriter))
	cmd.SetStderr(io.MultiWriter(c.stderr, logWriter))

	c.event.Emit("run", c.ID)

	if err := c.cmd.Run(); err != nil {
		if c.State.IsKilledByClose {
			logger.Infof("[command] killed by close: %s", c.Cmd.Script)
			return nil
		}

		if c.State.IsCancelled {
			logger.Infof("[command] cancelled: %s", c.Cmd.Script)
			return nil
		}

		c.event.Emit("error", fmt.Errorf("failed to run command: %s", err))

		c.State.IsError = true
		c.State.Status = "error"
		c.State.Error = err
		c.State.ErroredAt = datetime.Now()
		return fmt.Errorf("failed to run command: %s", err)
	}

	c.event.Emit("finish", c.ID)

	c.State.IsFinished = true
	c.State.Status = "finished"
	c.State.FinishedAt = datetime.Now()
	return nil
}

func (c *Command) SetStdout(w io.Writer) {
	c.stdout = w
}

func (c *Command) SetStderr(w io.Writer) {
	c.stderr = w
}

func (c *Command) Cancel() error {
	if c.cmd == nil {
		return fmt.Errorf("command is not running, please do Run() first")
	}

	c.State.IsCancelled = true
	c.State.Status = "cancelled"

	c.event.Emit("cancel", c.ID)

	return c.cmd.Cancel()
}

func (c *Command) On(event string, fn func(payload any)) {
	c.event.On(event, eventemitter.HandleFunc(fn))
}

// IsRunning returns true if the command is running
func (c *Command) IsRunning() bool {
	return !c.State.IsCancelled && !c.State.IsFinished && !c.State.IsError
}
