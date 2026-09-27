package dbredis

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestExecuteResultFromScalar(t *testing.T) {
	result, err := executeResultFromValue(Value{Kind: KindInteger, Integer: 42}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) != 1 || len(result.Rows) != 1 || result.Rows[0][0] != int64(42) {
		t.Fatalf("result=%+v", result)
	}
}

func TestExecuteResultFromArrayTruncatesRows(t *testing.T) {
	result, err := executeResultFromValue(Value{
		Kind: KindArray,
		Array: []Value{
			{Kind: KindBulkString, Text: "a"},
			{Kind: KindBulkString, Text: "b"},
			{Kind: KindBulkString, Text: "c"},
		},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || !result.Truncated {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Columns) != 2 || result.Columns[0].Name != "index" {
		t.Fatalf("columns=%+v", result.Columns)
	}
}

func TestNormalizeRedisCellBoundsLargeTextAndNestedArrays(t *testing.T) {
	cell, truncated := normalizeRedisCell(Value{
		Kind: KindBulkString,
		Text: strings.Repeat("x", maxRedisCellText+1),
	}, 0)
	if !truncated || len(cell.(string)) != maxRedisCellText {
		t.Fatalf("cell len=%d truncated=%v", len(cell.(string)), truncated)
	}

	nested := Value{Kind: KindArray, Array: make([]Value, maxRedisNestedItems+1)}
	cell, truncated = normalizeRedisCell(nested, 0)
	items, ok := cell.([]interface{})
	if !ok || len(items) != maxRedisNestedItems || !truncated {
		t.Fatalf("nested cell type=%T len=%d truncated=%v", cell, len(items), truncated)
	}
}

func TestRedisResultFitsGenericProtocolBounds(t *testing.T) {
	result, err := executeResultFromValue(Value{
		Kind: KindArray,
		Array: []Value{
			{Kind: KindSimpleString, Text: "OK"},
			{Kind: KindNull},
		},
	}, dbadapter.MaxRows)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbadapter.ValidateExecuteResult(result); err != nil {
		t.Fatal(err)
	}
}
