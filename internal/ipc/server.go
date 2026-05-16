package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

// Handler processes a single Request and returns a Response.
type Handler func(ctx context.Context, req *Request) *Response

// StreamHandler handles a long-lived streaming connection (e.g. log follow).
// It is responsible for writing newline-delimited JSON LogEvent lines to conn
// until ctx is cancelled or a write error occurs. The conn is closed by the
// caller after StreamHandler returns.
type StreamHandler func(ctx context.Context, req *Request, conn net.Conn)

// Server is a newline-delimited JSON RPC server bound to a TCP loopback
// address. Each connection handles one request/response and is then closed,
// unless it is a streaming command (CmdLogStream, CmdExec) in which case the
// StreamHandler runs until completion.
type Server struct {
	addr          string
	handler       Handler
	streamHandler StreamHandler

	mu       sync.Mutex
	listener net.Listener
	wg       sync.WaitGroup
}

// NewServer constructs a Server. addr should normally be DefaultAddr.
func NewServer(addr string, h Handler) *Server {
	return &Server{addr: addr, handler: h}
}

// SetStreamHandler registers the handler for streaming commands.
func (s *Server) SetStreamHandler(h StreamHandler) {
	s.mu.Lock()
	s.streamHandler = h
	s.mu.Unlock()
}

// Listen binds the TCP socket. It is separate from Serve so that the daemon
// can fail fast if another instance already holds the port.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("ipc listen %s: %w", s.addr, err)
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()
	return nil
}

// Serve accepts connections in a loop until ctx is cancelled or Close is
// called. It is safe to call once after a successful Listen.
func (s *Server) Serve(ctx context.Context) error {
	s.mu.Lock()
	ln := s.listener
	s.mu.Unlock()
	if ln == nil {
		return errors.New("ipc: Serve called before Listen")
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				s.wg.Wait()
				return nil
			}
			// transient accept error — try again
			continue
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handle(ctx, c)
		}(conn)
	}
}

// Close shuts the listener and waits for in-flight handlers to finish.
func (s *Server) Close() error {
	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	s.wg.Wait()
	return nil
}

func (s *Server) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		writeResp(c, &Response{OK: false, Error: "invalid json: " + err.Error()})
		return
	}

	// Streaming commands bypass the normal request/response cycle.
	if req.Command == CmdLogStream || req.Command == CmdExec {
		s.mu.Lock()
		sh := s.streamHandler
		s.mu.Unlock()
		if sh != nil {
			sh(ctx, &req, c)
		} else {
			writeResp(c, &Response{OK: false, Error: "streaming not supported"})
		}
		return
	}

	resp := s.handler(ctx, &req)
	if resp == nil {
		resp = &Response{OK: true}
	}
	writeResp(c, resp)
}

func writeResp(c net.Conn, resp *Response) {
	enc := json.NewEncoder(c)
	_ = enc.Encode(resp) // Encode appends '\n'
}
