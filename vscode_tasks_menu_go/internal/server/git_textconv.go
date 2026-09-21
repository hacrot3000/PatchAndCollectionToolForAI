package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/state"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

const terminalGitTextconvAttributes = `# Managed by vscode_tasks_menu for terminal-only Git diffs.
# The textconv helper changes only standalone CR to LF; CRLF stays unchanged.
*.java diff=taskmenu-cr
*.kt diff=taskmenu-cr
*.kts diff=taskmenu-cr
*.c diff=taskmenu-cr
*.h diff=taskmenu-cr
*.cc diff=taskmenu-cr
*.cpp diff=taskmenu-cr
*.cxx diff=taskmenu-cr
*.hpp diff=taskmenu-cr
*.hxx diff=taskmenu-cr
*.go diff=taskmenu-cr
*.rs diff=taskmenu-cr
*.py diff=taskmenu-cr
*.js diff=taskmenu-cr
*.jsx diff=taskmenu-cr
*.mjs diff=taskmenu-cr
*.cjs diff=taskmenu-cr
*.ts diff=taskmenu-cr
*.tsx diff=taskmenu-cr
*.json diff=taskmenu-cr
*.jsonc diff=taskmenu-cr
*.xml diff=taskmenu-cr
*.html diff=taskmenu-cr
*.htm diff=taskmenu-cr
*.css diff=taskmenu-cr
*.scss diff=taskmenu-cr
*.less diff=taskmenu-cr
*.php diff=taskmenu-cr
*.sh diff=taskmenu-cr
*.bash diff=taskmenu-cr
*.zsh diff=taskmenu-cr
*.yaml diff=taskmenu-cr
*.yml diff=taskmenu-cr
*.toml diff=taskmenu-cr
*.ini diff=taskmenu-cr
*.cfg diff=taskmenu-cr
*.conf diff=taskmenu-cr
*.properties diff=taskmenu-cr
*.gradle diff=taskmenu-cr
*.md diff=taskmenu-cr
*.txt diff=taskmenu-cr
*.sql diff=taskmenu-cr
*.proto diff=taskmenu-cr
*.vue diff=taskmenu-cr
*.svelte diff=taskmenu-cr
*.cs diff=taskmenu-cr
*.swift diff=taskmenu-cr
*.rb diff=taskmenu-cr
*.pl diff=taskmenu-cr
*.lua diff=taskmenu-cr
*.scala diff=taskmenu-cr
Makefile diff=taskmenu-cr
Dockerfile diff=taskmenu-cr
`

func configureTerminalGitTextconv(workspace string, spec *tasks.Execution) error {
	if spec == nil {
		return fmt.Errorf("terminal execution is nil")
	}
	if err := state.EnsureDir(workspace); err != nil {
		return err
	}
	attributesPath := filepath.Join(state.Dir(workspace), "git-textconv.attributes")
	content := []byte(terminalGitTextconvAttributes)
	if existing := readUserGlobalAttributes(attributesPath); len(existing) > 0 {
		content = append(content, '\n')
		content = append(content, []byte("# Existing user-level Git attributes follow and keep higher precedence.\n")...)
		content = append(content, existing...)
		if content[len(content)-1] != '\n' {
			content = append(content, '\n')
		}
	}
	if current, err := os.ReadFile(attributesPath); err != nil || !bytes.Equal(current, content) {
		if err := os.WriteFile(attributesPath, content, 0o600); err != nil {
			return fmt.Errorf("write terminal Git attributes: %w", err)
		}
	}
	if err := os.Chmod(attributesPath, 0o600); err != nil {
		return fmt.Errorf("protect terminal Git attributes: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve vscode_tasks_menu executable: %w", err)
	}
	count, err := gitConfigEnvCount(spec.Env)
	if err != nil {
		return err
	}
	textconvCommand := shellQuoteGitCommandArg(exe) + " --git-textconv"
	overrides := map[string]string{
		"GIT_CONFIG_COUNT":                         strconv.Itoa(count + 3),
		"GIT_CONFIG_KEY_" + strconv.Itoa(count):     "core.attributesFile",
		"GIT_CONFIG_VALUE_" + strconv.Itoa(count):   attributesPath,
		"GIT_CONFIG_KEY_" + strconv.Itoa(count+1):   "diff.taskmenu-cr.textconv",
		"GIT_CONFIG_VALUE_" + strconv.Itoa(count+1): textconvCommand,
		"GIT_CONFIG_KEY_" + strconv.Itoa(count+2):   "diff.taskmenu-cr.cachetextconv",
		"GIT_CONFIG_VALUE_" + strconv.Itoa(count+2): "false",
	}
	return tasks.ApplyEnvironmentOverrides(spec, overrides)
}

func gitConfigEnvCount(env []string) (int, error) {
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key != "GIT_CONFIG_COUNT" {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || count < 0 {
			return 0, fmt.Errorf("invalid inherited GIT_CONFIG_COUNT %q", value)
		}
		return count, nil
	}
	return 0, nil
}

func shellQuoteGitCommandArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func readUserGlobalAttributes(managedPath string) []byte {
	path := configuredGlobalAttributesPath()
	if path == "" {
		return nil
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	managed, err := filepath.Abs(managedPath)
	if err == nil && clean == managed {
		return nil
	}
	info, err := os.Stat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil
	}
	return data
}

func configuredGlobalAttributesPath() string {
	if git, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command(git, "config", "--global", "--path", "--get", "core.attributesFile")
		if out, err := cmd.Output(); err == nil {
			if path := strings.TrimSpace(string(out)); path != "" {
				return path
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		path := filepath.Join(xdg, "git", "attributes")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	path := filepath.Join(home, ".config", "git", "attributes")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	legacy := filepath.Join(home, ".gitattributes")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return ""
}
