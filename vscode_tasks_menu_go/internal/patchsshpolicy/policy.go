// Package patchsshpolicy bounds commands sent by AI-generated PATCH packages.
// Remote SSH always invokes a shell; keep every accepted token in a literal,
// metacharacter-free ASCII grammar and never permit script interpreters.
package patchsshpolicy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var safeToken = regexp.MustCompile("^[A-Za-z0-9_./-]{1,160}$")

// RelativePath rejects escape, absolute paths and option-like operands.
func RelativePath(value string) bool {
	if !safeToken.MatchString(value) || strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "-") || strings.Contains(value, "//") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || strings.HasPrefix(part, "-") {
			return false
		}
	}
	return true
}

// Validate returns a single remote-shell command only if every argv token
// belongs to the read-only whitelist. No shell strings, Python or scripts.
func Validate(argv []string) (string, error) {
	reject := func(reason string) (string, error) {
		return "", fmt.Errorf("remote SSH command forbidden: %s", reason)
	}
	if len(argv) == 0 || len(argv) > 8 {
		return reject("expected 1-8 argv tokens")
	}
	for _, arg := range argv {
		if !safeToken.MatchString(arg) {
			return reject("unsafe argument characters")
		}
	}
	switch argv[0] {
	case "pwd":
		if len(argv) != 1 {
			return reject("pwd accepts no arguments")
		}
	case "ls":
		pathSeen := false
		for _, token := range argv[1:] {
			if strings.HasPrefix(token, "-") {
				if pathSeen || len(token) < 2 {
					return reject("ls options must precede path")
				}
				for _, ch := range token[1:] {
					if !strings.ContainsRune("alh1", ch) {
						return reject("ls option not in read-only whitelist")
					}
				}
			} else {
				if pathSeen || !RelativePath(token) {
					return reject("ls supports one relative path")
				}
				pathSeen = true
			}
		}
	case "tree":
		pathSeen := false
		levelSeen := false
		for i := 1; i < len(argv); i++ {
			token := argv[i]
			switch token {
			case "-a", "-d":
				if pathSeen {
					return reject("tree options must precede path")
				}
			case "-L":
				if levelSeen || pathSeen || i+1 >= len(argv) {
					return reject("tree -L requires one depth")
				}
				i++
				if argv[i] != "1" && argv[i] != "2" && argv[i] != "3" {
					return reject("tree depth must be 1-3")
				}
				levelSeen = true
			default:
				if pathSeen || !RelativePath(token) {
					return reject("tree supports only read-only options and one relative path")
				}
				pathSeen = true
			}
		}
	case "find":
		i := 1
		if i < len(argv) && !strings.HasPrefix(argv[i], "-") {
			if !RelativePath(argv[i]) {
				return reject("find path must be relative")
			}
			i++
		}
		if i+1 >= len(argv) || argv[i] != "-maxdepth" {
			return reject("find requires bounded -maxdepth")
		}
		depth, err := strconv.Atoi(argv[i+1])
		if err != nil || depth < 1 || depth > 3 {
			return reject("find depth must be 1-3")
		}
		i += 2
		if i < len(argv) {
			if len(argv) != i+2 || argv[i] != "-type" || (argv[i+1] != "f" && argv[i+1] != "d") {
				return reject("find supports only optional -type f/d")
			}
		}
	default:
		return reject("only pwd, ls, tree and bounded find; no scripts or interpreters")
	}
	if len(strings.Join(argv, " ")) > 512 {
		return reject("command too long")
	}
	return strings.Join(argv, " "), nil
}
