// Package patchsshpolicy validates the single read-only command in an AI PATCH
// before TaskDeck sends it to a saved SSH profile. An SSH server uses a shell
// for a remote command, so every accepted token is safely shell-quoted here.
// Arbitrary shell commands, scripts and interpreters remain forbidden.
package patchsshpolicy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	safeShellToken = regexp.MustCompile("^[A-Za-z0-9_./-]+$")
	safeRelativePath = regexp.MustCompile("^[A-Za-z0-9_./ -]{1,160}$")
	safeFileGlob = regexp.MustCompile("^[A-Za-z0-9_./*? -]{1,128}$")
	safeSedRange = regexp.MustCompile("^([1-9][0-9]{0,5})(?:,([1-9][0-9]{0,5}))?p$")
)

// RelativePath restricts search operands to relative paths. Internal spaces
// are supported because they will be shell-quoted before sending over SSH.
func RelativePath(value string) bool {
	if !safeRelativePath.MatchString(value) || strings.TrimSpace(value) != value ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "~") || strings.Contains(value, "//") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || strings.HasPrefix(part, "-") {
			return false
		}
	}
	return true
}

func safePattern(value string) bool {
	if value == "" || len(value) > 512 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func safeGlob(value string) bool {
	return safeFileGlob.MatchString(value) && !strings.Contains(value, "..") &&
		!strings.HasPrefix(value, "-") && strings.TrimSpace(value) == value
}

// POSIX-shell quoting is needed because OpenSSH's remote command interface
// serializes an argv vector into a remote login-shell command string.
// Single quote escaping is the only place where raw pattern text is handled.
func quoteArg(value string) string {
	if safeShellToken.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shortFlagsOK(token, allowed string) bool {
	if len(token) < 2 || !strings.HasPrefix(token, "-") ||
		strings.HasPrefix(token, "--") {
		return false
	}
	for _, r := range token[1:] {
		if !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

func searchArgs(name string, args []string) error {
	allowed := "niFEwlcrqov"
	if name == "rg" {
		allowed = "niFwlcvq"
	}
	i := 0
	afterSeparator := false
	for i < len(args) {
		token := args[i]
		if token == "--" {
			afterSeparator = true
			i++
			break
		}
		if token == "-g" && name == "rg" {
			if i+1 >= len(args) || !safeGlob(args[i+1]) {
				return fmt.Errorf("rg -g requires a safe glob")
			}
			i += 2
			continue
		}
		if name == "grep" && (strings.HasPrefix(token, "--include=") || strings.HasPrefix(token, "--exclude=")) {
			if !safeGlob(strings.SplitN(token, "=", 2)[1]) {
				return fmt.Errorf("grep include/exclude requires a safe glob")
			}
			i++
			continue
		}
		if strings.HasPrefix(token, "-") {
			if !shortFlagsOK(token, allowed) {
				switch token {
				case "--color=never", "--line-number", "--ignore-case",
					"--fixed-strings", "--word-regexp", "--count",
					"--files-with-matches", "--no-messages":
					// Supported by both grep and rg.
				case "--hidden":
					if name != "rg" {
						return fmt.Errorf("hidden is an rg-only option")
					}
				default:
					return fmt.Errorf("search flag outside read-only allowlist: %s", token)
				}
			}
			i++
			continue
		}
		break
	}
	if i >= len(args) {
		return fmt.Errorf("search pattern is required")
	}
	if strings.HasPrefix(args[i], "-") && !afterSeparator {
		return fmt.Errorf("patterns starting with - require --")
	}
	if !safePattern(args[i]) {
		return fmt.Errorf("invalid search pattern")
	}
	i++
	if name == "grep" && i == len(args) {
		return fmt.Errorf("grep requires a file or directory (stdin is forbidden)")
	}
	if len(args)-i > 8 {
		return fmt.Errorf("too many search paths")
	}
	for _, path := range args[i:] {
		if !RelativePath(path) {
			return fmt.Errorf("search path must be relative")
		}
	}
	return nil
}

func findArgs(args []string) error {
	i := 0
	if i < len(args) && !strings.HasPrefix(args[i], "-") {
		if !RelativePath(args[i]) {
			return fmt.Errorf("find path must be relative")
		}
		i++
	}
	if i+1 >= len(args) || args[i] != "-maxdepth" {
		return fmt.Errorf("find requires -maxdepth 1..3")
	}
	maxDepth, err := strconv.Atoi(args[i+1])
	if err != nil || maxDepth < 1 || maxDepth > 3 {
		return fmt.Errorf("find maxdepth must be 1..3")
	}
	i += 2
	seenType, seenName, seenMinDepth := false, false, false
	for i < len(args) {
		switch args[i] {
		case "-type":
			if seenType || i+1 >= len(args) ||
				(args[i+1] != "f" && args[i+1] != "d") {
				return fmt.Errorf("find -type accepts only f or d, once")
			}
			seenType = true
			i += 2
		case "-name", "-iname":
			if seenName || i+1 >= len(args) || !safeGlob(args[i+1]) {
				return fmt.Errorf("find -name/-iname requires a safe file glob")
			}
			seenName = true
			i += 2
		case "-mindepth":
			if seenMinDepth || i+1 >= len(args) {
				return fmt.Errorf("find -mindepth must be specified once")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 || n > maxDepth {
				return fmt.Errorf("find mindepth cannot exceed maxdepth")
			}
			seenMinDepth = true
			i += 2
		case "-print", "-print0":
			i++
		default:
			// In particular, reject -exec, -execdir, -ok, -okdir, -delete,
			// -fprint, -fprintf and every other executing/writing action.
			return fmt.Errorf("find action outside read-only whitelist: %s", args[i])
		}
	}
	return nil
}

func readArgs(name string, args []string) error {
	switch name {
	case "cat":
		if len(args) < 1 || len(args) > 8 {
			return fmt.Errorf("cat requires 1-8 file paths")
		}
		for _, path := range args {
			if !RelativePath(path) {
				return fmt.Errorf("cat path must be relative")
			}
		}
	case "head", "tail":
		file := ""
		if len(args) == 1 {
			file = args[0]
		} else if len(args) == 3 && args[0] == "-n" {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 || n > 5000 {
				return fmt.Errorf("%s -n must be 1..5000", name)
			}
			file = args[2]
		} else {
			return fmt.Errorf("%s accepts only -n 1..5000 and one file", name)
		}
		if !RelativePath(file) {
			return fmt.Errorf("%s path must be relative", name)
		}
	case "wc":
		i := 0
		for i < len(args) {
			switch args[i] {
			case "-l", "-w", "-c", "-m":
				i++
			default:
				goto wcFiles
			}
		}
	wcFiles:
		if len(args)-i < 1 || len(args)-i > 8 {
			return fmt.Errorf("wc requires 1-8 file paths")
		}
		for _, file := range args[i:] {
			if !RelativePath(file) {
				return fmt.Errorf("wc path must be relative")
			}
		}
	case "nl":
		if len(args) == 2 && args[0] == "-ba" {
			args = args[1:]
		}
		if len(args) != 1 || !RelativePath(args[0]) {
			return fmt.Errorf("nl accepts only optional -ba and one file")
		}
	case "sed":
		if len(args) != 3 || args[0] != "-n" || !RelativePath(args[2]) {
			return fmt.Errorf("sed requires -n RANGEp file")
		}
		match := safeSedRange.FindStringSubmatch(args[1])
		if match == nil {
			return fmt.Errorf("sed accepts only numeric print ranges")
		}
		if match[2] != "" {
			start, _ := strconv.Atoi(match[1])
			end, _ := strconv.Atoi(match[2])
			if end < start {
				return fmt.Errorf("sed range end precedes start")
			}
		}
	}
	return nil
}

// Validate returns a *quoted* remote-shell command after validating every
// token against a read-only, argument-aware whitelist. The returned string
// must never include unsanitized user strings.
func Validate(argv []string) (string, error) {
	if len(argv) == 0 || len(argv) > 24 {
		return "", fmt.Errorf("remote SSH command forbidden: expected 1..24 argv tokens")
	}
	total := 0
	for _, token := range argv {
		total += len(token)
		if !utf8.ValidString(token) || len(token) > 512 {
			return "", fmt.Errorf("remote SSH command forbidden: invalid token encoding or length")
		}
		for _, r := range token {
			if unicode.IsControl(r) {
				return "", fmt.Errorf("remote SSH command forbidden: control characters")
			}
		}
	}
	if total > 4096 {
		return "", fmt.Errorf("remote SSH command forbidden: command too large")
	}
	var err error
	switch argv[0] {
	case "pwd":
		if len(argv) != 1 {
			err = fmt.Errorf("pwd accepts no arguments")
		}
	case "ls":
		pathSeen := false
		for _, token := range argv[1:] {
			if strings.HasPrefix(token, "-") {
				if pathSeen || !shortFlagsOK(token, "alh1") {
					err = fmt.Errorf("ls accepts only -a/-l/-h/-1 before the path")
					break
				}
			} else if pathSeen || !RelativePath(token) {
				err = fmt.Errorf("ls accepts one relative path")
				break
			} else {
				pathSeen = true
			}
		}
	case "tree":
		pathSeen, levelSeen := false, false
		for i := 1; i < len(argv); i++ {
			token := argv[i]
			switch token {
			case "-a", "-d":
				if pathSeen {
					err = fmt.Errorf("tree options must precede path")
				}
			case "-L":
				if levelSeen || pathSeen || i+1 >= len(argv) {
					err = fmt.Errorf("tree -L requires a depth")
					break
				}
				i++
				if argv[i] != "1" && argv[i] != "2" && argv[i] != "3" {
					err = fmt.Errorf("tree depth must be 1..3")
				}
				levelSeen = true
			default:
				if pathSeen || !RelativePath(token) {
					err = fmt.Errorf("tree accepts one relative path")
				} else {
					pathSeen = true
				}
			}
			if err != nil {
				break
			}
		}
	case "find":
		err = findArgs(argv[1:])
	case "grep", "rg":
		err = searchArgs(argv[0], argv[1:])
	case "cat", "head", "tail", "wc", "nl", "sed":
		err = readArgs(argv[0], argv[1:])
	default:
		err = fmt.Errorf("executable outside SSH read-only whitelist (no scripts/interpreters)")
	}
	if err != nil {
		return "", fmt.Errorf("remote SSH command forbidden: %w", err)
	}
	quoted := make([]string, len(argv))
	for i, token := range argv {
		quoted[i] = quoteArg(token)
	}
	out := strings.Join(quoted, " ")
	if len(out) > 8192 {
		return "", fmt.Errorf("remote SSH command forbidden: command too long")
	}
	return out, nil
}
