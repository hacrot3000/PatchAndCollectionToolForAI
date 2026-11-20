package dbadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type testHandler struct {
	manifest Manifest
	calls    int
}

func (h *testHandler) Manifest() Manifest {
	return h.manifest
}

func (h *testHandler) Handle(_ context.Context, request Envelope) (interface{}, *ProtocolError) {
	h.calls++
	if request.Operation == OpPing {
		return map[string]bool{"pong": true}, nil
	}
	return nil, &ProtocolError{Code: "UNEXPECTED", Message: "unexpected operation"}
}

func testServeManifest() Manifest {
	return Manifest{
		ID:              "test",
		Name:            "Test",
		Kind:            "test",
		ProtocolVersion: ProtocolVersion,
		Command:         "/bin/true",
		Capabilities: CapabilitySet{
			Connect: true,
			Ping:    true,
		},
	}
}

func encodeRequestLine(t *testing.T, id string, operation Operation) string {
	t.Helper()
	request, err := NewRequest(id, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func TestServeHandlesBuiltInHelloAndCapabilitiesWithoutHandlerDispatch(t *testing.T) {
	handler := &testHandler{manifest: testServeManifest()}
	input := encodeRequestLine(t, "req-1", OpHello) +
		encodeRequestLine(t, "req-2", OpCapabilities)
	var output bytes.Buffer

	if err := Serve(context.Background(), strings.NewReader(input), &output, handler); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls=%d want 0", handler.calls)
	}

	scanner := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(scanner) != 2 {
		t.Fatalf("responses=%d output=%q", len(scanner), output.String())
	}
	for i, line := range scanner {
		var response Envelope
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatal(err)
		}
		if response.Type != "response" || response.RequestID == "" || response.Error != nil {
			t.Fatalf("response %d=%+v", i, response)
		}
	}
}

func TestServeDispatchesSupportedOperation(t *testing.T) {
	handler := &testHandler{manifest: testServeManifest()}
	var output bytes.Buffer
	if err := Serve(
		context.Background(),
		strings.NewReader(encodeRequestLine(t, "req-1", OpPing)),
		&output,
		handler,
	); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1", handler.calls)
	}
	var response Envelope
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatal(err)
	}
	var payload map[string]bool
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload["pong"] {
		t.Fatalf("payload=%v", payload)
	}
}

func TestServeReturnsProtocolErrorForUnsupportedOperation(t *testing.T) {
	handler := &testHandler{manifest: testServeManifest()}
	var output bytes.Buffer
	if err := Serve(
		context.Background(),
		strings.NewReader(encodeRequestLine(t, "req-1", OpExecute)),
		&output,
		handler,
	); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls=%d want 0", handler.calls)
	}
	var response Envelope
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != "UNSUPPORTED_OPERATION" {
		t.Fatalf("response error=%+v", response.Error)
	}
}

func TestServeCancelInterruptsActiveOperation(t *testing.T) {
	manifest := testServeManifest()
	manifest.Capabilities.Execute = true
	manifest.Capabilities.Cancel = true
	handler := &cancelServeHandler{manifest: manifest}
	execute, err := NewRequest("req-execute", OpExecute, nil)
	if err != nil { t.Fatal(err) }
	cancelReq, err := NewRequest("req-cancel", OpCancel, CancelPayload{RequestID: "req-execute"})
	if err != nil { t.Fatal(err) }
	first, _ := json.Marshal(execute)
	second, _ := json.Marshal(cancelReq)
	var output bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(string(first)+"\n"+string(second)+"\n"), &output, handler); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 { t.Fatalf("responses=%d output=%q", len(lines), output.String()) }
	byID := map[string]Envelope{}
	for _, line := range lines {
		var env Envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil { t.Fatal(err) }
		byID[env.RequestID] = env
	}
	if byID["req-execute"].Error == nil || byID["req-execute"].Error.Code != "CANCELED" {
		t.Fatalf("execute response=%+v", byID["req-execute"])
	}
	var canceled CancelResult
	if err := json.Unmarshal(byID["req-cancel"].Payload, &canceled); err != nil { t.Fatal(err) }
	if !canceled.Canceled { t.Fatalf("cancel response=%+v", byID["req-cancel"]) }
}

type cancelServeHandler struct {
	manifest Manifest
}

func (h *cancelServeHandler) Manifest() Manifest { return h.manifest }

func (h *cancelServeHandler) Handle(ctx context.Context, request Envelope) (interface{}, *ProtocolError) {
	switch request.Operation {
	case OpExecute:
		<-ctx.Done()
		return nil, &ProtocolError{Code: "QUERY_CANCELED", Message: ctx.Err().Error()}
	case OpPing:
		return map[string]bool{"pong": true}, nil
	default:
		return map[string]bool{"ok": true}, nil
	}
}
