// Package control defines the IPC protocol between the logwatch daemon and CLI.
// Communication happens over a UNIX domain socket using newline-delimited JSON.
package control

// CmdType identifies the CLI command sent to the daemon.
type CmdType string

const (
	CmdStatus CmdType = "status"
	CmdReload CmdType = "reload"
	CmdStop   CmdType = "stop"
	CmdTail   CmdType = "tail"
)

// Command is the request envelope sent by the CLI.
type Command struct {
	Type CmdType `json:"type"`
}

// StatusResponse carries daemon health information.
type StatusResponse struct {
	PID        int      `json:"pid"`
	Uptime     string   `json:"uptime"`
	ConfigPath string   `json:"config_path"`
	Files      []string `json:"files"`
	RuleCount  int      `json:"rule_count"`
	AlertCount int64    `json:"alert_count"`
}

// AckResponse is the generic success/error reply.
type AckResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// TailEvent is a single streamed alert pushed to tail subscribers.
type TailEvent struct {
	Time     string            `json:"time"`
	Severity string            `json:"severity"`
	Rule     string            `json:"rule"`
	Level    string            `json:"level"`
	Source   string            `json:"source"`
	Message  string            `json:"message"`
	Format   string            `json:"format"`
	Raw      string            `json:"raw"`
	Fields   map[string]string `json:"fields,omitempty"`
}
