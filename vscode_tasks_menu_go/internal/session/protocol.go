package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	patchProtocolName     = "taskdeck.patch"
	patchProtocolVersion  = 1
	maxProtocolEventBytes   = 1 << 20
	maxProtocolCommandBytes = 1 << 20
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

type ProtocolCommandWriter interface {
	ProtocolCommand(string, []byte) error
}

func validateProtocolCommand(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxProtocolCommandBytes {
		return nil, fmt.Errorf("Patch protocol command size is invalid")
	}
	if bytes.IndexByte(data, '\n') >= 0 || bytes.IndexByte(data, '\r') >= 0 {
		return nil, fmt.Errorf("Patch protocol command must be one JSONL record")
	}
	var envelope struct {
		Protocol string `json:"protocol"`
		Version  int    `json:"version"`
		Type     string `json:"type"`
		Seq      int64  `json:"seq"`
		Command  string `json:"command"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if envelope.Protocol != patchProtocolName || envelope.Version != patchProtocolVersion || envelope.Type != "command" {
		return nil, fmt.Errorf("unsupported Patch protocol command envelope")
	}
	if envelope.Seq < 1 || strings.TrimSpace(envelope.Command) == "" {
		return nil, fmt.Errorf("invalid Patch protocol command identity")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("invalid Patch protocol command object: %w", err)
	}
	return json.Marshal(value)
}

func cloneProtocolState(in ProtocolState) ProtocolState {
	out := in
	out.LastEvent = append(json.RawMessage(nil), in.LastEvent...)
	out.QueueSnapshot = append(json.RawMessage(nil), in.QueueSnapshot...)
	return out
}
