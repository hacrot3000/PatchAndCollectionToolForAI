package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxPageTitleRunes = 200

// ReadPageTitle reads [ui].page_title from the workspace-local
// vscode_tasks_menu.ini. An absent file/key means the UI should use its default
// title.
func ReadPageTitle(workspace string) (string, error) {
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("đọc %s: %w", path, err)
	}
	if len(data) > 1<<20 {
		return "", fmt.Errorf("%s quá lớn", path)
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
		if section != "ui" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "page_title") {
			continue
		}
		title, err := normalizePageTitle(value)
		if err != nil {
			return "", fmt.Errorf("ui.page_title không hợp lệ: %w", err)
		}
		return title, nil
	}
	return "", nil
}

// SetPageTitle updates only [ui].page_title and preserves the other INI
// sections, including authentication settings. The rewrite is atomic and keeps
// the local config private because it may contain credentials.
func SetPageTitle(workspace, value string) error {
	title, err := normalizePageTitle(value)
	if err != nil {
		return err
	}
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("đọc %s: %w", path, err)
	}

	text := strings.ReplaceAll(string(data), "\r", "")
	lines := strings.Split(text, "\n")
	// Drop only the synthetic final empty element; a newline is restored below.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	uiStart, uiEnd, keyIndex := -1, len(lines), -1
	section := ""
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			newSection := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "ui" && uiEnd == len(lines) {
				uiEnd = i
			}
			section = newSection
			if section == "ui" && uiStart == -1 {
				uiStart = i
			}
			continue
		}
		if section != "ui" || keyIndex != -1 {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "page_title") {
			keyIndex = i
		}
	}

	setting := "page_title = " + title
	switch {
	case keyIndex >= 0:
		lines[keyIndex] = setting
	case uiStart >= 0:
		insertAt := uiEnd
		lines = append(lines, "")
		copy(lines[insertAt+1:], lines[insertAt:])
		lines[insertAt] = setting
	default:
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[ui]", setting)
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
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("cập nhật %s: %w", path, err)
	}
	return nil
}

func normalizePageTitle(value string) (string, error) {
	title := strings.TrimSpace(value)
	if strings.ContainsAny(title, "\r\n\x00") {
		return "", fmt.Errorf("page title không được chứa xuống dòng hoặc NUL")
	}
	if len([]rune(title)) > maxPageTitleRunes {
		return "", fmt.Errorf("page title tối đa %d ký tự", maxPageTitleRunes)
	}
	return title, nil
}
