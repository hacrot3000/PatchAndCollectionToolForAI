package session

import "encoding/json"

const (
	patchProtocolName     = "taskdeck.patch"
	patchProtocolVersion  = 1
	maxProtocolEventBytes = 1 << 20
)

type protocolEnvelope struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
	Type     string `json:"type"`
	Seq      int64  `json:"seq"`
}

type ProtocolState struct {
	Available     bool            `json:"available"`
	Enabled       bool            `json:"enabled"`
	EventCount    int             `json:"event_count"`
	LastSeq       int64           `json:"last_seq,omitempty"`
	LastEvent     json.RawMessage `json:"last_event,omitempty"`
	QueueSnapshot json.RawMessage `json:"queue_snapshot,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type ProtocolStateProvider interface {
	ProtocolState(string) (ProtocolState, error)
}

func cloneProtocolState(in ProtocolState) ProtocolState {
	out := in
	out.LastEvent = append(json.RawMessage(nil), in.LastEvent...)
	out.QueueSnapshot = append(json.RawMessage(nil), in.QueueSnapshot...)
	return out
}
