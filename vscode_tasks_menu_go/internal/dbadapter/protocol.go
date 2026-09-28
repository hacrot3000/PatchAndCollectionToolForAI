package dbadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ProtocolName    = "taskdeck.db"
	ProtocolVersion = 1

	MaxMessageBytes = 1 << 20
	MaxRows         = 1000
	MaxColumns      = 256
	MaxObjects      = 5000
	MaxCellBytes    = 256 << 10
)

type Operation string

const (
	OpHello          Operation = "hello"
	OpCapabilities   Operation = "capabilities"
	OpConnect        Operation = "connect"
	OpDisconnect     Operation = "disconnect"
	OpPing           Operation = "ping"
	OpListCatalogs   Operation = "list_catalogs"
	OpListObjects    Operation = "list_objects"
	OpDescribeObject Operation = "describe_object"
	OpBrowseRows     Operation = "browse_rows"
	OpMutateRows     Operation = "mutate_rows"
	OpObjectAction   Operation = "object_action"
	OpExecute        Operation = "execute"
	OpCancel         Operation = "cancel"
	OpBegin          Operation = "begin"
	OpCommit         Operation = "commit"
	OpRollback       Operation = "rollback"
)

type Envelope struct {
	Protocol  string          `json:"protocol"`
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Operation Operation       `json:"operation,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *ProtocolError  `json:"error,omitempty"`
}

type ProtocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type ExecuteEditInfo struct {
	Catalog           string                   `json:"catalog,omitempty"`
	Schema            string                   `json:"schema,omitempty"`
	Kind              string                   `json:"kind,omitempty"`
	Name              string                   `json:"name,omitempty"`
	Editable          bool                     `json:"editable"`
	EditabilityReason string                   `json:"editability_reason,omitempty"`
	Columns           []BrowseColumn           `json:"columns,omitempty"`
	RowIdentities     []map[string]interface{} `json:"row_identities,omitempty"`
}

type ExecuteResult struct {
	Columns      []Column         `json:"columns,omitempty"`
	Rows         [][]interface{}  `json:"rows,omitempty"`
	AffectedRows int64            `json:"affected_rows,omitempty"`
	Truncated    bool             `json:"truncated,omitempty"`
	Edit         *ExecuteEditInfo `json:"edit,omitempty"`
}

type Object struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Catalog  string `json:"catalog,omitempty"`
	Schema   string `json:"schema,omitempty"`
	Parent   string `json:"parent,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func NewRequest(requestID string, operation Operation, payload interface{}) (Envelope, error) {
	requestID = strings.TrimSpace(requestID)
	if err := validateToken("request id", requestID, 128, true); err != nil {
		return Envelope{}, err
	}
	if !KnownOperation(operation) {
		return Envelope{}, fmt.Errorf("unsupported database adapter operation %q", operation)
	}
	raw, err := marshalPayload(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Protocol:  ProtocolName,
		Version:   ProtocolVersion,
		Type:      "request",
		RequestID: requestID,
		Operation: operation,
		Payload:   raw,
	}, nil
}

func NewResponse(request Envelope, payload interface{}) (Envelope, error) {
	if err := ValidateEnvelope(request); err != nil {
		return Envelope{}, err
	}
	if request.Type != "request" {
		return Envelope{}, errors.New("database adapter response requires a request envelope")
	}
	raw, err := marshalPayload(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Protocol:  ProtocolName,
		Version:   ProtocolVersion,
		Type:      "response",
		RequestID: request.RequestID,
		SessionID: request.SessionID,
		Operation: request.Operation,
		Payload:   raw,
	}, nil
}

func NewErrorResponse(request Envelope, code, message string) (Envelope, error) {
	if err := ValidateEnvelope(request); err != nil {
		return Envelope{}, err
	}
	if request.Type != "request" {
		return Envelope{}, errors.New("database adapter error response requires a request envelope")
	}
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if err := validateToken("error code", code, 64, true); err != nil {
		return Envelope{}, err
	}
	if err := validateText("error message", message, 4096, true); err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Protocol:  ProtocolName,
		Version:   ProtocolVersion,
		Type:      "response",
		RequestID: request.RequestID,
		SessionID: request.SessionID,
		Operation: request.Operation,
		Error:     &ProtocolError{Code: code, Message: message},
	}, nil
}

func ValidateEnvelope(env Envelope) error {
	if env.Protocol != ProtocolName {
		return fmt.Errorf("unsupported database adapter protocol %q", env.Protocol)
	}
	if env.Version != ProtocolVersion {
		return fmt.Errorf("unsupported database adapter protocol version %d", env.Version)
	}
	switch env.Type {
	case "request", "response", "event":
	default:
		return fmt.Errorf("unsupported database adapter message type %q", env.Type)
	}
	if env.Type == "request" || env.Type == "response" {
		if err := validateToken("request id", env.RequestID, 128, true); err != nil {
			return err
		}
	}
	if env.SessionID != "" {
		if err := validateToken("session id", env.SessionID, 128, false); err != nil {
			return err
		}
	}
	if env.Operation != "" && !KnownOperation(env.Operation) {
		return fmt.Errorf("unsupported database adapter operation %q", env.Operation)
	}
	if env.Type == "request" && env.Operation == "" {
		return errors.New("database adapter request operation is required")
	}
	if env.Error != nil {
		if env.Type != "response" {
			return errors.New("database adapter errors are valid only on responses")
		}
		if err := validateToken("error code", strings.TrimSpace(env.Error.Code), 64, true); err != nil {
			return err
		}
		if err := validateText("error message", strings.TrimSpace(env.Error.Message), 4096, true); err != nil {
			return err
		}
		if len(env.Payload) != 0 {
			return errors.New("database adapter response must not contain payload and error together")
		}
	}
	if len(env.Payload) > MaxMessageBytes {
		return fmt.Errorf("database adapter payload exceeds %d bytes", MaxMessageBytes)
	}
	return nil
}

func ValidateResponsePayload(env Envelope) error {
	if err := ValidateEnvelope(env); err != nil {
		return err
	}
	if env.Type != "response" {
		return fmt.Errorf("database adapter payload validation requires a response envelope")
	}
	if env.Error != nil {
		return nil
	}
	switch env.Operation {
	case OpExecute:
		var result ExecuteResult
		if err := json.Unmarshal(env.Payload, &result); err != nil {
			return fmt.Errorf("decode database execute result: %w", err)
		}
		return ValidateExecuteResult(result)
	case OpBrowseRows:
		var result BrowseRowsResult
		if err := json.Unmarshal(env.Payload, &result); err != nil {
			return fmt.Errorf("decode database browse rows result: %w", err)
		}
		return ValidateBrowseRowsResult(result)
	case OpMutateRows:
		var result MutateRowsResult
		if err := json.Unmarshal(env.Payload, &result); err != nil {
			return fmt.Errorf("decode database mutate rows result: %w", err)
		}
		return ValidateMutateRowsResult(result)
	case OpObjectAction:
		var result ObjectActionResult
		if err := json.Unmarshal(env.Payload, &result); err != nil {
			return fmt.Errorf("decode database object action result: %w", err)
		}
		return ValidateObjectActionResult(result)
	default:
		return nil
	}
}

func ValidateExecuteResult(result ExecuteResult) error {
	if len(result.Columns) > MaxColumns {
		return fmt.Errorf("database result exceeds %d columns", MaxColumns)
	}
	if len(result.Rows) > MaxRows {
		return fmt.Errorf("database result exceeds %d rows", MaxRows)
	}
	for i, column := range result.Columns {
		if err := validateText("column name", strings.TrimSpace(column.Name), 512, true); err != nil {
			return fmt.Errorf("column %d: %w", i+1, err)
		}
		if column.Type != "" {
			if err := validateText("column type", strings.TrimSpace(column.Type), 256, false); err != nil {
				return fmt.Errorf("column %d: %w", i+1, err)
			}
		}
	}
	for i, row := range result.Rows {
		if len(result.Columns) != 0 && len(row) != len(result.Columns) {
			return fmt.Errorf("row %d has %d cells for %d columns", i+1, len(row), len(result.Columns))
		}
		for j, cell := range row {
			raw, err := json.Marshal(cell)
			if err != nil {
				return fmt.Errorf("row %d cell %d cannot be encoded: %w", i+1, j+1, err)
			}
			if len(raw) > MaxCellBytes {
				return fmt.Errorf("row %d cell %d exceeds %d bytes", i+1, j+1, MaxCellBytes)
			}
		}
	}
	if result.Edit != nil {
		edit := result.Edit
		for label, value := range map[string]string{
			"database execute edit catalog": strings.TrimSpace(edit.Catalog),
			"database execute edit schema": strings.TrimSpace(edit.Schema),
			"database execute edit kind": strings.TrimSpace(edit.Kind),
			"database execute edit name": strings.TrimSpace(edit.Name),
			"database execute edit reason": strings.TrimSpace(edit.EditabilityReason),
		} {
			max := 512
			if strings.Contains(label, "reason") {
				max = 1024
			}
			if err := validateText(label, value, max, false); err != nil {
				return err
			}
		}
		if len(edit.Columns) != 0 && len(edit.Columns) != len(result.Columns) {
			return fmt.Errorf("database execute edit metadata has %d columns for %d result columns", len(edit.Columns), len(result.Columns))
		}
		if len(edit.RowIdentities) != 0 && len(edit.RowIdentities) != len(result.Rows) {
			return fmt.Errorf("database execute edit metadata has %d identities for %d rows", len(edit.RowIdentities), len(result.Rows))
		}
		for i, column := range edit.Columns {
			if err := validateText("database execute edit column name", strings.TrimSpace(column.Name), 512, true); err != nil {
				return fmt.Errorf("edit column %d: %w", i+1, err)
			}
			if err := validateText("database execute edit column type", strings.TrimSpace(column.Type), 256, false); err != nil {
				return fmt.Errorf("edit column %d: %w", i+1, err)
			}
		}
		for i, identity := range edit.RowIdentities {
			if len(identity) > MaxIdentityColumns {
				return fmt.Errorf("execute row %d identity exceeds %d columns", i+1, MaxIdentityColumns)
			}
			for key, value := range identity {
				if err := validateText("database execute row identity column", strings.TrimSpace(key), 512, true); err != nil {
					return fmt.Errorf("execute row %d identity: %w", i+1, err)
				}
				raw, err := json.Marshal(value)
				if err != nil {
					return fmt.Errorf("execute row %d identity %q cannot be encoded: %w", i+1, key, err)
				}
				if len(raw) > MaxCellBytes {
					return fmt.Errorf("execute row %d identity %q exceeds %d bytes", i+1, key, MaxCellBytes)
				}
			}
		}
	}
	return nil
}

func KnownOperation(operation Operation) bool {
	switch operation {
	case OpHello, OpCapabilities, OpConnect, OpDisconnect, OpPing,
		OpListCatalogs, OpListObjects, OpDescribeObject, OpBrowseRows, OpMutateRows,
		OpObjectAction, OpExecute, OpCancel,
		OpBegin, OpCommit, OpRollback:
		return true
	default:
		return false
	}
}

func marshalPayload(payload interface{}) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode database adapter payload: %w", err)
	}
	if len(raw) > MaxMessageBytes {
		return nil, fmt.Errorf("database adapter payload exceeds %d bytes", MaxMessageBytes)
	}
	return raw, nil
}

func validateToken(label, value string, max int, required bool) error {
	if err := validateText(label, value, max, required); err != nil {
		return err
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return fmt.Errorf("%s contains unsupported character %q", label, r)
	}
	return nil
}

func validateText(label, value string, max int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d bytes", label, max)
	}
	for _, r := range value {
		if r == 0 || r == '\r' || r == '\n' {
			return fmt.Errorf("%s contains unsupported control characters", label)
		}
	}
	return nil
}
