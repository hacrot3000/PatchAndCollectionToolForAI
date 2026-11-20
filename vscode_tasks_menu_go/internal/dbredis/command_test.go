package dbredis

import (
	"strings"
	"testing"
)

func TestParseSafeCommandSupportsQuotesAndEscapes(t *testing.T) {
	args, err := parseSafeCommand(`GET "user key"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "GET" || args[1] != "user key" {
		t.Fatalf("args=%#v", args)
	}

	args, err = parseSafeCommand(`HGET 'hash key' field\ name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[1] != "hash key" || args[2] != "field name" {
		t.Fatalf("args=%#v", args)
	}
}

func TestParseSafeCommandRejectsWritesAndUnsafeCommands(t *testing.T) {
	for _, text := range []string{
		"SET key value",
		"DEL key",
		"FLUSHALL",
		"CONFIG GET databases",
		"EVAL return 1 0",
		"MULTI",
	} {
		if _, err := parseSafeCommand(text); err == nil {
			t.Fatalf("unsafe command %q unexpectedly accepted", text)
		}
	}
}

func TestParseSafeCommandRejectsMalformedInput(t *testing.T) {
	for _, text := range []string{
		"",
		"GET 'unterminated",
		"GET key\\",
		strings.Repeat("x", maxRedisCommandTextBytes+1),
	} {
		if _, err := parseSafeCommand(text); err == nil {
			t.Fatalf("malformed command %q unexpectedly accepted", text)
		}
	}
}
