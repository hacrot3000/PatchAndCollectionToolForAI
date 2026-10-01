package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

type SelfUpdateSettings struct {
	RunFullValidationTests bool `json:"run_full_validation_tests"`
}

func DefaultSelfUpdateSettings() SelfUpdateSettings {
	return SelfUpdateSettings{RunFullValidationTests: false}
}

func ReadSelfUpdateSettings(workspace string) (SelfUpdateSettings, error) {
	settings := DefaultSelfUpdateSettings()
	configPath, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return SelfUpdateSettings{}, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return SelfUpdateSettings{}, fmt.Errorf("đọc %s: %w", configPath, err)
	}
	if len(data) > 1<<20 {
		return SelfUpdateSettings{}, fmt.Errorf("%s quá lớn", configPath)
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
		if section != "self_update" {
			continue
		}
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "run_full_validation_tests") {
			settings.RunFullValidationTests = parseBool(strings.TrimSpace(rawValue), false)
		}
	}
	return settings, nil
}

func SetSelfUpdateSettings(workspace string, settings SelfUpdateSettings) error {
	configPath, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return err
	}
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
		if section == "self_update" && end == len(lines) {
			end = i
		}
		section = next
		if section == "self_update" && start == -1 {
			start = i
		}
	}
	if start >= 0 && end == len(lines) {
		end = len(lines)
	}

	valueLine := "run_full_validation_tests = false"
	if settings.RunFullValidationTests {
		valueLine = "run_full_validation_tests = true"
	}

	if start < 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines,
			"[self_update]",
			"# Full developer test suite before activation. Mặc định tắt cho end users.",
			"# Build + binary/release validation vẫn luôn chạy dù setting này tắt.",
			valueLine,
		)
	} else {
		body := make([]string, 0, end-start-1)
		for _, raw := range lines[start+1 : end] {
			line := strings.TrimSpace(raw)
			key, _, ok := strings.Cut(line, "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "run_full_validation_tests") {
				continue
			}
			body = append(body, raw)
		}
		rebuilt := make([]string, 0, len(lines)+1)
		rebuilt = append(rebuilt, lines[:start+1]...)
		rebuilt = append(rebuilt, body...)
		if len(body) > 0 && strings.TrimSpace(body[len(body)-1]) != "" {
			rebuilt = append(rebuilt, "")
		}
		rebuilt = append(rebuilt, valueLine)
		rebuilt = append(rebuilt, lines[end:]...)
		lines = rebuilt
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("tạo thư mục config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".vscode_tasks_menu.ini.*")
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
