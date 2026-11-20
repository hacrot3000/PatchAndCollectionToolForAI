package dbmysql

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type credentialFile struct {
	dir  string
	path string
}

func createCredentialFile(secret string) (*credentialFile, error) {
	if secret == "" {
		return nil, nil
	}
	if strings.ContainsAny(secret, "\x00\r\n") {
		return nil, errors.New("MySQL password must not contain NUL or line breaks")
	}

	dir, err := os.MkdirTemp("", "taskdeck-mysql-*")
	if err != nil {
		return nil, fmt.Errorf("create MySQL credential directory: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		cleanup()
		return nil, fmt.Errorf("protect MySQL credential directory: %w", err)
	}

	path := filepath.Join(dir, "client.cnf")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create MySQL credential file: %w", err)
	}
	content := "[client]\npassword=\"" + escapeOptionFileValue(secret) + "\"\n"
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("write MySQL credential file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("sync MySQL credential file: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("close MySQL credential file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		cleanup()
		return nil, fmt.Errorf("protect MySQL credential file: %w", err)
	}
	return &credentialFile{dir: dir, path: path}, nil
}

func (f *credentialFile) Arg() string {
	if f == nil {
		return ""
	}
	return "--defaults-extra-file=" + f.path
}

func (f *credentialFile) Close() {
	if f == nil || f.dir == "" {
		return
	}
	_ = os.RemoveAll(f.dir)
	f.dir = ""
	f.path = ""
}

func escapeOptionFileValue(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) + 8)
	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString("\\\\")
		case '"':
			builder.WriteString("\\\"")
		case '\t':
			builder.WriteString("\\t")
		case '\b':
			builder.WriteString("\\b")
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
