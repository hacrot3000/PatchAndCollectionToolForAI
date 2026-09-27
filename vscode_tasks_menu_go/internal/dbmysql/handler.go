package dbmysql

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type Handler struct {
	manifest  dbadapter.Manifest
	client    Client
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
		h.config.Secret = ""
		h.config = Config{}
		h.client = Client{}
		h.connected = false
		return map[string]bool{"disconnected": true}, nil
	case dbadapter.OpPing:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		result, err := h.query(ctx, "SELECT 1 AS taskdeck_ping", 1)
		if err != nil {
			return nil, mysqlProtocolError("PING_FAILED", err)
		}
		if len(result.Rows) != 1 {
			return nil, &dbadapter.ProtocolError{Code: "PING_FAILED", Message: "MySQL ping returned no row"}
		}
		return map[string]bool{"ok": true}, nil
	case dbadapter.OpListCatalogs:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		return h.listCatalogs(ctx)
	case dbadapter.OpListObjects:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ListObjectsPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		return h.listObjects(ctx, payload)
	case dbadapter.OpDescribeObject:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.DescribeObjectPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		return h.describeObject(ctx, payload)
	case dbadapter.OpExecute:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ExecutePayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeExecutePayload(payload)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_QUERY", err)
		}
		statement, err := normalizeSingleStatement(normalized.Statement)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_QUERY", err)
		}
		if h.config.ReadOnly {
			if err := readOnlyStatement(statement); err != nil {
				return nil, mysqlProtocolError("READ_ONLY", err)
			}
		}
		result, err := h.query(ctx, statement, normalized.MaxRows)
		if err != nil {
			return nil, mysqlProtocolError("QUERY_FAILED", err)
		}
		return result, nil
	default:
		return nil, &dbadapter.ProtocolError{
			Code:    "UNSUPPORTED_OPERATION",
			Message: fmt.Sprintf("MySQL adapter does not support operation %q", request.Operation),
		}
	}
}

func (h *Handler) connect(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	var payload dbadapter.ConnectPayload
	if err := decodePayload(request.Payload, &payload); err != nil {
		return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
	}
	config, err := ConfigFromConnectPayload(payload)
	if err != nil {
		return nil, mysqlProtocolError("INVALID_CONFIG", err)
	}
	client, err := FindClient()
	if err != nil {
		return nil, mysqlProtocolError("CLIENT_UNAVAILABLE", err)
	}

	output, err := runClient(ctx, client, config, "SELECT 1 AS taskdeck_connect")
	if err != nil {
		config.Secret = ""
		return nil, mysqlProtocolError("CONNECT_FAILED", err)
	}
	result, err := parseXMLResult(output.stdout, 1)
	if err != nil || len(result.Rows) != 1 {
		config.Secret = ""
		if err == nil {
			err = errors.New("MySQL connection probe returned no row")
		}
		return nil, mysqlProtocolError("CONNECT_FAILED", err)
	}

	h.config.Secret = ""
	h.config = config
	h.client = client
	h.connected = true
	return map[string]interface{}{
		"connected": true,
		"client": map[string]string{
			"flavor":  client.Flavor,
			"version": client.Version,
		},
	}, nil
}

func (h *Handler) requireConnected() *dbadapter.ProtocolError {
	if h == nil || !h.connected {
		return &dbadapter.ProtocolError{Code: "NOT_CONNECTED", Message: "MySQL adapter is not connected"}
	}
	return nil
}

func (h *Handler) query(ctx context.Context, statement string, maxRows int) (dbadapter.ExecuteResult, error) {
	output, err := runClient(ctx, h.client, h.config, statement)
	if err != nil {
		return dbadapter.ExecuteResult{}, err
	}
	return parseXMLResult(output.stdout, maxRows)
}

func (h *Handler) listCatalogs(ctx context.Context) (interface{}, *dbadapter.ProtocolError) {
	result, err := h.query(
		ctx,
		"SELECT SCHEMA_NAME AS name FROM information_schema.SCHEMATA ORDER BY SCHEMA_NAME",
		dbadapter.MaxRows,
	)
	if err != nil {
		return nil, mysqlProtocolError("LIST_CATALOGS_FAILED", err)
	}
	if len(result.Rows) == 0 {
		return []dbadapter.Object{}, nil
	}
	index, err := resultColumnIndex(result, "name")
	if err != nil {
		return nil, mysqlProtocolError("LIST_CATALOGS_FAILED", err)
	}
	objects := make([]dbadapter.Object, 0, len(result.Rows))
	for _, row := range result.Rows {
		name := resultCellString(row[index])
		if name == "" {
			continue
		}
		objects = append(objects, dbadapter.Object{Kind: "database", Name: name, Catalog: name})
	}
	return objects, nil
}

func (h *Handler) listObjects(ctx context.Context, payload dbadapter.ListObjectsPayload) (interface{}, *dbadapter.ProtocolError) {
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{
			Code:    "CATALOG_REQUIRED",
			Message: "MySQL catalog/database is required",
		}
	}
	query := "SELECT TABLE_SCHEMA AS catalog, TABLE_NAME AS name, " +
		"CASE WHEN TABLE_TYPE = 'VIEW' THEN 'view' ELSE 'table' END AS kind " +
		"FROM information_schema.TABLES WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" ORDER BY TABLE_NAME"
	result, err := h.query(ctx, query, dbadapter.MaxRows)
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	if len(result.Rows) == 0 {
		return map[string]interface{}{
			"objects":   []dbadapter.Object{},
			"truncated": result.Truncated,
		}, nil
	}
	catalogIndex, err := resultColumnIndex(result, "catalog")
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	nameIndex, err := resultColumnIndex(result, "name")
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	kindIndex, err := resultColumnIndex(result, "kind")
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	objects := make([]dbadapter.Object, 0, len(result.Rows))
	for _, row := range result.Rows {
		kind := resultCellString(row[kindIndex])
		if payload.Kind != "" && !strings.EqualFold(strings.TrimSpace(payload.Kind), kind) {
			continue
		}
		objects = append(objects, dbadapter.Object{
			Kind:    kind,
			Name:    resultCellString(row[nameIndex]),
			Catalog: resultCellString(row[catalogIndex]),
		})
	}
	return map[string]interface{}{
		"objects":   objects,
		"truncated": result.Truncated,
	}, nil
}

func (h *Handler) describeObject(ctx context.Context, payload dbadapter.DescribeObjectPayload) (interface{}, *dbadapter.ProtocolError) {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "MySQL object name is required"}
	}
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_REQUIRED", Message: "MySQL catalog/database is required"}
	}
	query := "SELECT COLUMN_NAME AS name, COLUMN_TYPE AS type, IS_NULLABLE AS nullable, " +
		"COLUMN_DEFAULT AS default_value, EXTRA AS extra " +
		"FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" AND TABLE_NAME = " + mysqlTextExpression(name) +
		" ORDER BY ORDINAL_POSITION"
	result, err := h.query(ctx, query, dbadapter.MaxRows)
	if err != nil {
		return nil, mysqlProtocolError("DESCRIBE_FAILED", err)
	}
	if len(result.Rows) == 0 {
		return map[string]interface{}{
			"kind":      firstNonEmpty(strings.TrimSpace(payload.Kind), "table"),
			"name":      name,
			"catalog":   catalog,
			"columns":   []map[string]interface{}{},
			"truncated": result.Truncated,
		}, nil
	}

	indexes := make(map[string]int)
	for _, column := range []string{"name", "type", "nullable", "default_value", "extra"} {
		index, err := resultColumnIndex(result, column)
		if err != nil {
			return nil, mysqlProtocolError("DESCRIBE_FAILED", err)
		}
		indexes[column] = index
	}
	columns := make([]map[string]interface{}, 0, len(result.Rows))
	for _, row := range result.Rows {
		column := map[string]interface{}{
			"name":     resultCellString(row[indexes["name"]]),
			"type":     resultCellString(row[indexes["type"]]),
			"nullable": strings.EqualFold(resultCellString(row[indexes["nullable"]]), "YES"),
			"extra":    resultCellString(row[indexes["extra"]]),
		}
		if row[indexes["default_value"]] != nil {
			column["default"] = resultCellString(row[indexes["default_value"]])
		} else {
			column["default"] = nil
		}
		columns = append(columns, column)
	}
	return map[string]interface{}{
		"kind":      firstNonEmpty(strings.TrimSpace(payload.Kind), "table"),
		"name":      name,
		"catalog":   catalog,
		"columns":   columns,
		"truncated": result.Truncated,
	}, nil
}

func decodePayload(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid MySQL adapter payload: %w", err)
	}
	return nil
}

func mysqlProtocolError(code string, err error) *dbadapter.ProtocolError {
	message := strings.TrimSpace(err.Error())
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &dbadapter.ProtocolError{Code: code, Message: message}
}

func mysqlTextExpression(value string) string {
	return "CONVERT(0x" + hex.EncodeToString([]byte(value)) + " USING utf8mb4)"
}

func resultColumnIndex(result dbadapter.ExecuteResult, name string) (int, error) {
	for index, column := range result.Columns {
		if strings.EqualFold(column.Name, name) {
			return index, nil
		}
	}
	names := make([]string, 0, len(result.Columns))
	for _, column := range result.Columns {
		names = append(names, column.Name)
	}
	sort.Strings(names)
	return -1, fmt.Errorf("MySQL result missing column %q; got %v", name, names)
}

func resultCellString(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func RunAdapter(ctx context.Context, taskdeckExecutable string) error {
	handler, err := NewHandler(taskdeckExecutable)
	if err != nil {
		return err
	}
	return dbadapter.Serve(ctx, os.Stdin, os.Stdout, handler)
}
