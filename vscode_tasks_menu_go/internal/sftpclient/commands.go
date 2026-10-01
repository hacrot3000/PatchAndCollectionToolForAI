package sftpclient

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"
)

const (
	MaxRemotePathBytes = 4096
	MaxListEntries      = 5000
)

type Entry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

func QuotePath(value string) (string, error) {
	if err := validatePath(value); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\', '"', '*', '?', '[', ']':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

func validatePath(value string) error {
	if value == "" {
		return errors.New("sftp path is required")
	}
	if len(value) > MaxRemotePathBytes {
		return fmt.Errorf("sftp path exceeds %d bytes", MaxRemotePathBytes)
	}
	for _, r := range value {
		if r == 0 || r == '\r' || r == '\n' || unicode.IsControl(r) {
			return errors.New("sftp path contains control characters")
		}
	}
	return nil
}

func PwdCommand() string {
	return "pwd\n"
}

func ListCommand(remotePath string) (string, error) {
	path, err := QuotePath(remotePath)
	if err != nil {
		return "", err
	}
	// -l gives details, -a includes dotfiles, -n makes uid/gid numeric.
	return "ls -lan " + path + "\n", nil
}

func GetCommand(remotePath, localPath string) (string, error) {
	remote, err := QuotePath(remotePath)
	if err != nil {
		return "", err
	}
	local, err := QuotePath(localPath)
	if err != nil {
		return "", err
	}
	return "get " + remote + " " + local + "\n", nil
}

func PutCommand(localPath, remotePath string) (string, error) {
	local, err := QuotePath(localPath)
	if err != nil {
		return "", err
	}
	remote, err := QuotePath(remotePath)
	if err != nil {
		return "", err
	}
	return "put " + local + " " + remote + "\n", nil
}

func MkdirCommand(remotePath string) (string, error) {
	path, err := QuotePath(remotePath)
	if err != nil {
		return "", err
	}
	return "mkdir " + path + "\n", nil
}

func RenameCommand(oldPath, newPath string) (string, error) {
	oldValue, err := QuotePath(oldPath)
	if err != nil {
		return "", err
	}
	newValue, err := QuotePath(newPath)
	if err != nil {
		return "", err
	}
	return "rename " + oldValue + " " + newValue + "\n", nil
}

func RemoveCommand(remotePath string, directory bool) (string, error) {
	path, err := QuotePath(remotePath)
	if err != nil {
		return "", err
	}
	command := "rm "
	if directory {
		command = "rmdir "
	}
	return command + path + "\n", nil
}

func ParseLongList(output string) ([]Entry, error) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	entries := make([]Entry, 0)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "sftp>") || strings.HasPrefix(line, "Connected to ") {
			continue
		}
		if len(line) < 10 || !strings.ContainsRune("-dlcbps", rune(line[0])) {
			continue
		}
		fields, rest := splitFieldsAndRest(line, 8)
		if len(fields) != 8 || strings.TrimSpace(rest) == "" {
			continue
		}
		size, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("parse sftp list size %q", fields[4])
		}
		name := strings.TrimSpace(rest)
		entryType := "file"
		switch fields[0][0] {
		case 'd':
			entryType = "directory"
		case 'l':
			entryType = "symlink"
			if idx := strings.LastIndex(name, " -> "); idx > 0 {
				name = name[:idx]
			}
		default:
			if fields[0][0] != '-' {
				entryType = "other"
			}
		}
		// OpenSSH sftp may prefix names with the listed path, especially when
		// listing "." (for example "./.", "./..", "./file"). A directory
		// entry name itself cannot contain '/', so expose only the basename to
		// the UI and then filter the synthetic dot entries.
		name = path.Base(strings.TrimSpace(name))
		if name == "." || name == ".." || name == "/" || name == "" {
			continue
		}
		entries = append(entries, Entry{
			Name: name,
			Type: entryType,
			Size: size,
			Modified: normalizeListTime(fields[5], fields[6], fields[7]),
		})
		if len(entries) > MaxListEntries {
			return nil, fmt.Errorf("sftp listing exceeds %d entries", MaxListEntries)
		}
	}
	return entries, nil
}

func splitFieldsAndRest(value string, count int) ([]string, string) {
	fields := make([]string, 0, count)
	i := 0
	for len(fields) < count {
		for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
			i++
		}
		if i >= len(value) {
			return fields, ""
		}
		start := i
		for i < len(value) && value[i] != ' ' && value[i] != '\t' {
			i++
		}
		fields = append(fields, value[start:i])
	}
	for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
		i++
	}
	return fields, value[i:]
}

func normalizeListTime(month, day, clockOrYear string) string {
	// OpenSSH long listings are display data, not a protocol timestamp.
	// Keep the source fields instead of guessing a timezone.
	return strings.Join([]string{month, day, clockOrYear}, " ")
}
