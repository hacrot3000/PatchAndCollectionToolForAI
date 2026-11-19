package agentchat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type testDiagnosticSink struct{ events []DiagnosticEvent }

func (s *testDiagnosticSink) RecordProviderDiagnostic(e DiagnosticEvent) {
	s.events = append(s.events, e)
}

func TestProviderHTTPErrorHasSpecificSafeFields(t *testing.T) {
	body := []byte(`{"error":{"type":"invalid_request_error","code":"unsupported_parameter","message":"reasoning is not supported; Bearer secret-value"}}`)
	err := parseProviderHTTPError(400, "", body)
	if err.Code != "PROVIDER_HTTP_400" || err.Stage != "provider_http" || err.HTTPStatus != 400 || err.Retryable || err.Ambiguous {
		t.Fatalf("incorrect provider error classification: %+v", err)
	}
	if err.ProviderErrorType != "invalid_request_error" || err.ProviderErrorCode != "unsupported_parameter" || strings.Contains(err.ProviderMessage, "secret-value") {
		t.Fatalf("provider error details unsafe/incomplete: %+v", err)
	}
}

func TestDiagnosticSinkIsContextScoped(t *testing.T) {
	sink := &testDiagnosticSink{}
	ctx := WithDiagnosticSink(context.Background(), sink)
	RecordProviderDiagnostic(ctx, DiagnosticEvent{Event: "http.request"})
	RecordProviderDiagnostic(context.Background(), DiagnosticEvent{Event: "ignored"})
	if len(sink.events) != 1 || sink.events[0].Event != "http.request" || sink.events[0].At.IsZero() {
		t.Fatalf("diagnostics were not context-scoped: %+v", sink.events)
	}
	wire, err := json.Marshal(sink.events[0])
	if err != nil || strings.Contains(string(wire), "RequestBody") {
		t.Fatalf("raw body must not be JSON-serialized by default: %s %v", wire, err)
	}
}
