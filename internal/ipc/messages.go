// Package ipc defines the request/response wire types and TCP transport used
// between the gopm CLI and the gopmd daemon.
package ipc

import "github.com/thesoftwaremasons/gopm/pkg/config"

// DefaultAddr is the loopback TCP address the daemon listens on.
const DefaultAddr = "127.0.0.1:6119"

// CommandType identifies a daemon RPC.
type CommandType string

// Command names recognised by the daemon.
const (
	CmdStart     CommandType = "start"
	CmdStop      CommandType = "stop"
	CmdRestart   CommandType = "restart"
	CmdList      CommandType = "list"
	CmdLogs      CommandType = "logs"
	CmdLogsAll   CommandType = "logsall"
	CmdLogStream CommandType = "logstream"
	CmdDelete    CommandType = "delete"
	CmdPing      CommandType = "ping"
	CmdShutdown  CommandType = "shutdown"
	CmdExec      CommandType = "exec"
)

// Request is the message the CLI sends to the daemon. AppName, Config, and
// Lines are optional and used by specific commands.
type Request struct {
	Command  CommandType        `json:"command"`
	AppName  string             `json:"app_name,omitempty"`
	Apps     []config.AppConfig `json:"apps,omitempty"`
	Config   *config.AppConfig  `json:"config,omitempty"`
	Lines    int                `json:"lines,omitempty"`
	Follow   bool               `json:"follow,omitempty"`
	All      bool               `json:"all,omitempty"`
	Group    string             `json:"group,omitempty"`
	ExecArgs []string           `json:"exec_args,omitempty"`
	Since    string             `json:"since,omitempty"`
	Grep     string             `json:"grep,omitempty"`
}

// LogEvent is a single log line from a streaming log response.
type LogEvent struct {
	Name string `json:"name"`
	Line string `json:"line"`
}

// ProcessInfo is the per-process status returned by the list command.
type ProcessInfo struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	PID      int     `json:"pid"`
	Restarts int     `json:"restarts"`
	Uptime   string  `json:"uptime"`
	Watching bool    `json:"watching"`
	Memory   uint64  `json:"memory,omitempty"`
	CPU      float64 `json:"cpu,omitempty"`
}

// Response is the daemon's reply to a single Request.
type Response struct {
	OK        bool          `json:"ok"`
	Error     string        `json:"error,omitempty"`
	Processes []ProcessInfo `json:"processes,omitempty"`
	Logs      []string      `json:"logs,omitempty"`
	Message   string        `json:"message,omitempty"`
}
