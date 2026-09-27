package dbredis

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const maxRedisCommandTextBytes = 512 << 10

var safeRedisCommands = map[string]bool{
	"PING":       true,
	"GET":        true,
	"MGET":       true,
	"EXISTS":     true,
	"TYPE":       true,
	"TTL":        true,
	"PTTL":       true,
	"STRLEN":     true,
	"HGET":       true,
	"HMGET":      true,
	"HGETALL":    true,
	"HEXISTS":    true,
	"HLEN":       true,
	"LRANGE":     true,
	"LLEN":       true,
	"SMEMBERS":   true,
	"SCARD":      true,
	"SISMEMBER":  true,
	"ZRANGE":     true,
	"ZCARD":      true,
	"ZSCORE":     true,
	"SCAN":       true,
	"HSCAN":      true,
	"SSCAN":      true,
	"ZSCAN":      true,
	"INFO":       true,
}

func parseSafeCommand(text string) ([]string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("Redis command is required")
	}
	if len(text) > maxRedisCommandTextBytes {
		return nil, fmt.Errorf("Redis command exceeds %d bytes", maxRedisCommandTextBytes)
	}

	var (
		args    []string
		current strings.Builder
		quote   rune
		escape  bool
		started bool
	)
	flush := func() {
		if !started {
			return
		}
		args = append(args, current.String())
		current.Reset()
		started = false
	}

	for _, r := range text {
		if escape {
			switch r {
			case 'n':
				current.WriteRune('\n')
			case 'r':
				current.WriteRune('\r')
			case 't':
				current.WriteRune('\t')
			default:
				current.WriteRune(r)
			}
			escape = false
			started = true
			continue
		}
		if r == '\\' {
			escape = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				started = true
				continue
			}
			current.WriteRune(r)
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		current.WriteRune(r)
		started = true
	}
	if escape {
		return nil, errors.New("Redis command ends with an incomplete escape")
	}
	if quote != 0 {
		return nil, errors.New("Redis command has an unterminated quote")
	}
	flush()
	if len(args) == 0 {
		return nil, errors.New("Redis command is required")
	}
	if len(args) > 1024 {
		return nil, errors.New("Redis command has too many arguments")
	}
	command := strings.ToUpper(strings.TrimSpace(args[0]))
	if !safeRedisCommands[command] {
		return nil, fmt.Errorf("Redis command %q is not allowed in the initial read-oriented adapter", command)
	}
	args[0] = command
	return args, nil
}
