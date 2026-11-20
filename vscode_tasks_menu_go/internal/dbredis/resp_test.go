package dbredis

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWriteCommandEncodesRESP2Array(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteCommand(&buffer, []string{"GET", "user:1"}); err != nil {
		t.Fatal(err)
	}
	want := "*2\r\n$3\r\nGET\r\n$6\r\nuser:1\r\n"
	if buffer.String() != want {
		t.Fatalf("encoded command=%q want=%q", buffer.String(), want)
	}
}

func TestReadValueParsesRESP2Types(t *testing.T) {
	tests := []struct {
		input string
		kind  Kind
		text  string
		int64 int64
	}{
		{"+OK\r\n", KindSimpleString, "OK", 0},
		{"-ERR denied\r\n", KindError, "ERR denied", 0},
		{":42\r\n", KindInteger, "", 42},
		{"$5\r\nhello\r\n", KindBulkString, "hello", 0},
		{"$-1\r\n", KindNull, "", 0},
	}
	for _, tc := range tests {
		value, err := ReadValue(bufio.NewReader(strings.NewReader(tc.input)))
		if err != nil {
			t.Fatalf("input=%q error=%v", tc.input, err)
		}
		if value.Kind != tc.kind || value.Text != tc.text || value.Integer != tc.int64 {
			t.Fatalf("input=%q value=%+v", tc.input, value)
		}
	}
}

func TestReadValueParsesNestedArray(t *testing.T) {
	value, err := ReadValue(bufio.NewReader(strings.NewReader(
		"*3\r\n$3\r\none\r\n:2\r\n*2\r\n+OK\r\n$-1\r\n",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if value.Kind != KindArray || len(value.Array) != 3 {
		t.Fatalf("value=%+v", value)
	}
	if value.Array[0].Text != "one" || value.Array[1].Integer != 2 {
		t.Fatalf("array=%+v", value.Array)
	}
	if value.Array[2].Kind != KindArray || value.Array[2].Array[1].Kind != KindNull {
		t.Fatalf("nested array=%+v", value.Array[2])
	}
}

func TestReadValueRejectsInvalidLengthsAndTerminators(t *testing.T) {
	for _, input := range []string{
		"$-2\r\n",
		"*10001\r\n",
		"$3\r\nabcXX",
		":not-int\r\n",
		"?wat\r\n",
	} {
		if _, err := ReadValue(bufio.NewReader(strings.NewReader(input))); err == nil {
			t.Fatalf("invalid RESP input unexpectedly accepted: %q", input)
		}
	}
}

func TestWriteCommandRejectsOversizedArgument(t *testing.T) {
	err := WriteCommand(&bytes.Buffer{}, []string{"SET", strings.Repeat("x", maxRESPBulkBytes+1)})
	if err == nil || !strings.Contains(err.Error(), "argument exceeds") {
		t.Fatalf("error=%v", err)
	}
}

func TestRedisErrorType(t *testing.T) {
	err := &RedisError{Message: "NOAUTH authentication required"}
	if !strings.Contains(err.Error(), "NOAUTH") {
		t.Fatalf("error=%v", err)
	}
	if errors.Is(err, errors.New("other")) {
		t.Fatal("RedisError unexpectedly matches unrelated error")
	}
}
