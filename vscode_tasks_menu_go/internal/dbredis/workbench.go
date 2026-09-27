package dbredis

import (
	"context"
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

func redisReadOnlyResult(payload dbadapter.BrowseRowsPayload, columns []dbadapter.BrowseColumn, rows []dbadapter.BrowseRow, hasMore bool, total *int64) dbadapter.BrowseRowsResult {
	return dbadapter.BrowseRowsResult{
		Columns: columns,
		Rows: rows,
		Offset: payload.Offset,
		Limit: payload.Limit,
		HasMore: hasMore,
		TotalRows: total,
		Editable: false,
		EditabilityReason: "Redis viewer is read-only; type-aware mutations are enabled separately",
	}
}

func (h *Handler) browseRedisString(ctx context.Context, key string, payload dbadapter.BrowseRowsPayload) (dbadapter.BrowseRowsResult, error) {
	total := int64(1)
	if payload.Offset > 0 {
		return redisReadOnlyResult(payload, []dbadapter.BrowseColumn{{Name: "value", Type: "redis:string"}}, []dbadapter.BrowseRow{}, false, &total), nil
	}
	value, err := h.client.Do(ctx, "GET", key)
	if err != nil {
		return dbadapter.BrowseRowsResult{}, err
	}
	cell, _ := normalizeRedisCell(value, 0)
	rows := []dbadapter.BrowseRow{{Values: []interface{}{cell}, Identity: map[string]interface{}{"key": key}}}
	return redisReadOnlyResult(payload, []dbadapter.BrowseColumn{{Name: "value", Type: "redis:string"}}, rows, false, &total), nil
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
	return redisReadOnlyResult(payload, []dbadapter.BrowseColumn{
		{Name: "index", Type: "integer", Identity: true},
		{Name: "value", Type: "redis:list"},
	}, rows, hasMore, &length), nil
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
	return redisReadOnlyResult(payload, []dbadapter.BrowseColumn{
		{Name: "field", Type: "redis:hash-field", Identity: true},
		{Name: "value", Type: "redis:hash-value"},
	}, rows, hasMore, &total), nil
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
	return redisReadOnlyResult(payload, []dbadapter.BrowseColumn{
		{Name: "member", Type: "redis:set-member", Identity: true},
	}, rows, hasMore, &total), nil
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
	return redisReadOnlyResult(payload, []dbadapter.BrowseColumn{
		{Name: "member", Type: "redis:zset-member", Identity: true},
		{Name: "score", Type: "redis:zset-score"},
	}, rows, hasMore, &total), nil
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
