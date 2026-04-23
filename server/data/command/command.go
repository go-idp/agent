package command

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-idp/agent/entities"
	gzc "github.com/go-zoox/command"
	"github.com/go-zoox/core-utils/safe"
	"github.com/go-zoox/datetime"
	"github.com/go-zoox/eventemitter"
	"github.com/go-zoox/fs"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/uuid"

	tidp "github.com/go-idp/report/tidp"
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

	//
	IsAutoReport bool
	//
	allowReportFunc func(script string, environment map[string]string) bool
}

type State struct {
	StartedAt   *datetime.DateTime `json:"started_at"`
	CompletedAt *datetime.DateTime `json:"completed_at"`
	ErroredAt   *datetime.DateTime `json:"errored_at"`
	//
	// Stopped         bool `json:"stopped"`
	IsKilledByClose bool `json:"is_killed_by_close"`
	//
	IsCancelled bool `json:"is_cancelled"`
	IsCompleted bool `json:"is_completed"`
	IsError     bool `json:"is_error"`
	//
	IsTimeout bool `json:"is_timeout"`
	//
	Error error `json:"error"`
	//
	Status string `json:"status"` // running, cancelled, completed, error
}

type Log struct {
	ID  int    `json:"id"`
	Log string `json:"log"`
	// Timestamp in milliseconds
	TimestampInMS int64 `json:"ts"`
}

func (l Log) String() string {
	bytes, err := json.Marshal(l)
	if err != nil {
		return fmt.Sprintf("failed to marshal command log in server/data/command: %s", err)
	}

	return string(bytes)
}

type Config struct {
	ID string `json:"id"`

	Command *entities.Command `json:"command"`

	IsAutoReport bool
	//
	allowReportFunc func(script string, environment map[string]string) bool
}

func (c *Config) SetAllowReportFunc(f func(script string, environment map[string]string) bool) {
	c.allowReportFunc = f
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
		//
		IsAutoReport: opt.IsAutoReport,
		//
		allowReportFunc: opt.allowReportFunc,
	}, nil
}

func (c *Command) Run() error {
	c.State = &State{
		StartedAt: datetime.Now(),
		Status:    "running",
	}
	// Keep log file as the source of truth to avoid retaining large command output
	// chunks in memory for long-lived command objects.
	c.Log = nil

	workdir := fmt.Sprintf("%s/%s", c.Cmd.WorkDirBase, c.ID)
	if err := fs.Mkdirp(workdir); err != nil {
		return fmt.Errorf("failed to create work dir: %s", err)
	}

	script := c.Cmd.Script
	environment := c.Cmd.Environment
	if environment == nil {
		environment = map[string]string{}
	}

	if c.IsAutoReport {
		isAllowReport := true
		if c.allowReportFunc != nil {
			isAllowReport = c.allowReportFunc(script, environment)
		}

		if isAllowReport {
			approval, err := tidp.Report(&tidp.ReportRequest{
				Script:      script,
				Environment: environment,
			})
			if err == nil {
				if delay := approval.Delay(); delay > 0 {
					time.Sleep(delay)
				}

				if ok := approval.Approved(); !ok {
					// c.event.Emit("error", fmt.Errorf("failed to run command (approval): %s", approval.Reason()))
					return fmt.Errorf("failed to run command (tidp): %s", approval.Reason())
				}

				if injectScriptBefore := approval.InjectScriptsBefore(); injectScriptBefore != "" {
					script = injectScriptBefore + "\n\n" + script
				}

				if injectScriptAfter := approval.InjectScriptsAfter(); injectScriptAfter != "" {
					script = script + "\n\n" + injectScriptAfter
				}

				if injectEvent := approval.InjectEnvironment(); injectEvent != nil {
					for k, v := range injectEvent {
						environment[k] = v
					}
				}
			}
		}
	}

	cmd, err := gzc.New(&gzc.Config{
		Command:     script,
		Shell:       c.Cmd.Shell,
		WorkDir:     workdir,
		Environment: environment,
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

	cmd.SetStdout(c.stdout)
	cmd.SetStderr(c.stderr)

	c.event.Emit("run", c.ID)

	if err := c.cmd.Run(); err != nil {
		if c.State.IsKilledByClose {
			logger.Infof("[command][id: %s] cancelled (connection closed)", c.ID)
			return fmt.Errorf("command is cancelled (connection closed)")
		}

		if c.State.IsCancelled {
			logger.Infof("[command][id: %s] cancelled", c.ID)
			return fmt.Errorf("command is cancelled")
		}

		c.event.Emit("error", fmt.Errorf("failed to run command: %s", err))
		c.State.IsError = true
		c.State.Status = "error"
		c.State.Error = err
		c.State.ErroredAt = datetime.Now()

		logger.Infof("[command][id: %s] failed to run: %s \n\n##### SCRIPT START #####\n%s\n##### SCRIPT START #####\n", c.ID, err.Error(), c.Cmd.Script)

		return fmt.Errorf("failed to run command: %s", err)
	}

	c.event.Emit("complete", c.ID)

	c.State.IsCompleted = true
	c.State.Status = "completed"
	c.State.CompletedAt = datetime.Now()

	logger.Infof("[command][id: %s] succeed to run", c.ID)
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
	return !c.State.IsCancelled && !c.State.IsCompleted && !c.State.IsError
}
