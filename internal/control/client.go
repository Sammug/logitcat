package control

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client connects to the daemon's UNIX socket and sends commands.
type Client struct {
	socketPath string
}

// NewClient returns a Client for the given socket path.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// Status returns the daemon's current health metrics.
func (c *Client) Status() (*StatusResponse, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(Command{Type: CmdStatus}); err != nil {
		return nil, err
	}

	var resp StatusResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Reload asks the daemon to hot-reload its configuration.
func (c *Client) Reload() (*AckResponse, error) {
	return c.ack(CmdReload)
}

// Stop asks the daemon to shut down gracefully.
func (c *Client) Stop() (*AckResponse, error) {
	return c.ack(CmdStop)
}

// Tail streams live TailEvents from the daemon, calling fn for each one.
// Blocks until the connection drops or fn returns false.
func (c *Client) Tail(fn func(TailEvent) bool) error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(Command{Type: CmdTail}); err != nil {
		return err
	}

	dec := json.NewDecoder(conn)
	for {
		var event TailEvent
		if err := dec.Decode(&event); err != nil {
			return nil // daemon closed the connection
		}
		if !fn(event) {
			return nil
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (c *Client) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to logwatch daemon: %w", err)
	}
	return conn, nil
}

func (c *Client) ack(cmd CmdType) (*AckResponse, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(Command{Type: cmd}); err != nil {
		return nil, err
	}

	var resp AckResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
