package dbmongo

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildScriptEmbedsEncodedURIAndMarker(t *testing.T) {
	config := Config{
		Host: "db.example.com", Port: 27017,
		Username: "user", Password: "p@ss/word",
		Database: "main", ConnectTimeout: 10 * time.Second,
	}
	script, err := buildScript(config, "return {value:42};")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "connect(") || !strings.Contains(script, resultMarker) {
		t.Fatalf("script missing protocol scaffolding: %s", script)
	}
	if strings.Contains(script, "p@ss/word") {
		t.Fatalf("raw reserved password was not URI encoded in script: %s", script)
	}
	if !strings.Contains(script, "return {value:42};") {
		t.Fatalf("operation body missing: %s", script)
	}
}

func TestJavaScriptJSONUsesJSONParseStringBoundary(t *testing.T) {
	malicious := "x\"; throw new Error('boom');//"
	value := map[string]interface{}{
		"name":  malicious,
		"proto": map[string]interface{}{"__proto__": "safe-data"},
	}
	expression, err := javascriptJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(expression, "JSON.parse(") || !strings.HasSuffix(expression, ")") {
		t.Fatalf("expression=%q", expression)
	}
	literal := strings.TrimSuffix(strings.TrimPrefix(expression, "JSON.parse("), ")")
	jsonText, err := strconv.Unquote(literal)
	if err != nil {
		t.Fatalf("JSON.parse argument is not a single quoted JavaScript/JSON string: %v expression=%s", err, expression)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(jsonText), &decoded); err != nil {
		t.Fatalf("JSON.parse payload is not valid JSON: %v payload=%q", err, jsonText)
	}
	if decoded["name"] != malicious {
		t.Fatalf("malicious-looking text was not preserved as data: %#v", decoded["name"])
	}
}

func TestParseScriptResultDecodesJSONAndRedactsErrors(t *testing.T) {
	var result map[string]interface{}
	output := []byte(resultMarker + `{"ok":true,"result":{"name":"Alice","count":2}}` + "\n")
	if err := parseScriptResult(output, "", &result); err != nil {
		t.Fatal(err)
	}
	if result["name"] != "Alice" {
		t.Fatalf("result=%v", result)
	}
	if count, ok := result["count"].(json.Number); !ok || count.String() != "2" {
		t.Fatalf("count=%T %#v", result["count"], result["count"])
	}

	err := parseScriptResult([]byte(resultMarker+`{"ok":false,"error":"auth failed top-secret/encoded and top-secret%2Fencoded"}`+"\n"), "top-secret/encoded", nil)
	if err == nil || strings.Contains(err.Error(), "top-secret") || strings.Contains(err.Error(), "%2Fencoded") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseScriptResultRejectsUnexpectedOrMultipleOutput(t *testing.T) {
	for _, output := range [][]byte{
		[]byte("unexpected\n" + resultMarker + `{"ok":true,"result":1}` + "\n"),
		[]byte(resultMarker + `{"ok":true,"result":1}` + "\n" + resultMarker + `{"ok":true,"result":2}` + "\n"),
		[]byte(""),
	} {
		if err := parseScriptResult(output, "", new(interface{})); err == nil {
			t.Fatalf("unexpected output accepted: %q", output)
		}
	}
}
