package dbredis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	maxBrowseKeys    = 1000
	scanCountHint    = 200
	maxPreviewItems  = 100
)

type Handler struct {
	manifest  dbadapter.Manifest
	client    *Client
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
		if _, err := h.client.Do(ctx, "PING"); err != nil {
			return nil, redisProtocolError("PING_FAILED", err)
		}
		return map[string]bool{"ok": true}, nil
	case dbadapter.OpListCatalogs:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		name := redisCatalogName(h.config.Database)
		return []dbadapter.Object{{
			Kind:    "database",
			Name:    name,
			Catalog: name,
			Metadata: map[string]interface{}{"index": h.config.Database},
		}}, nil
	case dbadapter.OpListObjects:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ListObjectsPayload
		if err := decodeRedisPayload(request.Payload, &payload); err != nil {
			return nil, redisProtocolError("INVALID_PAYLOAD", err)
		}
		return h.listObjects(ctx, payload)
	case dbadapter.OpDescribeObject:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.DescribeObjectPayload
		if err := decodeRedisPayload(request.Payload, &payload); err != nil {
			return nil, redisProtocolError("INVALID_PAYLOAD", err)
		}
		return h.describeObject(ctx, payload)
	case dbadapter.OpExecute:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ExecutePayload
		if err := decodeRedisPayload(request.Payload, &payload); err != nil {
			return nil, redisProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeExecutePayload(payload)
		if err != nil {
			return nil, redisProtocolError("INVALID_COMMAND", err)
		}
		args, err := parseSafeCommand(normalized.Statement)
		if err != nil {
			return nil, redisProtocolError("COMMAND_NOT_ALLOWED", err)
		}
		value, err := h.client.Do(ctx, args...)
		if err != nil {
			return nil, redisProtocolError("COMMAND_FAILED", err)
		}
		result, err := executeResultFromValue(value, normalized.MaxRows)
		if err != nil {
			return nil, redisProtocolError("RESULT_FAILED", err)
		}
		return result, nil
	default:
		return nil, &dbadapter.ProtocolError{
			Code:    "UNSUPPORTED_OPERATION",
			Message: fmt.Sprintf("Redis adapter does not support operation %q", request.Operation),
		}
	}
}

func (h *Handler) connect(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	var payload dbadapter.ConnectPayload
	if err := decodeRedisPayload(request.Payload, &payload); err != nil {
		return nil, redisProtocolError("INVALID_PAYLOAD", err)
	}
	config, err := ConfigFromConnectPayload(payload)
	if err != nil {
		return nil, redisProtocolError("INVALID_CONFIG", err)
	}
	client, err := Dial(ctx, config)
	if err != nil {
		config.Password = ""
		return nil, redisProtocolError("CONNECT_FAILED", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = client.Close()
		}
	}()

	if config.Password != "" {
		var authArgs []string
		if config.Username != "" {
			authArgs = []string{"AUTH", config.Username, config.Password}
		} else {
			authArgs = []string{"AUTH", config.Password}
		}
		if _, err := client.Do(ctx, authArgs...); err != nil {
			config.Password = ""
			return nil, redisProtocolError("AUTH_FAILED", err)
		}
	}
	config.Password = ""
	if config.Database != 0 {
		if _, err := client.Do(ctx, "SELECT", strconv.Itoa(config.Database)); err != nil {
			return nil, redisProtocolError("SELECT_FAILED", err)
		}
	}
	if _, err := client.Do(ctx, "PING"); err != nil {
		return nil, redisProtocolError("CONNECT_FAILED", err)
	}

	h.disconnect()
	h.client = client
	h.config = config
	h.connected = true
	closeOnError = false
	return map[string]interface{}{
		"connected": true,
		"database":  redisCatalogName(config.Database),
	}, nil
}

func (h *Handler) disconnect() {
	if h == nil {
		return
	}
	if h.client != nil {
		_ = h.client.Close()
	}
	h.client = nil
	h.config.Password = ""
	h.config = Config{}
	h.connected = false
}

func (h *Handler) requireConnected() *dbadapter.ProtocolError {
	if h == nil || !h.connected || h.client == nil {
		return &dbadapter.ProtocolError{Code: "NOT_CONNECTED", Message: "Redis adapter is not connected"}
	}
	return nil
}

func (h *Handler) listObjects(ctx context.Context, payload dbadapter.ListObjectsPayload) (interface{}, *dbadapter.ProtocolError) {
	catalog := strings.TrimSpace(payload.Catalog)
	current := redisCatalogName(h.config.Database)
	if catalog != "" && catalog != current {
		return nil, &dbadapter.ProtocolError{
			Code:    "CATALOG_UNAVAILABLE",
			Message: fmt.Sprintf("Redis adapter is connected to %s; reconnect with another database to browse %s", current, catalog),
		}
	}
	if kind := strings.TrimSpace(payload.Kind); kind != "" && !strings.EqualFold(kind, "key") {
		return map[string]interface{}{"objects": []dbadapter.Object{}, "truncated": false}, nil
	}
	keys, truncated, err := h.scanKeys(ctx)
	if err != nil {
		return nil, redisProtocolError("LIST_OBJECTS_FAILED", err)
	}
	sort.Strings(keys)
	objects := make([]dbadapter.Object, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, dbadapter.Object{
			Kind:    "key",
			Name:    key,
			Catalog: current,
		})
	}
	return map[string]interface{}{
		"objects":   objects,
		"truncated": truncated,
	}, nil
}

func (h *Handler) scanKeys(ctx context.Context) ([]string, bool, error) {
	cursor := "0"
	keys := make([]string, 0, minInt(scanCountHint, maxBrowseKeys))
	truncated := false
	for {
		value, err := h.client.Do(ctx, "SCAN", cursor, "COUNT", strconv.Itoa(scanCountHint))
		if err != nil {
			return nil, false, err
		}
		if value.Kind != KindArray || len(value.Array) != 2 {
			return nil, false, errors.New("Redis SCAN returned an unexpected response")
		}
		nextCursor, err := redisValueString(value.Array[0])
		if err != nil {
			return nil, false, err
		}
		keyArray := value.Array[1]
		if keyArray.Kind != KindArray {
			return nil, false, errors.New("Redis SCAN key list is not an array")
		}
		for _, item := range keyArray.Array {
			key, err := redisValueString(item)
			if err != nil {
				return nil, false, err
			}
			if len(keys) >= maxBrowseKeys {
				truncated = true
				break
			}
			keys = append(keys, key)
		}
		if truncated || nextCursor == "0" {
			break
		}
		cursor = nextCursor
	}
	return keys, truncated, nil
}

func (h *Handler) describeObject(ctx context.Context, payload dbadapter.DescribeObjectPayload) (interface{}, *dbadapter.ProtocolError) {
	key := strings.TrimSpace(payload.Name)
	if key == "" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "Redis key is required"}
	}
	catalog := strings.TrimSpace(payload.Catalog)
	current := redisCatalogName(h.config.Database)
	if catalog != "" && catalog != current {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "Redis key is outside the connected logical database"}
	}

	typeValue, err := h.client.Do(ctx, "TYPE", key)
	if err != nil {
		return nil, redisProtocolError("DESCRIBE_FAILED", err)
	}
	keyType, err := redisValueString(typeValue)
	if err != nil {
		return nil, redisProtocolError("DESCRIBE_FAILED", err)
	}
	ttlValue, err := h.client.Do(ctx, "TTL", key)
	if err != nil {
		return nil, redisProtocolError("DESCRIBE_FAILED", err)
	}
	if ttlValue.Kind != KindInteger {
		return nil, &dbadapter.ProtocolError{Code: "DESCRIBE_FAILED", Message: "Redis TTL returned an unexpected response"}
	}

	detail := map[string]interface{}{
		"kind":        "key",
		"name":        key,
		"catalog":     current,
		"type":        keyType,
		"ttl_seconds": ttlValue.Integer,
	}
	if preview, previewTruncated, err := h.previewKey(ctx, key, keyType); err == nil {
		detail["preview"] = preview
		detail["preview_truncated"] = previewTruncated
	} else {
		detail["preview_error"] = redisProtocolError("PREVIEW_FAILED", err).Message
	}
	return detail, nil
}

func (h *Handler) previewKey(ctx context.Context, key, keyType string) (interface{}, bool, error) {
	var (
		value Value
		err   error
	)
	switch strings.ToLower(strings.TrimSpace(keyType)) {
	case "none":
		return nil, false, nil
	case "string":
		value, err = h.client.Do(ctx, "GET", key)
	case "hash":
		value, err = h.client.Do(ctx, "HSCAN", key, "0", "COUNT", strconv.Itoa(maxPreviewItems))
	case "list":
		value, err = h.client.Do(ctx, "LRANGE", key, "0", strconv.Itoa(maxPreviewItems-1))
	case "set":
		value, err = h.client.Do(ctx, "SSCAN", key, "0", "COUNT", strconv.Itoa(maxPreviewItems))
	case "zset":
		value, err = h.client.Do(ctx, "ZRANGE", key, "0", strconv.Itoa(maxPreviewItems-1), "WITHSCORES")
	default:
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	preview, truncated := normalizeRedisCell(value, 0)
	return preview, truncated, nil
}

func decodeRedisPayload(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid Redis adapter payload: %w", err)
	}
	return nil
}

func redisProtocolError(code string, err error) *dbadapter.ProtocolError {
	message := strings.TrimSpace(err.Error())
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &dbadapter.ProtocolError{Code: code, Message: message}
}

func redisCatalogName(database int) string {
	return "db" + strconv.Itoa(database)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func RunAdapter(ctx context.Context, taskdeckExecutable string) error {
	handler, err := NewHandler(taskdeckExecutable)
	if err != nil {
		return err
	}
	defer handler.disconnect()
	return dbadapter.Serve(ctx, os.Stdin, os.Stdout, handler)
}
