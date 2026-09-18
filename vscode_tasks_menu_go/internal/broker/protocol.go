package broker

const ProtocolVersion = 1

// Info is the durable identity advertised by the session broker. The daemon
// validates both ProtocolVersion and Workspace before trusting a discovered
// broker after restart/self-update.
type Info struct {
	ProtocolVersion int    `json:"protocol_version"`
	PID             int    `json:"pid"`
	Workspace       string `json:"workspace"`
	SocketPath      string `json:"socket_path"`
	StartedAt       string `json:"started_at"`
}
