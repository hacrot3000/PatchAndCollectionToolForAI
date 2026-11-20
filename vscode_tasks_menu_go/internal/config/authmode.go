package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

// WriteAuthenticationMode atomically updates only the authentication/listener
// keys needed when switching between legacy single-user Basic Auth and
// multi-user shared-server mode. Unknown keys/comments and unrelated sections
// are preserved verbatim.
func WriteAuthenticationMode(workspace string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"server.protocol":          cfg.Protocol,
		"auth.username":            cfg.Username,
		"auth.password":            cfg.Password,
		"shared_server.project_id": cfg.SharedProjectID,
		"shared_server.identity_db": cfg.SharedIdentityDB,
	} {
		if err := validateINIValue(label, value); err != nil {
			return err
		}
	}

	path, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	lines := splitINIForRewrite(string(data))
	updates := []struct {
		section string
		key     string
		value   string
	}{
		{"server", "protocol", strings.ToLower(strings.TrimSpace(cfg.Protocol))},
		{"server", "port", strconv.Itoa(cfg.Port)},
		{"auth", "enabled", strconv.FormatBool(cfg.AuthEnabled)},
		{"auth", "username", cfg.Username},
		{"auth", "password", cfg.Password},
		{"shared_server", "enabled", strconv.FormatBool(cfg.SharedServerEnabled)},
		{"shared_server", "project_id", cfg.SharedProjectID},
		{"shared_server", "identity_db", cfg.SharedIdentityDB},
	}
	for _, update := range updates {
		lines = setINIValue(lines, update.section, update.key, update.value)
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vscode_tasks_menu.ini.auth-mode.*")
	if err != nil {
		return fmt.Errorf("create auth-mode config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write auth-mode config temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}

func validateINIValue(label, value string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s contains an invalid newline or NUL", label)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s cannot start or end with whitespace", label)
	}
	return nil
}

func splitINIForRewrite(text string) []string {
	text = strings.ReplaceAll(text, "\r", "")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func setINIValue(lines []string, wantedSection, wantedKey, value string) []string {
	section := ""
	sectionStart, sectionEnd, keyIndex := -1, len(lines), -1
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			next := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == wantedSection && sectionEnd == len(lines) {
				sectionEnd = i
			}
			section = next
			if section == wantedSection && sectionStart < 0 {
				sectionStart = i
			}
			continue
		}
		if section != wantedSection || keyIndex >= 0 {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), wantedKey) {
			keyIndex = i
		}
	}
	setting := wantedKey + " = " + value
	switch {
	case keyIndex >= 0:
		lines[keyIndex] = setting
	case sectionStart >= 0:
		insertAt := sectionEnd
		for insertAt > sectionStart+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		lines = append(lines, "")
		copy(lines[insertAt+1:], lines[insertAt:])
		lines[insertAt] = setting
	default:
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "["+wantedSection+"]", setting)
	}
	return lines
}
