package dbadapter

import (
	"fmt"
	"strings"
)

type ConnectPayload struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Database string `json:"database,omitempty"`
	File     string `json:"file,omitempty"`
	Secret   string `json:"secret,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
	Options  map[string]string `json:"options,omitempty"`
}

type ListObjectsPayload struct {
	Catalog string `json:"catalog,omitempty"`
	Schema  string `json:"schema,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type DescribeObjectPayload struct {
	Catalog string `json:"catalog,omitempty"`
	Schema  string `json:"schema,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name"`
}

type ExecutePayload struct {
	Statement string `json:"statement"`
	MaxRows   int    `json:"max_rows"`
}

func NormalizeExecutePayload(payload ExecutePayload) (ExecutePayload, error) {
	payload.Statement = strings.TrimSpace(payload.Statement)
	if payload.Statement == "" {
		return ExecutePayload{}, fmt.Errorf("database statement is required")
	}
	if len(payload.Statement) > MaxMessageBytes/2 {
		return ExecutePayload{}, fmt.Errorf("database statement exceeds %d bytes", MaxMessageBytes/2)
	}
	if payload.MaxRows == 0 {
		payload.MaxRows = MaxRows
	}
	if payload.MaxRows < 1 || payload.MaxRows > MaxRows {
		return ExecutePayload{}, fmt.Errorf("database max_rows must be between 1 and %d", MaxRows)
	}
	return payload, nil
}
