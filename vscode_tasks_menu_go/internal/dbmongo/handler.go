package dbmongo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		h.disconnect()
		return map[string]bool{"disconnected": true}, nil
	case dbadapter.OpPing:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var result map[string]interface{}
		if err := h.run(ctx, pingOperationBody(), &result); err != nil {
			return nil, mongoProtocolError("PING_FAILED", err)
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
		if err := decodeMongoPayload(request.Payload, &payload); err != nil {
			return nil, mongoProtocolError("INVALID_PAYLOAD", err)
		}
		return h.listObjects(ctx, payload)
	case dbadapter.OpDescribeObject:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.DescribeObjectPayload
		if err := decodeMongoPayload(request.Payload, &payload); err != nil {
			return nil, mongoProtocolError("INVALID_PAYLOAD", err)
		}
		return h.describeObject(ctx, payload)
	case dbadapter.OpExecute:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ExecutePayload
		if err := decodeMongoPayload(request.Payload, &payload); err != nil {
			return nil, mongoProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeExecutePayload(payload)
		if err != nil {
			return nil, mongoProtocolError("INVALID_QUERY", err)
		}
		query, err := ParseFindQuery(normalized.Statement, normalized.MaxRows)
		if err != nil {
			return nil, mongoProtocolError("INVALID_QUERY", err)
		}
		database := strings.TrimSpace(query.Database)
		if database == "" {
			database = strings.TrimSpace(h.config.Database)
		}
		if database == "" {
			return nil, &dbadapter.ProtocolError{Code: "DATABASE_REQUIRED", Message: "MongoDB find query requires a database"}
		}
		body, err := findOperationBody(database, query)
		if err != nil {
			return nil, mongoProtocolError("INVALID_QUERY", err)
		}
		var result struct {
			Documents []interface{} `json:"documents"`
			Truncated bool          `json:"truncated"`
		}
		if err := h.run(ctx, body, &result); err != nil {
			return nil, mongoProtocolError("QUERY_FAILED", err)
		}
		normalizedResult, err := documentsToExecuteResult(result.Documents, result.Truncated)
		if err != nil {
			return nil, mongoProtocolError("RESULT_FAILED", err)
		}
		return normalizedResult, nil
	default:
		return nil, &dbadapter.ProtocolError{
			Code:    "UNSUPPORTED_OPERATION",
			Message: fmt.Sprintf("MongoDB adapter does not support operation %q", request.Operation),
		}
	}
}

func (h *Handler) connect(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	var payload dbadapter.ConnectPayload
	if err := decodeMongoPayload(request.Payload, &payload); err != nil {
		return nil, mongoProtocolError("INVALID_PAYLOAD", err)
	}
	config, err := ConfigFromConnectPayload(payload)
	if err != nil {
		return nil, mongoProtocolError("INVALID_CONFIG", err)
	}
	client, err := FindClient()
	if err != nil {
		config.Password = ""
		return nil, mongoProtocolError("CLIENT_UNAVAILABLE", err)
	}
	script, err := buildScript(config, pingOperationBody())
	if err != nil {
		config.Password = ""
		return nil, mongoProtocolError("CONNECT_FAILED", err)
	}
	output, err := runScript(ctx, client, script, config.Password)
	if err != nil {
		config.Password = ""
		return nil, mongoProtocolError("CONNECT_FAILED", err)
	}
	var ping map[string]interface{}
	if err := parseScriptResult(output.stdout, config.Password, &ping); err != nil {
		config.Password = ""
		return nil, mongoProtocolError("CONNECT_FAILED", err)
	}

	h.disconnect()
	h.client = client
	h.config = config
	h.connected = true
	return map[string]interface{}{
		"connected": true,
		"client": map[string]string{
			"version": client.Version,
		},
	}, nil
}

func (h *Handler) disconnect() {
	if h == nil {
		return
	}
	h.config.Password = ""
	h.config = Config{}
	h.client = Client{}
	h.connected = false
}

func (h *Handler) requireConnected() *dbadapter.ProtocolError {
	if h == nil || !h.connected || h.client.Path == "" {
		return &dbadapter.ProtocolError{Code: "NOT_CONNECTED", Message: "MongoDB adapter is not connected"}
	}
	return nil
}

func (h *Handler) run(ctx context.Context, operationBody string, target interface{}) error {
	script, err := buildScript(h.config, operationBody)
	if err != nil {
		return err
	}
	output, err := runScript(ctx, h.client, script, h.config.Password)
	if err != nil {
		return err
	}
	return parseScriptResult(output.stdout, h.config.Password, target)
}

func (h *Handler) listCatalogs(ctx context.Context) (interface{}, *dbadapter.ProtocolError) {
	var result struct {
		Databases []struct {
			Name string `json:"name"`
		} `json:"databases"`
	}
	if err := h.run(ctx, listCatalogsOperationBody(), &result); err != nil {
		return nil, mongoProtocolError("LIST_CATALOGS_FAILED", err)
	}
	truncated := false
	if len(result.Databases) > dbadapter.MaxObjects {
		result.Databases = result.Databases[:dbadapter.MaxObjects]
		truncated = true
	}
	objects := make([]dbadapter.Object, 0, len(result.Databases))
	for _, database := range result.Databases {
		name := strings.TrimSpace(database.Name)
		if name == "" {
			continue
		}
		objects = append(objects, dbadapter.Object{Kind: "database", Name: name, Catalog: name})
	}
	return map[string]interface{}{"catalogs": objects, "truncated": truncated}, nil
}

func (h *Handler) listObjects(ctx context.Context, payload dbadapter.ListObjectsPayload) (interface{}, *dbadapter.ProtocolError) {
	database := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if database == "" {
		return nil, &dbadapter.ProtocolError{Code: "DATABASE_REQUIRED", Message: "MongoDB database is required"}
	}
	if err := validateDatabaseName(database, true); err != nil {
		return nil, mongoProtocolError("INVALID_DATABASE", err)
	}
	body, err := listObjectsOperationBody(database)
	if err != nil {
		return nil, mongoProtocolError("INVALID_DATABASE", err)
	}
	var infos []map[string]interface{}
	if err := h.run(ctx, body, &infos); err != nil {
		return nil, mongoProtocolError("LIST_OBJECTS_FAILED", err)
	}
	truncated := false
	if len(infos) > dbadapter.MaxObjects {
		infos = infos[:dbadapter.MaxObjects]
		truncated = true
	}
	objects := make([]dbadapter.Object, 0, len(infos))
	for _, info := range infos {
		name, _ := info["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		kind, _ := info["type"].(string)
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			kind = "collection"
		}
		if requested := strings.TrimSpace(payload.Kind); requested != "" && !strings.EqualFold(requested, kind) {
			continue
		}
		objects = append(objects, dbadapter.Object{
			Kind:    kind,
			Name:    name,
			Catalog: database,
		})
	}
	return map[string]interface{}{"objects": objects, "truncated": truncated}, nil
}

func (h *Handler) describeObject(ctx context.Context, payload dbadapter.DescribeObjectPayload) (interface{}, *dbadapter.ProtocolError) {
	database := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if database == "" {
		return nil, &dbadapter.ProtocolError{Code: "DATABASE_REQUIRED", Message: "MongoDB database is required"}
	}
	if err := validateDatabaseName(database, true); err != nil {
		return nil, mongoProtocolError("INVALID_DATABASE", err)
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" || len(name) > 512 || strings.ContainsAny(name, "\x00\r\n") {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "MongoDB collection name is invalid"}
	}
	body, err := describeObjectOperationBody(database, name)
	if err != nil {
		return nil, mongoProtocolError("INVALID_OBJECT", err)
	}
	var detail map[string]interface{}
	if err := h.run(ctx, body, &detail); err != nil {
		return nil, mongoProtocolError("DESCRIBE_FAILED", err)
	}
	return map[string]interface{}{
		"kind":    firstNonEmpty(strings.TrimSpace(payload.Kind), "collection"),
		"name":    name,
		"catalog": database,
		"detail":  detail,
	}, nil
}

func pingOperationBody() string {
	return "    const __r = __taskdeckDb.runCommand({ping:1});\n" +
		"    if (!__r || __r.ok !== 1) throw new Error('MongoDB ping failed');\n" +
		"    return {ok:true};"
}

func listCatalogsOperationBody() string {
	return "    const __r = __taskdeckDb.getSiblingDB('admin').runCommand({listDatabases:1,nameOnly:true});\n" +
		"    if (!__r || __r.ok !== 1) throw new Error('MongoDB listDatabases failed');\n" +
		"    return {databases:__r.databases || []};"
}

func listObjectsOperationBody(database string) (string, error) {
	databaseLiteral, err := javascriptString(database)
	if err != nil {
		return "", err
	}
	return "    const __db = __taskdeckDb.getSiblingDB(" + databaseLiteral + ");\n" +
		"    return __db.getCollectionInfos();", nil
}

func describeObjectOperationBody(database, name string) (string, error) {
	databaseLiteral, err := javascriptString(database)
	if err != nil {
		return "", err
	}
	nameLiteral, err := javascriptString(name)
	if err != nil {
		return "", err
	}
	return "    const __db = __taskdeckDb.getSiblingDB(" + databaseLiteral + ");\n" +
		"    const __name = " + nameLiteral + ";\n" +
		"    return {info:__db.getCollectionInfos({name:__name}),indexes:__db.getCollection(__name).getIndexes()};", nil
}

func decodeMongoPayload(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid MongoDB adapter payload: %w", err)
	}
	return nil
}

func mongoProtocolError(code string, err error) *dbadapter.ProtocolError {
	message := strings.TrimSpace(err.Error())
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &dbadapter.ProtocolError{Code: code, Message: message}
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
	defer handler.disconnect()
	return dbadapter.Serve(ctx, os.Stdin, os.Stdout, handler)
}

var _ = errors.New
