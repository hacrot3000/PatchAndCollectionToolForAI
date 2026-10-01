package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const DefaultSelfUpdateBranch = "main"

type SelfUpdateSettings struct {
	Branch string `json:"branch"`
	RunFullValidationTests bool `json:"run_full_validation_tests"`
}

func DefaultSelfUpdateSettings() SelfUpdateSettings {
	return SelfUpdateSettings{Branch: DefaultSelfUpdateBranch, RunFullValidationTests: false}
}

func NormalizeSelfUpdateBranch(value string) (string, error) {
	branch := strings.TrimSpace(value)
	if branch == "" {
		branch = DefaultSelfUpdateBranch
	}
	if len(branch) > 200 {
		return "", fmt.Errorf("self_update.branch quá dài")
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") ||
		strings.Contains(branch, "//") || strings.Contains(branch, "..") ||
		strings.Contains(branch, "@{") || strings.Contains(branch, "\\") ||
		strings.HasSuffix(branch, ".") {
		return "", fmt.Errorf("self_update.branch không hợp lệ: %q", branch)
	}
	for _, ch := range branch {
		switch {
		case ch >= 'a' && ch <= 'z',
			ch >= 'A' && ch <= 'Z',
			ch >= '0' && ch <= '9',
			ch == '-', ch == '_', ch == '.', ch == '/':
		default:
			return "", fmt.Errorf("self_update.branch chứa ký tự không hợp lệ: %q", branch)
		}
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(strings.ToLower(part), ".lock") {
			return "", fmt.Errorf("self_update.branch không hợp lệ: %q", branch)
		}
	}
	return branch, nil
}

func ReadSelfUpdateSettings(workspace string) (SelfUpdateSettings, error) {
	cfg, _, err := Load(workspace)
	if err != nil {
		return SelfUpdateSettings{}, err
	}
	branch, err := NormalizeSelfUpdateBranch(cfg.SelfUpdateBranch)
	if err != nil {
		return SelfUpdateSettings{}, err
	}
	return SelfUpdateSettings{Branch: branch, RunFullValidationTests: cfg.SelfUpdateFullValidation}, nil
}

func SetSelfUpdateSettings(workspace string, value SelfUpdateSettings) error {
	branch, err := NormalizeSelfUpdateBranch(value.Branch)
	if err != nil {
		return err
	}
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

	branchLine := "branch = " + branch
	validationLine := "run_full_validation_tests = false"
	if value.RunFullValidationTests { validationLine = "run_full_validation_tests = true" }
	if start < 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines,
			"[self_update]",
			"# Branch dùng để check/download source khi self-update.",
			branchLine,
			"# Full developer test suite before activation. Mặc định tắt cho end users.",
			validationLine,
		)
	} else {
		body := make([]string, 0, end-start)
		for _, raw := range lines[start+1 : end] {
			line := strings.TrimSpace(raw)
			key, _, ok := strings.Cut(line, "=")
			if ok {
				name := strings.ToLower(strings.TrimSpace(key))
				if name == "branch" || name == "run_full_validation_tests" { continue }
			}
			body = append(body, raw)
		}
		rebuilt := make([]string, 0, len(lines)+1)
		rebuilt = append(rebuilt, lines[:start+1]...)
		rebuilt = append(rebuilt, body...)
		if len(body) > 0 && strings.TrimSpace(body[len(body)-1]) != "" {
			rebuilt = append(rebuilt, "")
		}
		rebuilt = append(rebuilt, branchLine, validationLine)
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
