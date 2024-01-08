package server

import (
	"encoding/json"
	"fmt"
	"io"

	// "os/exec"
	"strings"
	"time"

	"github.com/go-idp/agent/entities"
	"github.com/go-zoox/command"
	"github.com/go-zoox/command/errors"
	"github.com/go-zoox/datetime"
	"github.com/go-zoox/fs"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/websocket"
	"github.com/go-zoox/websocket/conn"
)

// WSClientWriter is the writer for websocket client
type WSClientWriter struct {
	io.Writer
	Conn websocket.Conn
	Flag byte
}

func (w WSClientWriter) Write(p []byte) (n int, err error) {
	if err := w.Conn.WriteTextMessage(append([]byte{w.Flag}, p...)); err != nil {
		return 0, err
	}

	return len(p), nil
}

type ConnData struct {
	Cmd                        command.Command
	AuthClient                 *entities.AuthRequest
	CommandN                   *entities.Command
	IsAuthenticated            bool
	Stopped                    bool
	IsKilledByClose            bool
	AuthenticationTimeoutTimer *time.Timer
	HeartbeatTimeoutTimer      *time.Timer
}

func createWsService(cfg *Config) func(server websocket.Server) {
	heartbeatTimeout := 30 * time.Second
	authenticator := createAuthenticator(cfg)

	return func(server websocket.Server) {
		server.OnConnect(func(conn conn.Conn) error {
			data := &ConnData{}
			if cfg.ClientID == "" && cfg.ClientSecret == "" && cfg.AuthService == "" {
				data.IsAuthenticated = true
			}

			data.AuthenticationTimeoutTimer = time.AfterFunc(30*time.Second, func() {
				if !data.IsAuthenticated {
					logger.Debugf("[ws][id: %s] authentication timeout", conn.ID())

					conn.Close()
				}
			})
			data.HeartbeatTimeoutTimer = time.AfterFunc(heartbeatTimeout, func() {
				logger.Debugf("[ws][id: %s] heart beat timeout", conn.ID())

				conn.Close()
			})

			conn.Set("state", data)

			logger.Debugf("[ws][id: %s] connect", conn.ID())
			return nil
		})

		server.OnClose(func(conn conn.Conn, code int, message string) error {
			logger.Debugf("[ws][id: %s] Close (code: %d, message: %s)", conn.ID(), code, message)

			data, ok := conn.Get("state").(*ConnData)
			if !ok {
				return fmt.Errorf("failed to get state")
			}

			if data.Cmd != nil && !data.Stopped {
				if !cfg.IsCommandCancelOnCloseDisable {
					data.IsKilledByClose = true
					if data.Cmd != nil {
						data.Cmd.Cancel()
					}
				}
			}

			return nil
		})

		server.OnTextMessage(func(conn websocket.Conn, msg []byte) error {
			go func(conn websocket.Conn, msg []byte) (err error) {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[ws][id: %s] receive text message panic => %v", conn.ID(), r)
						conn.WriteTextMessage(append([]byte{entities.MessageCommandStderr}, []byte(fmt.Sprintf("internal server error: %v\n", r))...))
						conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(1)})
						return
					}
				}()

				data, ok := conn.Get("state").(*ConnData)
				if !ok {
					return fmt.Errorf("failed to get state")
				}

				switch msg[0] {
				case entities.MessagePing:
					logger.Debugf("[ws][id: %s] receive ping", conn.ID())
					data.HeartbeatTimeoutTimer.Reset(heartbeatTimeout)
					return nil
				case entities.MessageAuthRequest:
					logger.Infof("[ws][id: %s] auth request", conn.ID())
					data.AuthClient = &entities.AuthRequest{}
					if err := json.Unmarshal(msg[1:], data.AuthClient); err != nil {
						logger.Errorf("[ws][id: %s] failed to unmarshal auth request: %s", conn.ID(), err)
						return nil
					}
					data.AuthenticationTimeoutTimer.Stop()
					if err := authenticator(data.AuthClient.ClientID, data.AuthClient.ClientSecret); err != nil {
						logger.Errorf("[ws][id: %s] failed to authenticate => %v", conn.ID(), err)

						conn.WriteTextMessage(append([]byte{entities.MessageAuthResponseFailure}, []byte(fmt.Sprintf("failed to authenticate: %s\n", err))...))
						conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(1)})
						conn.Close()
						return nil
					}

					data.IsAuthenticated = true
					logger.Infof("[ws][id: %s] authenticated", conn.ID())
					conn.WriteTextMessage([]byte{entities.MessageAuthResponseSuccess})
				case entities.MessageCommand:
					if !data.IsAuthenticated {
						logger.Errorf("[ws][id: %s] not authenticated", conn.ID())
						conn.WriteTextMessage(append([]byte{entities.MessageCommandStderr}, []byte("not authenticated\n")...))
						conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(1)})
						conn.Close()
						return nil
					}

					commandN := &entities.Command{}
					data.CommandN = commandN
					tmpScriptFilepath := ""
					if err := json.Unmarshal(msg[1:], commandN); err != nil {
						logger.Errorf("failed to unmarshal command request: %s", err)

						conn.WriteTextMessage(append([]byte{entities.MessageCommandStderr}, []byte("invalid command request\n")...))
						conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(1)})
						return nil
					}

					id := conn.ID()
					if commandN.ID != "" {
						id = commandN.ID
					}
					cmdCfg, err := cfg.GetCommandConfig(id, commandN)
					if err != nil {
						logger.Errorf("failed to get command config: %s", err)
						conn.WriteTextMessage(append([]byte{entities.MessageCommandStderr}, []byte("internal server error\n")...))
						conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(1)})
						return nil
					}
					defer func() {
						// @TODO clean workdir
						if cfg.IsAutoCleanWorkDir {
							if fs.IsExist(cmdCfg.WorkDir) {
								logger.Infof("[command] clean work dir: %s", cmdCfg.WorkDir)
								if err := fs.Remove(cmdCfg.WorkDir); err != nil {
									panic(fmt.Errorf("failed to clean workdir(%s): %s", cmdCfg.WorkDir, err))
								}
							}
						}
					}()

					env := []string{}
					environment := map[string]string{
						// "HOME":    os.Getenv("HOME"),
						// "USER":    os.Getenv("USER"),
						// "LOGNAME": os.Getenv("LOGNAME"),
						// "SHELL":   cfg.Shell,
						// "TERM":    os.Getenv("TERM"),
						// "PATH":    os.Getenv("PATH"),
					}
					if commandN.Environment != nil {
						for k, v := range commandN.Environment {
							environment[k] = v
						}
					}
					if cfg.Environment != nil {
						for k, v := range cfg.Environment {
							environment[k] = v
						}
					}
					for k, v := range environment {
						env = append(env, fmt.Sprintf("%s=%s", k, v))
					}

					cmd, err := command.New(&command.Config{
						Command:     commandN.Script,
						Shell:       cfg.Shell,
						WorkDir:     cmdCfg.WorkDir,
						Environment: environment,
						User:        commandN.User,
						Engine:      commandN.Engine,
						Image:       commandN.Image,
						Memory:      commandN.Memory,
						CPU:         commandN.CPU,
						Platform:    commandN.Platform,
						Network:     commandN.Network,
						Privileged:  commandN.Privileged,
					})
					if err != nil {
						panic(fmt.Errorf("failed to create command (1): %s", err))
					}
					data.Cmd = cmd

					// timeout
					var commandTimeoutTimer *time.Timer
					if cfg.Timeout != 0 {
						commandTimeoutTimer = time.AfterFunc(time.Duration(cfg.Timeout)*time.Second, func() {
							if cmd != nil {
								cmd.Cancel()
							}
						})
					}

					cmd.SetStdout(io.MultiWriter(cmdCfg.Log, &WSClientWriter{Conn: conn, Flag: entities.MessageCommandStdout}))
					cmd.SetStderr(io.MultiWriter(cmdCfg.Log, &WSClientWriter{Conn: conn, Flag: entities.MessageCommandStderr}))

					logger.Infof("[command] start to run: %s", commandN.Script)
					cmdCfg.Script.WriteString(commandN.Script)
					cmdCfg.Env.WriteString(strings.Join(env, "\n"))
					cmdCfg.StartAt.WriteString(datetime.Now().Format("YYYY-MM-DD HH:mm:ss"))
					err = cmd.Run()
					if err != nil {
						if data.IsKilledByClose {
							logger.Infof("[command] killed by Close: %s", commandN.Script)
							return nil
						}

						cmdCfg.FailedAt.WriteString(datetime.Now().Format("YYYY-MM-DD HH:mm:ss"))
						cmdCfg.Error.WriteString(err.Error())
						cmdCfg.Status.WriteString("failure")

						exitCode := -1
						if errx, ok := err.(*errors.ExitError); ok {
							exitCode = errx.ExitCode()
						}

						logger.Errorf("[command] failed to run: %s (err: %v, exit code: %d)", commandN.Script, err, exitCode)
						conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(exitCode)})
						return nil
					}

					cmdCfg.SucceedAt.WriteString(datetime.Now().Format("YYYY-MM-DD HH:mm:ss"))
					cmdCfg.Status.WriteString("success")
					logger.Infof("[command] succeed to run: %s", commandN.Script)

					conn.WriteTextMessage([]byte{entities.MessageCommandExitCode, byte(0)})

					if tmpScriptFilepath != "" && fs.IsExist(tmpScriptFilepath) {
						if err := fs.Remove(tmpScriptFilepath); err != nil {
							panic(fmt.Errorf("failed to remove tmp script file: %s", err))
						}
					}

					if commandTimeoutTimer != nil {
						commandTimeoutTimer.Stop()
					}

					if data.HeartbeatTimeoutTimer != nil {
						data.HeartbeatTimeoutTimer.Stop()
					}

					data.Stopped = true
				default:
					logger.Errorf("unknown message type: %d", msg[0])
				}

				return nil
			}(conn, msg)

			return nil
		})
	}
}
