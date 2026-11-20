package dbadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

type Handler interface {
	Manifest() Manifest
	Handle(context.Context, Envelope) (interface{}, *ProtocolError)
}

func Serve(ctx context.Context, input io.Reader, output io.Writer, handler Handler) error {
	if ctx == nil {
		return errors.New("database adapter serve context is required")
	}
	if input == nil || output == nil {
		return errors.New("database adapter input/output is required")
	}
	if handler == nil {
		return errors.New("database adapter handler is required")
	}
	manifest := handler.Manifest()
	if err := manifest.Validate(); err != nil {
		return err
	}

	serveCtx, cancelServe := context.WithCancel(ctx)
	defer cancelServe()

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), MaxMessageBytes)
	encoder := json.NewEncoder(output)

	var (
		encodeMu sync.Mutex
		opMu     sync.Mutex
		activeMu sync.Mutex
		active   = make(map[string]context.CancelFunc)
		wg       sync.WaitGroup
		errMu    sync.Mutex
		asyncErr error
	)

	setAsyncErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if asyncErr == nil {
			asyncErr = err
			cancelServe()
		}
		errMu.Unlock()
	}
	writeResponse := func(response Envelope) error {
		encodeMu.Lock()
		defer encodeMu.Unlock()
		return encodeEnvelope(encoder, response)
	}
	writeError := func(request Envelope, code, message string) error {
		response, err := NewErrorResponse(request, code, message)
		if err != nil {
			return err
		}
		return writeResponse(response)
	}
	writePayload := func(request Envelope, payload interface{}) error {
		response, err := NewResponse(request, payload)
		if err != nil {
			return err
		}
		return writeResponse(response)
	}

	for scanner.Scan() {
		if serveCtx.Err() != nil {
			break
		}

		var request Envelope
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode database adapter request: %w", err)
		}
		if err := ValidateEnvelope(request); err != nil {
			return fmt.Errorf("validate database adapter request: %w", err)
		}
		if request.Type != "request" {
			return fmt.Errorf("database adapter expected request, got %q", request.Type)
		}
		if !manifest.Capabilities.Supports(request.Operation) {
			if err := writeError(request, "UNSUPPORTED_OPERATION", fmt.Sprintf("operation %q is not supported", request.Operation)); err != nil {
				return err
			}
			continue
		}

		switch request.Operation {
		case OpHello:
			if err := writePayload(request, map[string]interface{}{
				"adapter_id":       manifest.ID,
				"adapter_name":     manifest.Name,
				"adapter_kind":     manifest.Kind,
				"protocol_version": manifest.ProtocolVersion,
			}); err != nil {
				return err
			}
			continue
		case OpCapabilities:
			if err := writePayload(request, manifest.Capabilities); err != nil {
				return err
			}
			continue
		case OpCancel:
			var payload CancelPayload
			if err := json.Unmarshal(request.Payload, &payload); err != nil {
				if err := writeError(request, "INVALID_CANCEL", "cancel request payload is invalid"); err != nil {
					return err
				}
				continue
			}
			payload.RequestID = strings.TrimSpace(payload.RequestID)
			if err := validateToken("cancel request id", payload.RequestID, 128, true); err != nil {
				if err := writeError(request, "INVALID_CANCEL", err.Error()); err != nil {
					return err
				}
				continue
			}
			activeMu.Lock()
			cancel, ok := active[payload.RequestID]
			activeMu.Unlock()
			if ok && cancel != nil {
				cancel()
			}
			if err := writePayload(request, CancelResult{Canceled: ok}); err != nil {
				return err
			}
			continue
		}

		requestCtx, requestCancel := context.WithCancel(serveCtx)
		activeMu.Lock()
		active[request.RequestID] = requestCancel
		activeMu.Unlock()

		wg.Add(1)
		go func(request Envelope, requestCtx context.Context, requestCancel context.CancelFunc) {
			defer wg.Done()
			defer requestCancel()
			defer func() {
				activeMu.Lock()
				delete(active, request.RequestID)
				activeMu.Unlock()
			}()

			opMu.Lock()
			defer opMu.Unlock()

			var (
				payload     interface{}
				protocolErr *ProtocolError
			)
			if requestCtx.Err() != nil {
				protocolErr = &ProtocolError{Code: "CANCELED", Message: "database operation canceled"}
			} else {
				payload, protocolErr = handler.Handle(requestCtx, request)
				if requestCtx.Err() != nil {
					protocolErr = &ProtocolError{Code: "CANCELED", Message: "database operation canceled"}
					payload = nil
				}
			}

			var response Envelope
			var err error
			if protocolErr != nil {
				response, err = NewErrorResponse(request, protocolErr.Code, protocolErr.Message)
			} else {
				response, err = NewResponse(request, payload)
			}
			if err != nil {
				setAsyncErr(err)
				return
			}
			if err := writeResponse(response); err != nil {
				setAsyncErr(err)
			}
		}(request, requestCtx, requestCancel)
	}
	if err := scanner.Err(); err != nil {
		cancelServe()
		activeMu.Lock()
		for _, cancel := range active {
			cancel()
		}
		activeMu.Unlock()
		wg.Wait()
		return fmt.Errorf("read database adapter request: %w", err)
	}

	wg.Wait()
	errMu.Lock()
	err := asyncErr
	errMu.Unlock()
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func encodeEnvelope(encoder *json.Encoder, env Envelope) error {
	if err := ValidateEnvelope(env); err != nil {
		return err
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encode database adapter response: %w", err)
	}
	if len(data)+1 > MaxMessageBytes {
		return fmt.Errorf("database adapter response exceeds %d bytes", MaxMessageBytes)
	}
	if err := encoder.Encode(env); err != nil {
		return fmt.Errorf("write database adapter response: %w", err)
	}
	return nil
}
