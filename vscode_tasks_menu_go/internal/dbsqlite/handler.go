package dbsqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type Handler struct {
	manifest  dbadapter.Manifest
	python    Python
	config    Config
	connected bool
}

func NewHandler(taskdeckExecutable string) (*Handler, error) {
	manifest, err := BuiltinManifest(taskdeckExecutable)
	if err != nil {
		return nil, err
	}
	return &Handler{manifest: manifest}, nil
}

func (h *Handler) Manifest() dbadapter.Manifest {
	return h.manifest
}

func (h *Handler) Handle(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	switch request.Operation {
	case dbadapter.OpConnect:
		return h.connect(ctx, request)
	case dbadapter.OpDisconnect:
		h.disconnect()
		return map[string]bool{"disconnected": true}, nil
	case dbadapter.OpPing:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var result map[string]interface{}
		if err := runHelper(ctx, h.python, h.config, "ping", nil, &result); err != nil {
			return nil, sqliteProtocolError("PING_FAILED", err)
		}
		return map[string]bool{"ok": true}, nil
	case dbadapter.OpListCatalogs:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		return []dbadapter.Object{{Kind: "database", Name: "main", Catalog: "main"}}, nil
	case dbadapter.OpListObjects:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ListObjectsPayload
		if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
			return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
		}
		if catalog := strings.TrimSpace(payload.Catalog); catalog != "" && catalog != "main" {
			return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "SQLite adapter exposes only the main database"}
		}
		var result struct {
			Objects   []dbadapter.Object `json:"objects"`
			Truncated bool               `json:"truncated"`
		}
		if err := runHelper(ctx, h.python, h.config, "list_objects", nil, &result); err != nil {
			return nil, sqliteProtocolError("LIST_OBJECTS_FAILED", err)
		}
		if kind := strings.TrimSpace(payload.Kind); kind != "" {
			filtered := make([]dbadapter.Object, 0, len(result.Objects))
			for _, object := range result.Objects {
				if strings.EqualFold(object.Kind, kind) {
					filtered = append(filtered, object)
				}
			}
			result.Objects = filtered
		}
		return map[string]interface{}{"objects": result.Objects, "truncated": result.Truncated}, nil
	case dbadapter.OpDescribeObject:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.DescribeObjectPayload
		if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
			return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
		}
		if catalog := strings.TrimSpace(payload.Catalog); catalog != "" && catalog != "main" {
			return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "SQLite adapter exposes only the main database"}
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "SQLite object name is required"}
		}
		var result map[string]interface{}
		if err := runHelper(ctx, h.python, h.config, "describe_object", map[string]interface{}{"name": name}, &result); err != nil {
			return nil, sqliteProtocolError("DESCRIBE_FAILED", err)
		}
		return result, nil
	case dbadapter.OpBrowseRows:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.BrowseRowsPayload
		if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
			return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeBrowseRowsPayload(payload)
		if err != nil {
			return nil, sqliteProtocolError("INVALID_BROWSE", err)
		}
		if catalog := strings.TrimSpace(normalized.Catalog); catalog != "" && catalog != "main" {
			return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "SQLite adapter exposes only the main database"}
		}
		var result dbadapter.BrowseRowsResult
		if err := runHelper(ctx, h.python, h.config, "browse_rows", normalized, &result); err != nil {
			return nil, sqliteProtocolError("BROWSE_FAILED", err)
		}
		if err := dbadapter.ValidateBrowseRowsResult(result); err != nil {
			return nil, sqliteProtocolError("BROWSE_RESULT_FAILED", err)
		}
		return result, nil
	case dbadapter.OpMutateRows:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		if h.config.ReadOnly {
			return nil, &dbadapter.ProtocolError{Code: "READ_ONLY", Message: "SQLite connection is read-only"}
		}
		var payload dbadapter.MutateRowsPayload
		if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
			return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeMutateRowsPayload(payload)
		if err != nil {
			return nil, sqliteProtocolError("INVALID_MUTATION", err)
		}
		if catalog := strings.TrimSpace(normalized.Catalog); catalog != "" && catalog != "main" {
			return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "SQLite adapter exposes only the main database"}
		}
		var result dbadapter.MutateRowsResult
		if err := runHelper(ctx, h.python, h.config, "mutate_rows", normalized, &result); err != nil {
			return nil, sqliteProtocolError("MUTATION_FAILED", err)
		}
		if err := dbadapter.ValidateMutateRowsResult(result); err != nil {
			return nil, sqliteProtocolError("MUTATION_RESULT_FAILED", err)
		}
		return result, nil
	case dbadapter.OpObjectAction:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ObjectActionPayload
		if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
			return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeObjectActionPayload(payload)
		if err != nil {
			return nil, sqliteProtocolError("INVALID_OBJECT_ACTION", err)
		}
		if catalog := strings.TrimSpace(normalized.Catalog); catalog != "" && catalog != "main" {
			return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "SQLite adapter exposes only the main database"}
		}
		if h.config.ReadOnly && normalized.Action != "count_rows" {
			return nil, &dbadapter.ProtocolError{Code: "READ_ONLY", Message: "SQLite connection is read-only"}
		}
		var result dbadapter.ObjectActionResult
		if err := runHelper(ctx, h.python, h.config, "object_action", normalized, &result); err != nil {
			return nil, sqliteProtocolError("OBJECT_ACTION_FAILED", err)
		}
		if err := dbadapter.ValidateObjectActionResult(result); err != nil {
			return nil, sqliteProtocolError("OBJECT_ACTION_RESULT_FAILED", err)
		}
		return result, nil
	case dbadapter.OpExecute:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ExecutePayload
		if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
			return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeExecutePayload(payload)
		if err != nil {
			return nil, sqliteProtocolError("INVALID_QUERY", err)
		}
		var result dbadapter.ExecuteResult
		if err := runHelper(ctx, h.python, h.config, "execute", normalized, &result); err != nil {
			return nil, sqliteProtocolError("QUERY_FAILED", err)
		}
		if err := dbadapter.ValidateExecuteResult(result); err != nil {
			return nil, sqliteProtocolError("RESULT_FAILED", err)
		}
		return result, nil
	default:
		return nil, &dbadapter.ProtocolError{
			Code:    "UNSUPPORTED_OPERATION",
			Message: fmt.Sprintf("SQLite adapter does not support operation %q", request.Operation),
		}
	}
}

func (h *Handler) connect(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	var payload dbadapter.ConnectPayload
	if err := decodeSQLitePayload(request.Payload, &payload); err != nil {
		return nil, sqliteProtocolError("INVALID_PAYLOAD", err)
	}
	config, err := ConfigFromConnectPayload(payload)
	if err != nil {
		return nil, sqliteProtocolError("INVALID_CONFIG", err)
	}
	python, err := FindPython()
	if err != nil {
		return nil, sqliteProtocolError("PYTHON_UNAVAILABLE", err)
	}
	var result map[string]interface{}
	if err := runHelper(ctx, python, config, "connect", nil, &result); err != nil {
		return nil, sqliteProtocolError("CONNECT_FAILED", err)
	}
	h.disconnect()
	h.python = python
	h.config = config
	h.connected = true
	return map[string]interface{}{
		"connected": true,
		"python":    python.Version,
		"read_only": config.ReadOnly,
	}, nil
}

func (h *Handler) disconnect() {
	if h == nil {
		return
	}
	h.python = Python{}
	h.config = Config{}
	h.connected = false
}

func (h *Handler) requireConnected() *dbadapter.ProtocolError {
	if h == nil || !h.connected || h.python.Path == "" {
		return &dbadapter.ProtocolError{Code: "NOT_CONNECTED", Message: "SQLite adapter is not connected"}
	}
	return nil
}

func decodeSQLitePayload(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid SQLite adapter payload: %w", err)
	}
	return nil
}

func sqliteProtocolError(code string, err error) *dbadapter.ProtocolError {
	message := strings.TrimSpace(err.Error())
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &dbadapter.ProtocolError{Code: code, Message: message}
}

func RunAdapter(ctx context.Context, taskdeckExecutable string) error {
	handler, err := NewHandler(taskdeckExecutable)
	if err != nil {
		return err
	}
	defer handler.disconnect()
	return dbadapter.Serve(ctx, os.Stdin, os.Stdout, handler)
}
