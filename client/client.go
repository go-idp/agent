package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"net/url"

	"github.com/go-idp/agent/constants"
	"github.com/go-idp/agent/entities"
	"github.com/go-idp/pipeline"
	pipelineClient "github.com/go-idp/pipeline/svc/client"
	"github.com/go-zoox/core-utils/strings"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/safe"
	"github.com/go-zoox/websocket"
)

// Client is the interface of caas client
type Client interface {
	Connect() error
	Close() error
	//
	Exec(command *entities.Command) error
	Cancel() error
	//
	Output(command *entities.Command) (response string, err error)
	//
	TerminalURL(path ...string) string
	//
	RunPipeline(p *pipeline.Pipeline) error
}

type ExitError struct {
	ExitCode int
	Message  string
}

func (e ExitError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("exit code: %d, message: %s", e.ExitCode, e.Message)
	}

	return fmt.Sprintf("exit code: %d", e.ExitCode)
}

// Config is the configuration of caas client
type Config struct {
	// Server is the address of caas server
	//	Example:
	//		plain: 				ws://localhost:8838
	//		tls: 					wss://localhost:8838
	//		custom path: 	ws://localhost:8838/custom-path
	Server string `config:"server"`

	// ClientID is the client id
	ClientID string `config:"client_id"`

	// ClientSecret is the client secret
	ClientSecret string `config:"client_secret"`

	// Stdout is the standard output writer
	Stdout io.Writer

	// Stderr is the standard error writer
	Stderr io.Writer

	// ExecTimeout is the timeout of command execution
	ExecTimeout time.Duration `config:"exec_timeout"`
}

type client struct {
	cfg *Config
	//
	exitCode chan int
	//
	stdout io.Writer
	stderr io.Writer
	//
	closeCh chan struct{}
	//
	messageCh chan []byte
	//
	isAuthenticated bool
	authErrCh       chan error
	authDoneCh      chan struct{}
}

// New creates a new caas client
func New(cfg *Config) Client {
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	stderr := cfg.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	if cfg.ExecTimeout == 0 {
		cfg.ExecTimeout = 7 * 24 * time.Hour
	}

	return &client{
		cfg:      cfg,
		exitCode: make(chan int),
		stdout:   stdout,
		stderr:   stderr,
		//
		messageCh: make(chan []byte),
		closeCh:   make(chan struct{}),
		//
		isAuthenticated: false,
		authErrCh:       make(chan error),
		authDoneCh:      make(chan struct{}),
	}
}

func (c *client) Connect() (err error) {
	u, err := url.Parse(c.cfg.Server)
	if err != nil {
		return fmt.Errorf("invalid caas server address: %s", err)
	}
	logger.Debugf("connecting to %s", u.String())

	wc, err := websocket.NewClient(func(opt *websocket.ClientOption) {
		opt.Context = context.Background()
		opt.Addr = u.String()
	})
	if err != nil {
		return err
	}

	wc.OnClose(func(conn websocket.Conn, code int, message string) error {
		if !c.isAuthenticated {
			return nil
		}

		c.stderr.Write([]byte(fmt.Sprintf("connection closed from server: %s\n", message)))
		c.exitCode <- 1
		return nil
	})

	wc.OnTextMessage(func(conn websocket.Conn, message []byte) error {
		switch message[0] {
		case entities.MessageCommandStdout:
			c.stdout.Write(message[1:])
		case entities.MessageCommandStderr:
			c.stderr.Write(message[1:])
		case entities.MessageCommandExitCode:
			c.exitCode <- int(message[1])
		case entities.MessageAuthResponseFailure:
			c.authErrCh <- fmt.Errorf("%s", message[1:])
		case entities.MessageAuthResponseSuccess:
			c.authErrCh <- nil
		case entities.MessageCommandCancelResponse:
			c.stderr.Write([]byte("command canceled\n"))
		default:
			logger.Errorf("unknown message type: %d", message[0])
		}

		return nil
	})

	wc.OnConnect(func(conn websocket.Conn) error {
		ctx, cancel := context.WithCancel(conn.Context())

		// close
		go func() {
			<-c.closeCh
			// logger.Infof("closing connection ...")
			// logger.Infof("canceling context ...")
			cancel()
			// logger.Infof("cancelled context")
			conn.Close()
			// logger.Infof("closed connection")
		}()

		// auth request
		go func() {
			time.Sleep(10 * time.Millisecond)
			authRequest := &entities.AuthRequest{
				ClientID:     c.cfg.ClientID,
				ClientSecret: c.cfg.ClientSecret,
			}
			message, err := json.Marshal(authRequest)
			if err != nil {
				logger.Errorf("failed to marshal auth request: %s", err)
				return
			}

			err = conn.WriteTextMessage(append([]byte{entities.MessageAuthRequest}, message...))
			if err != nil {
				logger.Errorf("failed to send auth request: %s", err)
			}
		}()

		go func() {
			<-c.authDoneCh

			// heart beat
			go func() {
				for {
					select {
					case <-ctx.Done():
						// logger.Infof("stop heart beat ping")
						return
					case <-time.After(3 * time.Second):
						// logger.Infof("heart beat ping ...")
						if err := conn.WriteTextMessage([]byte{entities.MessagePing}); err != nil {
							logger.Errorf("failed to send ping: %s", err)
							return
						}
					}
				}
			}()

			// send message
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case msg := <-c.messageCh:
						if err := conn.WriteTextMessage(msg); err != nil {
							logger.Errorf("failed to send message: %s", err)
							return
						}
					}
				}
			}()
		}()

		return nil
	})

	if err := wc.Connect(); err != nil {
		return err
	}

	if err := <-c.authErrCh; err != nil {
		return err
	}
	c.authDoneCh <- struct{}{}
	c.isAuthenticated = true

	return
}

func (c *client) Exec(command *entities.Command) error {
	go func() {
		time.AfterFunc(c.cfg.ExecTimeout, func() {
			c.stderr.Write([]byte("command exec timeout\n"))
			c.exitCode <- 1
		})
	}()

	message, err := json.Marshal(command)
	if err != nil {
		return &ExitError{
			ExitCode: 1,
			Message:  fmt.Sprintf("failed to marshal command request: %s", err),
		}
	}

	c.messageCh <- append([]byte{entities.MessageCommand}, message...)

	exitCode := <-c.exitCode

	if exitCode == 0 {
		return nil
	}

	return &ExitError{
		ExitCode: exitCode,
	}
}

func (c *client) Cancel() error {
	c.messageCh <- []byte{entities.MessageCommandCancelRequest}

	exitCode := <-c.exitCode

	if exitCode == 0 {
		return nil
	}

	close(c.exitCode)

	return &ExitError{
		ExitCode: exitCode,
	}
}

func (c *client) Output(command *entities.Command) (response string, err error) {
	responseBuf := NewBufWriter()

	c.stdout = responseBuf
	c.stderr = responseBuf
	if err = c.Exec(command); err != nil {
		return strings.TrimSpace(responseBuf.String()), nil
	}

	return strings.TrimSpace(responseBuf.String()), nil
}

func (c *client) Close() error {
	return safe.Do(func() error {
		c.closeCh <- struct{}{}
		close(c.closeCh)
		return nil
	})
}

func (c *client) TerminalURL(path ...string) string {
	terminalPath := constants.DefaultTerminalPath
	if len(path) > 0 && path[0] != "" {
		terminalPath = path[0]
	}

	u, err := url.Parse(c.cfg.Server)
	if err != nil {
		return ""
	}

	if strings.EndsWith(u.Path, "/") {
		u.Path = u.Path[:len(u.Path)-1]
	}
	u.Path = u.Path + terminalPath

	return u.String()
}

func (c *client) RunPipeline(p *pipeline.Pipeline) error {
	pc := pipelineClient.New(&pipelineClient.Config{
		Server:   c.cfg.Server,
		Username: c.cfg.ClientID,
		Password: c.cfg.ClientSecret,
		Path:     constants.DefaultPipelinePath,
	})

	pc.SetStdout(c.stdout)
	pc.SetStderr(c.stderr)

	if err := pc.Connect(); err != nil {
		return err
	}
	defer pc.Close()

	return pc.Run(p)
}

func NewBufWriter() *BufWriter {
	return &BufWriter{
		buf: &bytes.Buffer{},
	}
}

type BufWriter struct {
	io.Writer
	buf *bytes.Buffer
}

func (w *BufWriter) Write(p []byte) (n int, err error) {
	return w.buf.Write(p)
}

func (w *BufWriter) String() string {
	return w.buf.String()
}
