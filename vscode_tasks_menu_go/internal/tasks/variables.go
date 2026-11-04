package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// VariableContext contains host-resolved execution values. The File value
// must be an absolute local path validated by the server, not a browser path.
type VariableContext struct {
	File             string
	LineNumber       int
	ColumnNumber     int
	SelectedText     string
	Cwd              string
	Config           map[string]string
	WorkspaceFolders map[string]string
}

var taskVariablePattern = regexp.MustCompile("\\$\\{[^}]+\\}")

// LoadVSCodeConfiguration reads scalar workspace settings from JSONC.
func LoadVSCodeConfiguration(workspace string) (map[string]string, error) {
	path := filepath.Join(workspace, ".vscode", "settings.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace settings: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("workspace settings exceed 1 MiB")
	}
	clean, err := stripJSONC(data)
	if err != nil {
		return nil, fmt.Errorf("parse workspace settings: %w", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(clean, &settings); err != nil {
		return nil, fmt.Errorf("parse workspace settings: %w", err)
	}
	result := make(map[string]string)
	var visit func(string, any)
	visit = func(key string, value any) {
		switch v := value.(type) {
		case string:
			result[key] = v
		case bool:
			result[key] = strconv.FormatBool(v)
		case float64:
			result[key] = strconv.FormatFloat(v, 'f', -1, 64)
		case map[string]any:
			for child, item := range v {
				visit(key+"."+child, item)
			}
		}
	}
	for key, value := range settings {
		visit(key, value)
	}
	return result, nil
}

func relativeVariablePath(root, file string) (string, error) {
	if root == "" || file == "" {
		return "", fmt.Errorf("file active is required")
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("file active does not belong to workspace %q", root)
	}
	return filepath.ToSlash(rel), nil
}

func taskVariableValue(name, workspace string, inputs map[string]string, ctx VariableContext) (string, bool, error) {
	requireFile := func() error {
		if ctx.File == "" || !filepath.IsAbs(ctx.File) {
			return fmt.Errorf("biến ${%s} yêu cầu tab file local đang active", name)
		}
		return nil
	}
	switch name {
	case "workspaceFolder":
		return workspace, true, nil
	case "workspaceFolderBasename":
		return filepath.Base(workspace), true, nil
	case "cwd":
		if ctx.Cwd != "" {
			return ctx.Cwd, true, nil
		}
		return workspace, true, nil
	case "pathSeparator", "/":
		return string(filepath.Separator), true, nil
	case "userHome":
		home, err := os.UserHomeDir()
		if err != nil { return "", true, fmt.Errorf("cannot resolve user home: %w", err) }
		return home, true, nil
	case "file", "fileWorkspaceFolder", "fileBasename", "fileBasenameNoExtension", "fileExtname", "fileDirname", "fileDirnameBasename",
		"relativeFile", "relativeFileDirname", "lineNumber", "columnNumber", "selectedText":
		if err := requireFile(); err != nil {
			return "", true, err
		}
		base := filepath.Base(ctx.File)
		switch name {
		case "file":
			return ctx.File, true, nil
		case "fileWorkspaceFolder":
			return workspace, true, nil
		case "fileBasename":
			return base, true, nil
		case "fileBasenameNoExtension":
			return strings.TrimSuffix(base, filepath.Ext(base)), true, nil
		case "fileExtname":
			return filepath.Ext(base), true, nil
		case "fileDirname":
			return filepath.Dir(ctx.File), true, nil
		case "fileDirnameBasename":
			return filepath.Base(filepath.Dir(ctx.File)), true, nil
		case "lineNumber":
			if ctx.LineNumber <= 0 {
				return "", true, fmt.Errorf("không có dòng con trỏ active")
			}
			return strconv.Itoa(ctx.LineNumber), true, nil
		case "columnNumber":
			if ctx.ColumnNumber <= 0 {
				return "", true, fmt.Errorf("không có cột con trỏ active")
			}
			return strconv.Itoa(ctx.ColumnNumber), true, nil
		case "selectedText":
			return ctx.SelectedText, true, nil
		}
		rel, err := relativeVariablePath(workspace, ctx.File)
		if err != nil {
			return "", true, err
		}
		if name == "relativeFile" {
			return rel, true, nil
		}
		return filepath.ToSlash(filepath.Dir(rel)), true, nil
	}
	if strings.HasPrefix(name, "workspaceFolder:") {
		key := strings.TrimPrefix(name, "workspaceFolder:")
		root, ok := ctx.WorkspaceFolders[key]
		if !ok || root == "" {
			return "", true, fmt.Errorf("workspace %q not found", key)
		}
		return root, true, nil
	}
	if strings.HasPrefix(name, "relativeFile:") {
		if err := requireFile(); err != nil {
			return "", true, err
		}
		key := strings.TrimPrefix(name, "relativeFile:")
		root, ok := ctx.WorkspaceFolders[key]
		if !ok || root == "" {
			return "", true, fmt.Errorf("workspace %q not found", key)
		}
		rel, err := relativeVariablePath(root, ctx.File)
		return rel, true, err
	}
	if strings.HasPrefix(name, "env:") {
		key := strings.TrimPrefix(name, "env:")
		if !envVariablePattern.MatchString("${env:"+key+"}") {
			return "", true, fmt.Errorf("invalid env variable %q", key)
		}
		return os.Getenv(key), true, nil
	}
	if strings.HasPrefix(name, "input:") {
		key := strings.TrimPrefix(name, "input:")
		value, ok := inputs[key]
		if !ok {
			return "", true, fmt.Errorf("thiếu giá trị cho input %q", key)
		}
		return value, true, nil
	}
	if strings.HasPrefix(name, "config:") {
		key := strings.TrimPrefix(name, "config:")
		value, ok := ctx.Config[key]
		if !ok {
			return "", true, fmt.Errorf("missing workspace setting %q", key)
		}
		return value, true, nil
	}
	if name == "execPath" || name == "defaultBuildTask" || strings.HasPrefix(name, "command:") || strings.HasPrefix(name, "extension:") {
		return "", true, fmt.Errorf("VS Code variable %q is not supported by TaskDeck", name)
	}
	// Preserve ordinary shell expressions and unknown extension-specific references.
	return "", false, nil
}

// Substitution is quote-aware for shell command text. Input variables inside
// an existing quoted string must not break the surrounding shell argument.
func expandVariablesWithContext(value, workspace string, inputs map[string]string, ctx VariableContext, shell bool) (string, error) {
	matches := taskVariablePattern.FindAllStringIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}
	var out strings.Builder
	last := 0
	quote := byte(0)
	advance := func(part string) {
		for i := 0; i < len(part); i++ {
			ch := part[i]
			if ch == '\\' && quote != '\'' {
				if i+1 < len(part) {
					i++
				}
				continue
			}
			switch ch {
			case '\'':
				if quote == 0 {
					quote = '\''
				} else if quote == '\'' {
					quote = 0
				}
			case '"':
				if quote == 0 {
					quote = '"'
				} else if quote == '"' {
					quote = 0
				}
			}
		}
	}
	for _, indices := range matches {
		part := value[last:indices[0]]
		out.WriteString(part)
		if shell {
			advance(part)
		}
		token := value[indices[0]:indices[1]]
		replacement, known, err := taskVariableValue(token[2:len(token)-1], workspace, inputs, ctx)
		if err != nil {
			return "", err
		}
		if !known {
			out.WriteString(token)
		} else if !shell {
			out.WriteString(replacement)
		} else {
			switch quote {
			case '\'':
				out.WriteString(strings.ReplaceAll(replacement, "'", "'\"'\"'"))
			case '"':
				tick := string(rune(96))
				escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", tick, "\\"+tick).Replace(replacement)
				out.WriteString(escaped)
			default:
				out.WriteString(shellQuote(replacement))
			}
		}
		last = indices[1]
	}
	out.WriteString(value[last:])
	return out.String(), nil
}
