package dbredis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	maxRESPLineBytes  = 64 << 10
	maxRESPBulkBytes  = 8 << 20
	maxRESPArrayItems = 10000
	maxRESPDepth      = 16
)

type Kind byte

const (
	KindSimpleString Kind = '+'
	KindError        Kind = '-'
	KindInteger      Kind = ':'
	KindBulkString   Kind = '$'
	KindArray        Kind = '*'
	KindNull         Kind = 0
)

type Value struct {
	Kind    Kind
	Text    string
	Integer int64
	Array   []Value
}

type RedisError struct {
	Message string
}

func (e *RedisError) Error() string {
	if e == nil {
		return "Redis error"
	}
	return "Redis error: " + e.Message
}

func WriteCommand(w io.Writer, args []string) error {
	if w == nil {
		return errors.New("Redis command writer is required")
	}
	if len(args) == 0 || len(args) > 1024 {
		return errors.New("Redis command must contain 1..1024 arguments")
	}
	var builder strings.Builder
	builder.WriteByte('*')
	builder.WriteString(strconv.Itoa(len(args)))
	builder.WriteString("\r\n")
	for _, arg := range args {
		if len(arg) > maxRESPBulkBytes {
			return fmt.Errorf("Redis command argument exceeds %d bytes", maxRESPBulkBytes)
		}
		builder.WriteByte('$')
		builder.WriteString(strconv.Itoa(len(arg)))
		builder.WriteString("\r\n")
		builder.WriteString(arg)
		builder.WriteString("\r\n")
		if builder.Len() > maxRESPBulkBytes {
			return fmt.Errorf("Redis command exceeds %d bytes", maxRESPBulkBytes)
		}
	}
	_, err := io.WriteString(w, builder.String())
	if err != nil {
		return fmt.Errorf("write Redis command: %w", err)
	}
	return nil
}

func ReadValue(reader *bufio.Reader) (Value, error) {
	if reader == nil {
		return Value{}, errors.New("Redis response reader is required")
	}
	return readValue(reader, 0)
}

func readValue(reader *bufio.Reader, depth int) (Value, error) {
	if depth > maxRESPDepth {
		return Value{}, fmt.Errorf("Redis response exceeds nesting depth %d", maxRESPDepth)
	}
	prefix, err := reader.ReadByte()
	if err != nil {
		return Value{}, fmt.Errorf("read Redis response type: %w", err)
	}
	switch Kind(prefix) {
	case KindSimpleString:
		line, err := readRESPLine(reader)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindSimpleString, Text: line}, nil
	case KindError:
		line, err := readRESPLine(reader)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindError, Text: line}, nil
	case KindInteger:
		line, err := readRESPLine(reader)
		if err != nil {
			return Value{}, err
		}
		value, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return Value{}, fmt.Errorf("parse Redis integer %q: %w", line, err)
		}
		return Value{Kind: KindInteger, Integer: value}, nil
	case KindBulkString:
		line, err := readRESPLine(reader)
		if err != nil {
			return Value{}, err
		}
		length, err := strconv.Atoi(line)
		if err != nil {
			return Value{}, fmt.Errorf("parse Redis bulk length %q: %w", line, err)
		}
		if length == -1 {
			return Value{Kind: KindNull}, nil
		}
		if length < 0 || length > maxRESPBulkBytes {
			return Value{}, fmt.Errorf("Redis bulk length %d is invalid", length)
		}
		data := make([]byte, length+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return Value{}, fmt.Errorf("read Redis bulk string: %w", err)
		}
		if data[length] != '\r' || data[length+1] != '\n' {
			return Value{}, errors.New("Redis bulk string terminator is invalid")
		}
		return Value{Kind: KindBulkString, Text: string(data[:length])}, nil
	case KindArray:
		line, err := readRESPLine(reader)
		if err != nil {
			return Value{}, err
		}
		count, err := strconv.Atoi(line)
		if err != nil {
			return Value{}, fmt.Errorf("parse Redis array length %q: %w", line, err)
		}
		if count == -1 {
			return Value{Kind: KindNull}, nil
		}
		if count < 0 || count > maxRESPArrayItems {
			return Value{}, fmt.Errorf("Redis array length %d is invalid", count)
		}
		items := make([]Value, count)
		for i := range items {
			item, err := readValue(reader, depth+1)
			if err != nil {
				return Value{}, fmt.Errorf("Redis array item %d: %w", i+1, err)
			}
			items[i] = item
		}
		return Value{Kind: KindArray, Array: items}, nil
	default:
		return Value{}, fmt.Errorf("unsupported Redis response type byte 0x%02x", prefix)
	}
}

func readRESPLine(reader *bufio.Reader) (string, error) {
	var builder strings.Builder
	for {
		part, err := reader.ReadString('\n')
		if len(part) > 0 {
			builder.WriteString(part)
			if builder.Len() > maxRESPLineBytes {
				return "", fmt.Errorf("Redis response line exceeds %d bytes", maxRESPLineBytes)
			}
		}
		if err != nil {
			return "", fmt.Errorf("read Redis response line: %w", err)
		}
		if strings.HasSuffix(builder.String(), "\r\n") {
			text := builder.String()
			return text[:len(text)-2], nil
		}
	}
}
