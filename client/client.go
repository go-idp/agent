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

	"github.com/go-idp/agent/entities"
	"github.com/go-zoox/core-utils/strings"
	"github.com/go-zoox/logger"
	"github.com/go-zoox/safe"
	"github.com/go-zoox/websocket"
)

// Client is the interface of caas client
type Client interface {
	Connect() error
	Exec(command *entities.Command) error
	Close() error
	//
	Output(command *entities.Command) (response string, err error)
	//
	TerminalURL(path ...string) string
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
	Server       string `config:"server"`
	ClientID     string `config:"client_id"`
	ClientSecret string `config:"client_secret"`
	//
	Stdout io.Writer
	Stderr io.Writer
	//
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
	authCh chan struct{}
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
		authCh:    make(chan struct{}),
		closeCh:   make(chan struct{}),
	}
}

func (c *client) Connect() (err error) {
	u, err := url.Parse(c.cfg.Server)
	if err != nil {
		return fmt.Errorf("invalid caas server address: %s", err)
	}
	logger.Debugf("connecting to %s", u.String())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	wc, err := websocket.NewClient(func(opt *websocket.ClientOption) {
		opt.Context = ctx
		opt.Addr = u.String()
	})
	if err != nil {
		cancel()
		return err
	}

	wc.OnClose(func(conn websocket.Conn, code int, message string) error {
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
			c.stderr.Write(message[1:])
			c.exitCode <- 1
		case entities.MessageAuthResponseSuccess:
			c.authCh <- struct{}{}
		default:
			logger.Errorf("unknown message type: %d", message[0])
		}

		return nil
	})

	wc.OnConnect(func(conn websocket.Conn) error {
		cancel()

		// close
		go func() {
			<-c.closeCh
			conn.Close()
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
			}
			err = conn.WriteTextMessage(append([]byte{entities.MessageAuthRequest}, message...))
			if err != nil {
				logger.Errorf("failed to send auth request: %s", err)
			}
		}()

		<-c.authCh

		// heart beat
		go func() {
			for {
				time.Sleep(3 * time.Second)

				logger.Debugf("ping")
				if err := conn.WriteTextMessage([]byte{entities.MessagePing}); err != nil {
					logger.Debugf("failed to send ping: %s", err)
					return
				}
			}
		}()

		// send message
		go func() {
			for {
				select {
				case msg := <-c.messageCh:
					if err := conn.WriteTextMessage(msg); err != nil {
						logger.Errorf("failed to send message: %s", err)
						return
					}
				}
			}
		}()

		return nil
	})

	if err := wc.Connect(); err != nil {
		return err
	}

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
	terminalPath := "/terminal"
	if len(path) > 0 && path[0] != "" {
		terminalPath = path[0]
	}

	u, err := url.Parse(c.cfg.Server)
	if err != nil {
		return ""
	}

	u.Path = terminalPath
	return u.String()
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
