package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client is a one-shot newline-delimited JSON client. Each Send opens a
// fresh TCP connection, writes the request, reads one response, and closes.
type Client struct {
	addr    string
	timeout time.Duration
}

// NewClient builds a Client targeting the given TCP address.
func NewClient(addr string) *Client {
	return &Client{addr: addr, timeout: 30 * time.Second}
}

// NewClientWithTimeout builds a Client with a custom request timeout.
func NewClientWithTimeout(addr string, timeout time.Duration) *Client {
	return &Client{addr: addr, timeout: timeout}
}

// Send delivers req to the daemon and returns the parsed Response.
func (c *Client) Send(req *Request) (*Response, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	body = append(body, '\n')
	if _, err := conn.Write(body); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &resp, nil
}

// Stream sends req (a streaming command like CmdLogStream or CmdExec) and
// calls handler for each LogEvent received until the connection closes or
// handler returns false. The caller should cancel via context by closing the
// returned stop channel.
func (c *Client) Stream(req *Request, handler func(event LogEvent) bool) error {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	// No overall deadline for streaming connections — they stay open until
	// the server closes them or the caller returns false.
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	body = append(body, '\n')
	if _, err := conn.Write(body); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			// EOF or connection closed — normal end of stream.
			return nil
		}
		var ev LogEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// Skip malformed lines.
			continue
		}
		if !handler(ev) {
			return nil
		}
	}
}

// Ping returns nil if the daemon is reachable on addr.
func Ping(addr string) error {
	c := &Client{addr: addr, timeout: 500 * time.Millisecond}
	_, err := c.Send(&Request{Command: CmdPing})
	return err
}
