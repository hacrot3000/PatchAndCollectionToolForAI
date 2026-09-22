package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxTerminalCustomDirs = 64
	maxTerminalCWDLen      = 2048
)

type TerminalCWDSettings struct {
	SelectedCWD string   `json:"selected_cwd"`
	CustomDirs  []string `json:"custom_dirs"`
}

func DefaultTerminalCWDSettings() TerminalCWDSettings {
	return TerminalCWDSettings{SelectedCWD: ".", CustomDirs: []string{}}
}

func normalizeTerminalCWD(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." {
		return ".", nil
	}
	if len(value) > maxTerminalCWDLen || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("terminal cwd không hợp lệ")
	}
	if strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("terminal cwd phải là đường dẫn tương đối")
	}
	clean := path.Clean(strings.TrimPrefix(value, "./"))
	if clean == "." {
		return ".", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("terminal cwd phải nằm trong workspace")
	}
	return clean, nil
}

func NormalizeTerminalCWDSettings(value TerminalCWDSettings) (TerminalCWDSettings, error) {
	selected, err := normalizeTerminalCWD(value.SelectedCWD)
	if err != nil {
		return TerminalCWDSettings{}, err
	}
	out := DefaultTerminalCWDSettings()
	out.SelectedCWD = selected
	seen := map[string]bool{}
	for _, raw := range value.CustomDirs {
		dir, err := normalizeTerminalCWD(raw)
		if err != nil {
			return TerminalCWDSettings{}, err
		}
		if dir == "." || seen[dir] {
			continue
		}
		if len(out.CustomDirs) >= maxTerminalCustomDirs {
			return TerminalCWDSettings{}, fmt.Errorf("terminal custom dirs vượt quá %d mục", maxTerminalCustomDirs)
		}
		seen[dir] = true
		out.CustomDirs = append(out.CustomDirs, dir)
	}
	if selected != "." && !seen[selected] {
		if len(out.CustomDirs) >= maxTerminalCustomDirs {
			return TerminalCWDSettings{}, fmt.Errorf("terminal custom dirs vượt quá %d mục", maxTerminalCustomDirs)
		}
		out.CustomDirs = append(out.CustomDirs, selected)
	}
	return out, nil
}

func ReadTerminalCWDSettings(workspace string) (TerminalCWDSettings, error) {
	cfg := DefaultTerminalCWDSettings()
	configPath := filepath.Join(workspace, "vscode_tasks_menu.ini")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return TerminalCWDSettings{}, fmt.Errorf("đọc %s: %w", configPath, err)
	}
	if len(data) > 1<<20 {
		return TerminalCWDSettings{}, fmt.Errorf("%s quá lớn", configPath)
	}

	section := ""
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "terminal" {
			continue
		}
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		rawValue = strings.TrimSpace(rawValue)
		switch key {
		case "selected_cwd":
			cfg.SelectedCWD = rawValue
		case "custom_dirs":
			if rawValue == "" {
				cfg.CustomDirs = []string{}
				continue
			}
			if err := json.Unmarshal([]byte(rawValue), &cfg.CustomDirs); err != nil {
				return TerminalCWDSettings{}, fmt.Errorf("terminal.custom_dirs không hợp lệ: %w", err)
			}
		}
	}
	return NormalizeTerminalCWDSettings(cfg)
}

func SetTerminalCWDSettings(workspace string, value TerminalCWDSettings) error {
	settings, err := NormalizeTerminalCWDSettings(value)
	if err != nil {
		return err
	}
	configPath := filepath.Join(workspace, "vscode_tasks_menu.ini")
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("đọc %s: %w", configPath, err)
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start, end := -1, len(lines)
	section := ""
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		next := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
		if section == "terminal" && end == len(lines) {
			end = i
		}
		section = next
		if section == "terminal" && start == -1 {
			start = i
		}
	}
	if start >= 0 && end == len(lines) {
		end = len(lines)
	}

	customJSON, err := json.Marshal(settings.CustomDirs)
	if err != nil {
		return err
	}
	settingsLines := []string{
		"selected_cwd = " + settings.SelectedCWD,
		"custom_dirs = " + string(customJSON),
	}

	if start < 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[terminal]")
		lines = append(lines, settingsLines...)
	} else {
		body := make([]string, 0, end-start-1)
		for _, raw := range lines[start+1 : end] {
			line := strings.TrimSpace(raw)
			key, _, ok := strings.Cut(line, "=")
			if ok {
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "selected_cwd", "custom_dirs":
					continue
				}
			}
			body = append(body, raw)
		}
		rebuilt := make([]string, 0, len(lines)+2)
		rebuilt = append(rebuilt, lines[:start+1]...)
		rebuilt = append(rebuilt, body...)
		if len(body) > 0 && strings.TrimSpace(body[len(body)-1]) != "" {
			rebuilt = append(rebuilt, "")
		}
		rebuilt = append(rebuilt, settingsLines...)
		rebuilt = append(rebuilt, lines[end:]...)
		lines = rebuilt
	}

	content := strings.Join(lines, "\n") + "\n"
	tmp, err := os.CreateTemp(workspace, ".vscode_tasks_menu.ini.*")
	if err != nil {
		return fmt.Errorf("tạo file config tạm: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ghi file config tạm: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("cập nhật %s: %w", configPath, err)
	}
	return os.Chmod(configPath, 0o600)
}
