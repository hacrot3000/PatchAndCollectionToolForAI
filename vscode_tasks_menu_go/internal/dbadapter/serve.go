package dbadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), MaxMessageBytes)
	encoder := json.NewEncoder(output)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
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
			response, err := NewErrorResponse(request, "UNSUPPORTED_OPERATION", fmt.Sprintf("operation %q is not supported", request.Operation))
			if err != nil {
				return err
			}
			if err := encodeEnvelope(encoder, response); err != nil {
				return err
			}
			continue
		}

		var payload interface{}
		var protocolErr *ProtocolError
		switch request.Operation {
		case OpHello:
			payload = map[string]interface{}{
				"adapter_id":       manifest.ID,
				"adapter_name":     manifest.Name,
				"adapter_kind":     manifest.Kind,
				"protocol_version": manifest.ProtocolVersion,
			}
		case OpCapabilities:
			payload = manifest.Capabilities
		default:
			payload, protocolErr = handler.Handle(ctx, request)
		}

		var response Envelope
		var err error
		if protocolErr != nil {
			response, err = NewErrorResponse(request, protocolErr.Code, protocolErr.Message)
		} else {
			response, err = NewResponse(request, payload)
		}
		if err != nil {
			return err
		}
		if err := encodeEnvelope(encoder, response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read database adapter request: %w", err)
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
