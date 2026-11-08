// Package bus connects the hub to the NATS cluster (see docs/adr/0003).
//
// The connection retries forever: a node starts even if NATS is not up yet and
// reports itself not ready until the connection is established.
package bus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/xonoko/robust-home-automation/internal/config"
)

// Client is the hub's connection to NATS.
type Client struct {
	conn *nats.Conn
	js   jetstream.JetStream
	log  *slog.Logger
}

// Connect opens a connection to NATS. It does not wait for the first
// connection to succeed; use Connected to know the current state.
func Connect(cfg config.NATS, name string, log *slog.Logger) (*Client, error) {
	log = log.With("component", "bus")

	opts := []nats.Option{
		nats.Name(name),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(250 * time.Millisecond),
		nats.ReconnectJitter(250*time.Millisecond, time.Second),
		nats.ConnectHandler(func(c *nats.Conn) {
			log.Info("connected to NATS", "server", c.ConnectedUrlRedacted())
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("disconnected from NATS", "error", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("reconnected to NATS", "server", c.ConnectedUrlRedacted())
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			attrs := []any{"error", err}
			if sub != nil {
				attrs = append(attrs, "subject", sub.Subject)
			}
			log.Error("NATS async error", attrs...)
		}),
	}
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	}

	conn, err := nats.Connect(strings.Join(cfg.URLs, ","), opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	return &Client{conn: conn, js: js, log: log}, nil
}

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *nats.Conn { return c.conn }

// JetStream returns the JetStream API.
func (c *Client) JetStream() jetstream.JetStream { return c.js }

// Connected reports whether the client currently has a server connection.
func (c *Client) Connected() bool { return c.conn.IsConnected() }

// Check implements a readiness check.
func (c *Client) Check(context.Context) error {
	if !c.Connected() {
		return errors.New("not connected to NATS")
	}
	return nil
}

// Drain finishes in-flight messages and closes the connection, or closes it
// when ctx expires.
func (c *Client) Drain(ctx context.Context) error {
	if !c.conn.IsConnected() {
		// Nothing can be flushed to a server we cannot reach.
		c.log.Warn("closing NATS connection without draining: not connected")
		c.conn.Close()
		return nil
	}
	closed := make(chan struct{})
	c.conn.SetClosedHandler(func(*nats.Conn) { close(closed) })
	if err := c.conn.Drain(); err != nil {
		c.conn.Close()
		return fmt.Errorf("drain NATS connection: %w", err)
	}
	select {
	case <-closed:
		return nil
	case <-ctx.Done():
		c.conn.Close()
		return fmt.Errorf("drain NATS connection: %w", ctx.Err())
	}
}
