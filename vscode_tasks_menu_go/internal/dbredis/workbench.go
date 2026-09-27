package dbredis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const maxRedisBrowseOffset = 10000

func (h *Handler) browseKey(ctx context.Context, payload dbadapter.BrowseRowsPayload) (interface{}, *dbadapter.ProtocolError) {
	if len(payload.Filters) != 0 || len(payload.Sort) != 0 {
		return nil, &dbadapter.ProtocolError{
			Code:    "BROWSE_UNSUPPORTED",
			Message: "Redis key viewers do not support relational sort/filter descriptors",
		}
	}
	if payload.Offset > maxRedisBrowseOffset {
		return nil, &dbadapter.ProtocolError{
			Code:    "BROWSE_OFFSET_LIMIT",
			Message: fmt.Sprintf("Redis viewer offset must not exceed %d", maxRedisBrowseOffset),
		}
	}
	if kind := strings.TrimSpace(payload.Kind); kind != "" && !strings.EqualFold(kind, "key") {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "Redis row browser supports keys only"}
	}
	current := redisCatalogName(h.config.Database)
	if catalog := strings.TrimSpace(payload.Catalog); catalog != "" && catalog != current {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "Redis key is outside the connected logical database"}
	}
	key := strings.TrimSpace(payload.Name)
	if key == "" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "Redis key is required"}
	}
	typeValue, err := h.client.Do(ctx, "TYPE", key)
	if err != nil {
		return nil, redisProtocolError("BROWSE_FAILED", err)
	}
	keyType, err := redisValueString(typeValue)
	if err != nil {
		return nil, redisProtocolError("BROWSE_FAILED", err)
	}
	var result dbadapter.BrowseRowsResult
	switch strings.ToLower(strings.TrimSpace(keyType)) {
	case "string":
		result, err = h.browseRedisString(ctx, key, payload)
	case "list":
		result, err = h.browseRedisList(ctx, key, payload)
	case "hash":
		result, err = h.browseRedisHash(ctx, key, payload)
	case "set":
		result, err = h.browseRedisSet(ctx, key, payload)
	case "zset":
		result, err = h.browseRedisZSet(ctx, key, payload)
	case "none":
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_NOT_FOUND", Message: "Redis key no longer exists"}
	default:
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: fmt.Sprintf("Redis key type %q is not supported by the data viewer", keyType)}
	}
	if err != nil {
		return nil, redisProtocolError("BROWSE_FAILED", err)
	}
	if err := dbadapter.ValidateBrowseRowsResult(result); err != nil {
		return nil, redisProtocolError("BROWSE_RESULT_FAILED", err)
	}
	return result, nil
}

func redisBrowseResult(payload dbadapter.BrowseRowsPayload, columns []dbadapter.BrowseColumn, rows []dbadapter.BrowseRow, hasMore bool, total *int64, editable bool, reason string) dbadapter.BrowseRowsResult {
	if !editable && reason == "" {
		reason = "Redis key type is read-only in the grid editor"
	}
	return dbadapter.BrowseRowsResult{
		Columns: columns,
		Rows: rows,
		Offset: payload.Offset,
		Limit: payload.Limit,
		HasMore: hasMore,
		TotalRows: total,
		Editable: editable,
		EditabilityReason: reason,
	}
}

func (h *Handler) redisGridEditable() bool {
	return h != nil && !h.config.ReadOnly
}

func (h *Handler) browseRedisString(ctx context.Context, key string, payload dbadapter.BrowseRowsPayload) (dbadapter.BrowseRowsResult, error) {
	total := int64(1)
	if payload.Offset > 0 {
		return redisBrowseResult(payload, []dbadapter.BrowseColumn{{Name: "value", Type: "redis:string"}}, []dbadapter.BrowseRow{}, false, &total, false, "Redis strings use the key context tools for writes"), nil
	}
	value, err := h.client.Do(ctx, "GET", key)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	cell, _ := normalizeRedisCell(value, 0)
	rows := []dbadapter.BrowseRow{{Values: []interface{}{cell}, Identity: map[string]interface{}{"key": key}}}
	return redisBrowseResult(payload, []dbadapter.BrowseColumn{{Name: "value", Type: "redis:string"}}, rows, false, &total, false, "Redis strings use the key context tools for writes"), nil
}

func (h *Handler) browseRedisList(ctx context.Context, key string, payload dbadapter.BrowseRowsPayload) (dbadapter.BrowseRowsResult, error) {
	length, err := h.redisInteger(ctx, "LLEN", key)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	end := payload.Offset + payload.Limit
	value, err := h.client.Do(ctx, "LRANGE", key, strconv.Itoa(payload.Offset), strconv.Itoa(end))
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	if value.Kind != KindArray {
		return dbadapter.BrowseRowsResult{}, fmt.Errorf("Redis LRANGE returned an unexpected response")
	}
	hasMore := len(value.Array) > payload.Limit
	items := value.Array
	if hasMore {
		items = items[:payload.Limit]
	}
	rows := make([]dbadapter.BrowseRow, 0, len(items))
	for i, item := range items {
		cell, _ := normalizeRedisCell(item, 0)
		index := payload.Offset + i
		rows = append(rows, dbadapter.BrowseRow{
			Values: []interface{}{index, cell},
			Identity: map[string]interface{}{"index": index},
		})
	}
	return redisBrowseResult(payload, []dbadapter.BrowseColumn{
		{Name: "index", Type: "integer", Identity: true},
		{Name: "value", Type: "redis:list"},
	}, rows, hasMore, &length, false, "Redis lists are read-only in the grid editor"), nil
}

func (h *Handler) browseRedisHash(ctx context.Context, key string, payload dbadapter.BrowseRowsPayload) (dbadapter.BrowseRowsResult, error) {
	total, err := h.redisInteger(ctx, "HLEN", key)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	rows, hasMore, err := h.scanRedisEntries(ctx, "HSCAN", key, payload, true)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	editable := h.redisGridEditable()
	return redisBrowseResult(payload, []dbadapter.BrowseColumn{
		{Name: "field", Type: "redis:hash-field", Identity: true, Editable: false},
		{Name: "value", Type: "redis:hash-value", Editable: editable},
	}, rows, hasMore, &total, editable, map[bool]string{true: "", false: "Redis connection is read-only"}[editable]), nil
}

func (h *Handler) browseRedisSet(ctx context.Context, key string, payload dbadapter.BrowseRowsPayload) (dbadapter.BrowseRowsResult, error) {
	total, err := h.redisInteger(ctx, "SCARD", key)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	rows, hasMore, err := h.scanRedisEntries(ctx, "SSCAN", key, payload, false)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	editable := h.redisGridEditable()
	return redisBrowseResult(payload, []dbadapter.BrowseColumn{
		{Name: "member", Type: "redis:set-member", Identity: true, Editable: false},
	}, rows, hasMore, &total, editable, map[bool]string{true: "", false: "Redis connection is read-only"}[editable]), nil
}

func (h *Handler) browseRedisZSet(ctx context.Context, key string, payload dbadapter.BrowseRowsPayload) (dbadapter.BrowseRowsResult, error) {
	total, err := h.redisInteger(ctx, "ZCARD", key)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	end := payload.Offset + payload.Limit
	value, err := h.client.Do(ctx, "ZRANGE", key, strconv.Itoa(payload.Offset), strconv.Itoa(end), "WITHSCORES")
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	if value.Kind != KindArray || len(value.Array)%2 != 0 {
		return dbadapter.BrowseRowsResult{}, fmt.Errorf("Redis ZRANGE returned an unexpected response")
	}
	count := len(value.Array) / 2
	hasMore := count > payload.Limit
	if hasMore {
		count = payload.Limit
	}
	rows := make([]dbadapter.BrowseRow, 0, count)
	for i := 0; i < count; i++ {
		member, err := redisValueString(value.Array[i*2])
		if err != nil {
			return dbadapter.BrowseRowsResult{}, err
		}
		score, err := redisValueString(value.Array[i*2+1])
		if err != nil {
			return dbadapter.BrowseRowsResult{}, err
		}
		rows = append(rows, dbadapter.BrowseRow{
			Values: []interface{}{member, score},
			Identity: map[string]interface{}{"member": member},
		})
	}
	editable := h.redisGridEditable()
	return redisBrowseResult(payload, []dbadapter.BrowseColumn{
		{Name: "member", Type: "redis:zset-member", Identity: true, Editable: false},
		{Name: "score", Type: "redis:zset-score", Editable: editable},
	}, rows, hasMore, &total, editable, map[bool]string{true: "", false: "Redis connection is read-only"}[editable]), nil
}

func (h *Handler) scanRedisEntries(ctx context.Context, command, key string, payload dbadapter.BrowseRowsPayload, pairs bool) ([]dbadapter.BrowseRow, bool, error) {
	cursor := "0"
	skipped := 0
	rows := make([]dbadapter.BrowseRow, 0, payload.Limit+1)
	for {
		value, err := h.client.Do(ctx, command, key, cursor, "COUNT", strconv.Itoa(scanCountHint))
		if err != nil {
			return nil, false, err
		}
		if value.Kind != KindArray || len(value.Array) != 2 {
			return nil, false, fmt.Errorf("Redis %s returned an unexpected response", command)
		}
		nextCursor, err := redisValueString(value.Array[0])
		if err != nil {
			return nil, false, err
		}
		items := value.Array[1]
		if items.Kind != KindArray {
			return nil, false, fmt.Errorf("Redis %s items are not an array", command)
		}
		step := 1
		if pairs {
			step = 2
			if len(items.Array)%2 != 0 {
				return nil, false, fmt.Errorf("Redis %s returned an odd field/value list", command)
			}
		}
		for i := 0; i < len(items.Array); i += step {
			if skipped < payload.Offset {
				skipped++
				continue
			}
			if len(rows) > payload.Limit {
				break
			}
			first, err := redisValueString(items.Array[i])
			if err != nil {
				return nil, false, err
			}
			if pairs {
				cell, _ := normalizeRedisCell(items.Array[i+1], 0)
				rows = append(rows, dbadapter.BrowseRow{
					Values: []interface{}{first, cell},
					Identity: map[string]interface{}{"field": first},
				})
			} else {
				rows = append(rows, dbadapter.BrowseRow{
					Values: []interface{}{first},
					Identity: map[string]interface{}{"member": first},
				})
			}
		}
		if len(rows) > payload.Limit {
			return rows[:payload.Limit], true, nil
		}
		if nextCursor == "0" {
			return rows, false, nil
		}
		cursor = nextCursor
	}
}

func (h *Handler) redisInteger(ctx context.Context, command, key string) (int64, error) {
	value, err := h.client.Do(ctx, command, key)
	if err != nil {
		return 0, err
	}
	if value.Kind != KindInteger {
		return 0, fmt.Errorf("Redis %s returned an unexpected response", command)
	}
	return value.Integer, nil
}


func (h *Handler) mutateKey(ctx context.Context, payload dbadapter.MutateRowsPayload) (interface{}, *dbadapter.ProtocolError) {
	if kind := strings.TrimSpace(payload.Kind); kind != "" && !strings.EqualFold(kind, "key") {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "Redis mutations support keys only"}
	}
	current := redisCatalogName(h.config.Database)
	if catalog := strings.TrimSpace(payload.Catalog); catalog != "" && catalog != current {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "Redis key is outside the connected logical database"}
	}
	key := strings.TrimSpace(payload.Name)
	if key == "" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "Redis key is required"}
	}
	typeValue, err := h.client.Do(ctx, "TYPE", key)
	if err != nil {
		return nil, redisProtocolError("MUTATION_FAILED", err)
	}
	keyType, err := redisValueString(typeValue)
	if err != nil {
		return nil, redisProtocolError("MUTATION_FAILED", err)
	}
	results := make([]dbadapter.RowMutationResult, 0, len(payload.Mutations))
	for index, mutation := range payload.Mutations {
		item := dbadapter.RowMutationResult{Index: index, Action: mutation.Action}
		var mutationErr error
		switch strings.ToLower(keyType) {
		case "hash":
			item.AffectedRows, mutationErr = h.mutateRedisHash(ctx, key, mutation)
		case "set":
			item.AffectedRows, mutationErr = h.mutateRedisSet(ctx, key, mutation)
		case "zset":
			item.AffectedRows, mutationErr = h.mutateRedisZSet(ctx, key, mutation)
		default:
			mutationErr = fmt.Errorf("Redis %s keys are read-only in the grid editor", keyType)
		}
		if mutationErr != nil {
			item.Error = redisProtocolError("MUTATION_FAILED", mutationErr)
		}
		results = append(results, item)
	}
	out := dbadapter.MutateRowsResult{Results: results}
	if err := dbadapter.ValidateMutateRowsResult(out); err != nil {
		return nil, redisProtocolError("MUTATION_RESULT_FAILED", err)
	}
	return out, nil
}

func redisMutationText(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case json.Number:
		return typed.String(), nil
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), nil
	case bool:
		if typed {
			return "1", nil
		}
		return "0", nil
	case nil:
		return "", fmt.Errorf("Redis mutation value must not be NULL")
	default:
		return "", fmt.Errorf("Redis mutation value type %T is not supported", value)
	}
}

func redisIdentityText(identity map[string]interface{}, name string) (string, error) {
	if len(identity) != 1 {
		return "", fmt.Errorf("Redis mutation requires exactly one %s identity", name)
	}
	value, ok := identity[name]
	if !ok {
		return "", fmt.Errorf("Redis mutation requires %s identity", name)
	}
	return redisMutationText(value)
}

func redisOnlyValue(values map[string]interface{}, names ...string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("Redis mutation values are required")
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if !allowed[key] {
			return nil, fmt.Errorf("Redis mutation field %q is not allowed", key)
		}
		text, err := redisMutationText(value)
		if err != nil {
			return nil, err
		}
		out[key] = text
	}
	return out, nil
}

func (h *Handler) mutateRedisHash(ctx context.Context, key string, mutation dbadapter.RowMutation) (int64, error) {
	switch mutation.Action {
	case "insert":
		values, err := redisOnlyValue(mutation.Values, "field", "value")
		if err != nil || len(values) != 2 {
			if err == nil {
				err = fmt.Errorf("Redis hash insert requires field and value")
			}
			return 0, err
		}
		reply, err := h.client.Do(ctx, "HSETNX", key, values["field"], values["value"])
		if err != nil {
			return 0, err
		}
		if reply.Kind != KindInteger || reply.Integer != 1 {
			return 0, fmt.Errorf("Redis hash field already exists")
		}
		return 1, nil
	case "update":
		field, err := redisIdentityText(mutation.Identity, "field")
		if err != nil {
			return 0, err
		}
		values, err := redisOnlyValue(mutation.Values, "value")
		if err != nil || len(values) != 1 {
			if err == nil {
				err = fmt.Errorf("Redis hash update requires value only")
			}
			return 0, err
		}
		exists, err := h.client.Do(ctx, "HEXISTS", key, field)
		if err != nil {
			return 0, err
		}
		if exists.Kind != KindInteger || exists.Integer != 1 {
			return 0, fmt.Errorf("Redis hash field no longer exists")
		}
		if _, err := h.client.Do(ctx, "HSET", key, field, values["value"]); err != nil {
			return 0, err
		}
		return 1, nil
	case "delete":
		field, err := redisIdentityText(mutation.Identity, "field")
		if err != nil {
			return 0, err
		}
		reply, err := h.client.Do(ctx, "HDEL", key, field)
		if err != nil {
			return 0, err
		}
		if reply.Kind != KindInteger || reply.Integer != 1 {
			return 0, fmt.Errorf("Redis hash field no longer exists")
		}
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported Redis hash mutation %q", mutation.Action)
	}
}

func (h *Handler) mutateRedisSet(ctx context.Context, key string, mutation dbadapter.RowMutation) (int64, error) {
	switch mutation.Action {
	case "insert":
		values, err := redisOnlyValue(mutation.Values, "member")
		if err != nil || len(values) != 1 {
			if err == nil {
				err = fmt.Errorf("Redis set insert requires member")
			}
			return 0, err
		}
		reply, err := h.client.Do(ctx, "SADD", key, values["member"])
		if err != nil {
			return 0, err
		}
		if reply.Kind != KindInteger || reply.Integer != 1 {
			return 0, fmt.Errorf("Redis set member already exists")
		}
		return 1, nil
	case "delete":
		member, err := redisIdentityText(mutation.Identity, "member")
		if err != nil {
			return 0, err
		}
		reply, err := h.client.Do(ctx, "SREM", key, member)
		if err != nil {
			return 0, err
		}
		if reply.Kind != KindInteger || reply.Integer != 1 {
			return 0, fmt.Errorf("Redis set member no longer exists")
		}
		return 1, nil
	case "update":
		return 0, fmt.Errorf("Redis set members are identity values; delete and insert to rename a member")
	default:
		return 0, fmt.Errorf("unsupported Redis set mutation %q", mutation.Action)
	}
}

func (h *Handler) mutateRedisZSet(ctx context.Context, key string, mutation dbadapter.RowMutation) (int64, error) {
	switch mutation.Action {
	case "insert":
		values, err := redisOnlyValue(mutation.Values, "member", "score")
		if err != nil || len(values) != 2 {
			if err == nil {
				err = fmt.Errorf("Redis sorted-set insert requires member and score")
			}
			return 0, err
		}
		if _, err := strconv.ParseFloat(values["score"], 64); err != nil {
			return 0, fmt.Errorf("Redis sorted-set score is invalid")
		}
		reply, err := h.client.Do(ctx, "ZADD", key, "NX", values["score"], values["member"])
		if err != nil {
			return 0, err
		}
		if reply.Kind != KindInteger || reply.Integer != 1 {
			return 0, fmt.Errorf("Redis sorted-set member already exists")
		}
		return 1, nil
	case "update":
		member, err := redisIdentityText(mutation.Identity, "member")
		if err != nil {
			return 0, err
		}
		values, err := redisOnlyValue(mutation.Values, "score")
		if err != nil || len(values) != 1 {
			if err == nil {
				err = fmt.Errorf("Redis sorted-set update requires score only")
			}
			return 0, err
		}
		if _, err := strconv.ParseFloat(values["score"], 64); err != nil {
			return 0, fmt.Errorf("Redis sorted-set score is invalid")
		}
		exists, err := h.client.Do(ctx, "ZSCORE", key, member)
		if err != nil {
			return 0, err
		}
		if exists.Kind == KindNull {
			return 0, fmt.Errorf("Redis sorted-set member no longer exists")
		}
		if _, err := h.client.Do(ctx, "ZADD", key, "XX", values["score"], member); err != nil {
			return 0, err
		}
		return 1, nil
	case "delete":
		member, err := redisIdentityText(mutation.Identity, "member")
		if err != nil {
			return 0, err
		}
		reply, err := h.client.Do(ctx, "ZREM", key, member)
		if err != nil {
			return 0, err
		}
		if reply.Kind != KindInteger || reply.Integer != 1 {
			return 0, fmt.Errorf("Redis sorted-set member no longer exists")
		}
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported Redis sorted-set mutation %q", mutation.Action)
	}
}

func (h *Handler) redisObjectAction(ctx context.Context, payload dbadapter.ObjectActionPayload) (interface{}, *dbadapter.ProtocolError) {
	if kind := strings.TrimSpace(payload.Kind); kind != "" && !strings.EqualFold(kind, "key") {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "Redis object actions support keys only"}
	}
	current := redisCatalogName(h.config.Database)
	if catalog := strings.TrimSpace(payload.Catalog); catalog != "" && catalog != current {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_UNAVAILABLE", Message: "Redis key is outside the connected logical database"}
	}
	key := strings.TrimSpace(payload.Name)
	if key == "" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "Redis key is required"}
	}
	switch payload.Action {
	case "count_rows":
		typeValue, err := h.client.Do(ctx, "TYPE", key)
		if err != nil {
			return nil, redisProtocolError("COUNT_FAILED", err)
		}
		keyType, err := redisValueString(typeValue)
		if err != nil {
			return nil, redisProtocolError("COUNT_FAILED", err)
		}
		var count int64
		switch strings.ToLower(keyType) {
		case "none":
			count = 0
		case "string":
			count = 1
		case "list":
			count, err = h.redisInteger(ctx, "LLEN", key)
		case "hash":
			count, err = h.redisInteger(ctx, "HLEN", key)
		case "set":
			count, err = h.redisInteger(ctx, "SCARD", key)
		case "zset":
			count, err = h.redisInteger(ctx, "ZCARD", key)
		default:
			return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "Redis key type cannot be counted"}
		}
		if err != nil {
			return nil, redisProtocolError("COUNT_FAILED", err)
		}
		return dbadapter.ObjectActionResult{Count: &count}, nil
	case "drop":
		if h.config.ReadOnly {
			return nil, &dbadapter.ProtocolError{Code: "READ_ONLY", Message: "Redis connection is read-only"}
		}
		reply, err := h.client.Do(ctx, "DEL", key)
		if err != nil {
			return nil, redisProtocolError("DROP_FAILED", err)
		}
		if reply.Kind != KindInteger {
			return nil, &dbadapter.ProtocolError{Code: "DROP_FAILED", Message: "Redis DEL returned an unexpected response"}
		}
		return dbadapter.ObjectActionResult{AffectedRows: reply.Integer, Message: "Key deleted"}, nil
	case "truncate":
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "Redis keys do not support truncate; delete the key instead"}
	default:
		return nil, &dbadapter.ProtocolError{Code: "INVALID_OBJECT_ACTION", Message: "Unsupported Redis key action"}
	}
}
