package dbmongo

import (
	"encoding/json"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestParseFindQueryDefaultsAndBoundsLimit(t *testing.T) {
	query, err := ParseFindQuery("{\"collection\":\"users\",\"filter\":{\"active\":true},\"sort\":{\"created_at\":-1},\"limit\":500}", 100)
	if err != nil {
		t.Fatal(err)
	}
	if query.Op != "find" || query.Collection != "users" || query.Limit != 100 {
		t.Fatalf("query=%+v", query)
	}
	if query.Filter["active"] != true {
		t.Fatalf("filter=%v", query.Filter)
	}
	if number, ok := query.Sort["created_at"].(json.Number); !ok || number.String() != "-1" {
		t.Fatalf("sort=%#v", query.Sort)
	}
}

func TestParseFindQueryRejectsServerSideCodeAndUnsafeOperations(t *testing.T) {
	for _, statement := range []string{
		"{\"op\":\"aggregate\",\"collection\":\"users\"}",
		"{\"collection\":\"users\",\"filter\":{\"$where\":\"sleep(1000)\"}}",
		"{\"collection\":\"users\",\"filter\":{\"x\":{\"$function\":{\"body\":\"return true\",\"args\":[],\"lang\":\"js\"}}}}",
		"{\"collection\":\"users\",\"projection\":{\"x\":{\"$accumulator\":{}}}}",
		"{\"collection\":\"\"}",
		"{\"collection\":\"users\",\"limit\":-1}",
		"{\"collection\":\"users\",\"unknown\":true}",
		"{\"collection\":\"users\"} trailing",
	} {
		if _, err := ParseFindQuery(statement, 100); err == nil {
			t.Fatalf("unsafe query unexpectedly accepted: %s", statement)
		}
	}
}

func TestFindOperationBodyUsesJSONParseBoundary(t *testing.T) {
	query, err := ParseFindQuery("{\"collection\":\"users\\\\"; throw new Error('boom');//\",\"filter\":{\"name\":\"Alice\"}}", 10)
	if err != nil {
		t.Fatal(err)
	}
	body, err := findOperationBody("main", query)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "JSON.parse(") || !strings.Contains(body, ".find(") || !strings.Contains(body, ".limit(11)") {
		t.Fatalf("body=%s", body)
	}
	if strings.Contains(body, "users\"; throw new Error('boom');//") {
		t.Fatalf("collection escaped JSON string boundary: %s", body)
	}
}

func TestDocumentsToExecuteResultBoundsLargeCells(t *testing.T) {
	documents := []interface{}{
		map[string]interface{}{"name": "Alice"},
		map[string]interface{}{"blob": strings.Repeat("x", dbadapter.MaxCellBytes+1)},
	}
	result, err := documentsToExecuteResult(documents, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || !result.Truncated {
		t.Fatalf("result=%+v", result)
	}
	replacement, ok := result.Rows[1][0].(map[string]interface{})
	if !ok || replacement["_taskdeck_truncated"] != true {
		t.Fatalf("oversized document replacement=%#v", result.Rows[1][0])
	}
	if err := dbadapter.ValidateExecuteResult(result); err != nil {
		t.Fatal(err)
	}
}
