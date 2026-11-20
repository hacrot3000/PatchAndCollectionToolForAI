package agentchat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type DiagnosticEvent struct {
	At                time.Time
	Level             string
	Stage             string
	Event             string
	Code              string
	SafeMessage       string
	Endpoint          string
	APIType           string
	HTTPStatus        int
	ContentType       string
	RetryAfter        string
	ProviderRequestID string
	ProviderID        string
	ModelID           string
	Duration          time.Duration
	RequestBody       []byte `json:"-"`
	ResponseBody      []byte `json:"-"`
	StreamFrame       []byte `json:"-"`
	RequestSHA256     string
	ResponseSHA256    string
	RequestBytes      int
	ResponseBytes     int
	Truncated         bool
	ContextMode       string
	Reasoning         string
	HistoryItems      int
	ToolCount         int
	Streaming         bool
	APIKeyPresent     bool
}

type DiagnosticSink interface {
	RecordProviderDiagnostic(DiagnosticEvent)
}

type diagnosticContextKey struct{}

func WithDiagnosticSink(ctx context.Context, sink DiagnosticSink) context.Context {
	return context.WithValue(ctx, diagnosticContextKey{}, sink)
}

func RecordProviderDiagnostic(ctx context.Context, event DiagnosticEvent) {
	if ctx == nil {
		return
	}
	if sink, ok := ctx.Value(diagnosticContextKey{}).(DiagnosticSink); ok && sink != nil {
		if event.At.IsZero() {
			event.At = time.Now().UTC()
		}
		sink.RecordProviderDiagnostic(event)
	}
}

type ProviderError struct {
	Code              string
	Stage             string
	Message           string
	ProviderErrorType string
	ProviderErrorCode string
	ProviderMessage   string
	HTTPStatus        int
	RetryAfter        string
	Retryable         bool
	Ambiguous         bool
	TraceID           string
	BlockedSharedIP   string
}

func (e *ProviderError) Error() string {
	if e == nil || e.Message == "" {
		return "provider request failed"
	}
	return e.Message
}

func BodyDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func SafeProviderText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	for _, prefix := range []string{"Bearer ", "bearer ", "sk-", "api_key=", "api-key="} {
		if index := strings.Index(value, prefix); index >= 0 {
			value = value[:index] + "[redacted]"
		}
	}
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}
