package config

import (
	"fmt"
	"os"
	"path"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	defaultGitScanDepth = 8
	maxGitScanDepth     = 12
	maxGitRepositories  = 128
)

type GitSettings struct {
	ScanEnabled               bool              `json:"scan_enabled"`
	ScanDepth                 int               `json:"scan_depth"`
	DefaultRepository         string            `json:"default_repository"`
	AutoSelectFromTerminalCWD bool              `json:"auto_select_from_terminal_cwd"`
	Repositories              map[string]string `json:"repositories"`
}

func DefaultGitSettings() GitSettings {
	return GitSettings{
		ScanEnabled:       true,
		ScanDepth:         defaultGitScanDepth,
		DefaultRepository: ".",
		Repositories:      map[string]string{},
	}
}

func normalizeGitRepositoryPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." {
		return ".", nil
	}
	if len(value) > 2048 || strings.ContainsAny(value, "\r\n\x00") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("git repository path không hợp lệ")
	}
	clean := path.Clean(strings.TrimPrefix(value, "./"))
	if clean == "." {
		return ".", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("git repository path phải nằm trong workspace")
	}
	return clean, nil
}

func normalizeGitSettings(value GitSettings) (GitSettings, error) {
	if value.ScanDepth <= 0 {
		value.ScanDepth = defaultGitScanDepth
	}
	if value.ScanDepth > maxGitScanDepth {
		return GitSettings{}, fmt.Errorf("git.scan_depth vượt quá %d", maxGitScanDepth)
	}
	defaultRepo, err := normalizeGitRepositoryPath(value.DefaultRepository)
	if err != nil {
		return GitSettings{}, err
	}
	value.DefaultRepository = defaultRepo
	if value.Repositories == nil {
		value.Repositories = map[string]string{}
	}
	if len(value.Repositories) > maxGitRepositories {
		return GitSettings{}, fmt.Errorf("git.repositories vượt quá %d mục", maxGitRepositories)
	}
	normalized := make(map[string]string, len(value.Repositories))
	for rawName, rawPath := range value.Repositories {
		name := strings.TrimSpace(rawName)
		if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00=") {
			return GitSettings{}, fmt.Errorf("git repository name không hợp lệ")
		}
		repoPath, err := normalizeGitRepositoryPath(rawPath)
		if err != nil {
			return GitSettings{}, fmt.Errorf("git repository %q: %w", name, err)
		}
		normalized[name] = repoPath
	}
	value.Repositories = normalized
	return value, nil
}

func ReadGitSettings(workspace string) (GitSettings, error) {
	cfg := DefaultGitSettings()
	configPath, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return GitSettings{}, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return GitSettings{}, fmt.Errorf("đọc %s: %w", configPath, err)
	}
	if len(data) > 1<<20 {
		return GitSettings{}, fmt.Errorf("%s quá lớn", configPath)
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
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		switch section {
		case "git":
			switch strings.ToLower(key) {
			case "scan_enabled":
				cfg.ScanEnabled = parseBool(rawValue, true)
			case "scan_depth":
				var depth int
				if _, err := fmt.Sscanf(rawValue, "%d", &depth); err != nil {
					return GitSettings{}, fmt.Errorf("git.scan_depth không hợp lệ: %q", rawValue)
				}
				cfg.ScanDepth = depth
			case "default_repository":
				cfg.DefaultRepository = rawValue
			case "auto_select_from_terminal_cwd":
				cfg.AutoSelectFromTerminalCWD = parseBool(rawValue, false)
			}
		case "git.repositories":
			if cfg.Repositories == nil {
				cfg.Repositories = map[string]string{}
			}
			cfg.Repositories[key] = rawValue
		}
	}
	return normalizeGitSettings(cfg)
}
