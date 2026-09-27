package dbredis

import (
	"fmt"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	maxRedisCellText     = 64 << 10
	maxRedisNestedItems  = 256
	maxRedisNestedDepth  = 8
)

func executeResultFromValue(value Value, maxRows int) (dbadapter.ExecuteResult, error) {
	if maxRows < 1 || maxRows > dbadapter.MaxRows {
		return dbadapter.ExecuteResult{}, fmt.Errorf("Redis max rows must be between 1 and %d", dbadapter.MaxRows)
	}
	result := dbadapter.ExecuteResult{}
	switch value.Kind {
	case KindArray:
		result.Columns = []dbadapter.Column{
			{Name: "index", Type: "integer"},
			{Name: "value", Type: "redis"},
		}
		limit := len(value.Array)
		if limit > maxRows {
			limit = maxRows
			result.Truncated = true
		}
		result.Rows = make([][]interface{}, 0, limit)
		for i := 0; i < limit; i++ {
			cell, truncated := normalizeRedisCell(value.Array[i], 0)
			result.Truncated = result.Truncated || truncated
			result.Rows = append(result.Rows, []interface{}{i, cell})
		}
	default:
		cell, truncated := normalizeRedisCell(value, 0)
		result.Columns = []dbadapter.Column{{Name: "value", Type: "redis"}}
		result.Rows = [][]interface{}{{cell}}
		result.Truncated = truncated
	}
	if err := dbadapter.ValidateExecuteResult(result); err != nil {
		return dbadapter.ExecuteResult{}, err
	}
	return result, nil
}

func normalizeRedisCell(value Value, depth int) (interface{}, bool) {
	if depth > maxRedisNestedDepth {
		return "[nested value truncated]", true
	}
	switch value.Kind {
	case KindNull:
		return nil, false
	case KindInteger:
		return value.Integer, false
	case KindSimpleString, KindBulkString:
		if len(value.Text) > maxRedisCellText {
			return value.Text[:maxRedisCellText], true
		}
		return value.Text, false
	case KindArray:
		limit := len(value.Array)
		truncated := false
		if limit > maxRedisNestedItems {
			limit = maxRedisNestedItems
			truncated = true
		}
		out := make([]interface{}, 0, limit)
		for i := 0; i < limit; i++ {
			item, itemTruncated := normalizeRedisCell(value.Array[i], depth+1)
			truncated = truncated || itemTruncated
			out = append(out, item)
		}
		return out, truncated
	case KindError:
		text := value.Text
		if len(text) > maxRedisCellText {
			text = text[:maxRedisCellText]
			return text, true
		}
		return text, false
	default:
		return nil, true
	}
}

func redisValueString(value Value) (string, error) {
	switch value.Kind {
	case KindSimpleString, KindBulkString:
		return value.Text, nil
	case KindInteger:
		return fmt.Sprint(value.Integer), nil
	case KindNull:
		return "", nil
	default:
		return "", fmt.Errorf("Redis value kind %q is not scalar text", value.Kind)
	}
}
