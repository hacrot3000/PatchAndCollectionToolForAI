package broker

const ProtocolVersion = 1

const CapabilitySessionTitle = "session_title"
const CapabilityPatchProtocolEvents = "patch_protocol_events"

// Info is the durable identity advertised by the session broker. The daemon
// validates both ProtocolVersion and Workspace before trusting a discovered
// broker after restart/self-update.
type Info struct {
	ProtocolVersion int    `json:"protocol_version"`
	PID             int    `json:"pid"`
	Workspace       string `json:"workspace"`
	SocketPath      string `json:"socket_path"`
	StartedAt       string   `json:"started_at"`
	Capabilities    []string `json:"capabilities,omitempty"`
}

// ExecutionSpec is the broker wire representation of tasks.Execution. Env is
// explicit here because tasks.Execution intentionally excludes it from normal
// JSON serialization.
type ExecutionSpec struct {
	TaskID  int      `json:"task_id"`
	Label   string   `json:"label"`
	Detail  string   `json:"detail,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Cwd     string   `json:"cwd"`
	Env     []string `json:"env"`
	Preview        string   `json:"preview"`
	ProtocolEvents bool     `json:"protocol_events,omitempty"`
}

type ResizeRequest struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}
