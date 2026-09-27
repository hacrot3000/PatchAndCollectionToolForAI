package dbadapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewRequestAndResponseRoundTrip(t *testing.T) {
	request, err := NewRequest("req-1", OpPing, map[string]interface{}{"value": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvelope(request); err != nil {
		t.Fatal(err)
	}
	response, err := NewResponse(request, map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	if response.RequestID != request.RequestID || response.Operation != OpPing || response.Type != "response" {
		t.Fatalf("response=%+v", response)
	}
	if err := ValidateEnvelope(response); err != nil {
		t.Fatal(err)
	}
}

func TestEnvelopeRejectsUnknownVersionAndOperation(t *testing.T) {
	for _, env := range []Envelope{
		{Protocol: ProtocolName, Version: ProtocolVersion + 1, Type: "request", RequestID: "req-1", Operation: OpPing},
		{Protocol: ProtocolName, Version: ProtocolVersion, Type: "request", RequestID: "req-1", Operation: Operation("drop_everything")},
	} {
		if err := ValidateEnvelope(env); err == nil {
			t.Fatalf("expected invalid envelope: %+v", env)
		}
	}
}

func TestErrorResponseCannotCarryPayload(t *testing.T) {
	request, err := NewRequest("req-1", OpConnect, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := NewErrorResponse(request, "AUTH_FAILED", "authentication failed")
	if err != nil {
		t.Fatal(err)
	}
	response.Payload = json.RawMessage("{\"unexpected\":true}")
	if err := ValidateEnvelope(response); err == nil {
		t.Fatal("expected response with error and payload to fail")
	}
}

func TestExecuteResultBoundsRowsColumnsAndCells(t *testing.T) {
	valid := ExecuteResult{
		Columns: []Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}},
		Rows: [][]interface{}{{1, "Alice"}, {2, "Bob"}},
	}
	if err := ValidateExecuteResult(valid); err != nil {
		t.Fatal(err)
	}

	tooManyRows := ExecuteResult{Rows: make([][]interface{}, MaxRows+1)}
	if err := ValidateExecuteResult(tooManyRows); err == nil || !strings.Contains(err.Error(), "rows") {
		t.Fatalf("too many rows error=%v", err)
	}

	tooManyColumns := ExecuteResult{Columns: make([]Column, MaxColumns+1)}
	if err := ValidateExecuteResult(tooManyColumns); err == nil || !strings.Contains(err.Error(), "columns") {
		t.Fatalf("too many columns error=%v", err)
	}

	tooLargeCell := ExecuteResult{
		Columns: []Column{{Name: "value"}},
		Rows: [][]interface{}{{strings.Repeat("x", MaxCellBytes+1)}},
	}
	if err := ValidateExecuteResult(tooLargeCell); err == nil || !strings.Contains(err.Error(), "cell") {
		t.Fatalf("oversized cell error=%v", err)
	}
}

func TestExecuteResultRejectsRowWidthMismatch(t *testing.T) {
	result := ExecuteResult{
		Columns: []Column{{Name: "id"}, {Name: "name"}},
		Rows:    [][]interface{}{{1}},
	}
	if err := ValidateExecuteResult(result); err == nil || !strings.Contains(err.Error(), "2 columns") {
		t.Fatalf("error=%v", err)
	}
}

func TestRequestIDsRejectControlAndPathCharacters(t *testing.T) {
	for _, id := range []string{"", "../req", "req/1", "req\n2"} {
		if _, err := NewRequest(id, OpPing, nil); err == nil {
			t.Fatalf("request id %q unexpectedly accepted", id)
		}
	}
}
