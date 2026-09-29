package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	DefaultRunningIndicatorMode = "boxes"
	DefaultRunningIndicatorRPM  = 2.0
	minRunningIndicatorRPM      = 1.0
	maxRunningIndicatorRPM      = 120.0
)

type RunningIndicatorSettings struct {
	Mode string  `json:"mode"`
	RPM  float64 `json:"rpm"`
}

func DefaultRunningIndicatorSettings() RunningIndicatorSettings {
	return RunningIndicatorSettings{Mode: DefaultRunningIndicatorMode, RPM: DefaultRunningIndicatorRPM}
}

func NormalizeRunningIndicatorSettings(value RunningIndicatorSettings) (RunningIndicatorSettings, error) {
	mode := strings.ToLower(strings.TrimSpace(value.Mode))
	if mode == "" {
		mode = DefaultRunningIndicatorMode
	}
	switch mode {
	case "boxes", "spinner", "braille", "time":
	default:
		return RunningIndicatorSettings{}, fmt.Errorf("running indicator mode phải là boxes, spinner, braille hoặc time")
	}
	rpm := value.RPM
	if rpm == 0 {
		rpm = DefaultRunningIndicatorRPM
	}
	if rpm < minRunningIndicatorRPM || rpm > maxRunningIndicatorRPM {
		return RunningIndicatorSettings{}, fmt.Errorf("running indicator rpm phải nằm trong khoảng %.0f..%.0f", minRunningIndicatorRPM, maxRunningIndicatorRPM)
	}
	rpm = float64(int(rpm + 0.5))
	return RunningIndicatorSettings{Mode: mode, RPM: rpm}, nil
}

func ReadRunningIndicatorSettings(workspace string) (RunningIndicatorSettings, error) {
	cfg := DefaultRunningIndicatorSettings()
	configPath, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return RunningIndicatorSettings{}, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return RunningIndicatorSettings{}, fmt.Errorf("đọc %s: %w", configPath, err)
	}
	if len(data) > 1<<20 {
		return RunningIndicatorSettings{}, fmt.Errorf("%s quá lớn", configPath)
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
		if section != "appearance" {
			continue
		}
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "running_indicator":
			cfg.Mode = strings.TrimSpace(rawValue)
		case "running_indicator_rpm":
			rpm, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
			if err != nil {
				return RunningIndicatorSettings{}, fmt.Errorf("appearance.running_indicator_rpm không hợp lệ: %w", err)
			}
			cfg.RPM = rpm
		}
	}
	return NormalizeRunningIndicatorSettings(cfg)
}

func SetRunningIndicatorSettings(workspace string, value RunningIndicatorSettings) error {
	settings, err := NormalizeRunningIndicatorSettings(value)
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
		if section == "appearance" && end == len(lines) {
			end = i
		}
		section = next
		if section == "appearance" && start == -1 {
			start = i
		}
	}
	if start >= 0 && end == len(lines) {
		end = len(lines)
	}

	settingsLines := []string{
		"running_indicator = " + settings.Mode,
		"running_indicator_rpm = " + strconv.FormatFloat(settings.RPM, 'f', -1, 64),
	}

	if start < 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[appearance]")
		lines = append(lines, settingsLines...)
	} else {
		body := make([]string, 0, end-start-1)
		for _, raw := range lines[start+1 : end] {
			line := strings.TrimSpace(raw)
			key, _, ok := strings.Cut(line, "=")
			if ok {
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "running_indicator", "running_indicator_rpm":
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
