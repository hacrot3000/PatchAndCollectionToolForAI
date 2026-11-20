package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Execution struct {
	TaskID           int      `json:"task_id"`
	Label            string   `json:"label"`
	Detail           string   `json:"detail,omitempty"`
	Command          string   `json:"command"`
	Args             []string `json:"args"`
	Cwd              string   `json:"cwd"`
	Env              []string `json:"-"`
	Preview          string   `json:"preview"`
	TargetType       string   `json:"-"`
	TargetProfileID  string   `json:"-"`
	ProtocolEvents   bool     `json:"-"`
	ProtocolCommands bool     `json:"-"`
	SessionKind      string   `json:"-"`
	OwnerUserID      string   `json:"-"`
	ProjectID        string   `json:"-"`
}

const (
	SessionKindTask     = "task"
	SessionKindTerminal = "terminal"
	SessionKindPatch    = "patch"
)

var envVariablePattern = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)
var inputVariablePattern = regexp.MustCompile(`\$\{input:([^}]+)\}`)

func ResolveExecution(task Task, workspace string) (Execution, error) {
	return ResolveExecutionWithInputs(task, workspace, nil)
}

func ResolveExecutionWithInputs(task Task, workspace string, inputs map[string]string) (Execution, error) {
	return ResolveExecutionWithContext(task, workspace, inputs, VariableContext{})
}

func ResolveExecutionWithContext(task Task, workspace string, inputs map[string]string, variables VariableContext) (Execution, error) {
	command, ok := task.Command.(string)
	if !ok || strings.TrimSpace(command) == "" {
		return Execution{}, fmt.Errorf("task %q không có command dạng chuỗi", task.Label)
	}
	cwd := workspace
	var err error
	if raw := stringValue(task.Options["cwd"]); raw != "" {
		cwd, err = expandVariablesWithContext(raw, workspace, inputs, variables, false)
		if err != nil { return Execution{}, fmt.Errorf("task %q: %w", task.Label, err) }
		if !filepath.IsAbs(cwd) { cwd = filepath.Join(workspace, cwd) }
	}
	variables.Cwd = filepath.Clean(cwd)
	command, err = expandVariablesWithContext(command, workspace, inputs, variables, !strings.EqualFold(task.Type, "process"))
	if err != nil { return Execution{}, fmt.Errorf("task %q: %w", task.Label, err) }
	args, err := stringArgsWithContext(task.Args, workspace, inputs, variables)
	if err != nil { return Execution{}, fmt.Errorf("task %q: %w", task.Label, err) }
	env, err := mergeEnvironmentWithContext(task.Options["env"], workspace, inputs, variables)
	if err != nil { return Execution{}, fmt.Errorf("task %q: %w", task.Label, err) }

	execSpec := Execution{TaskID: task.ID, Label: task.Label, Detail: task.Detail, Cwd: filepath.Clean(cwd), Env: env}
	if strings.EqualFold(task.Type, "process") {
		execSpec.Command = command
		execSpec.Args = args
		execSpec.Preview = joinPreview(command, args)
		return execSpec, nil
	}

	shellExecutable := "/bin/bash"
	// subprocess.run(..., shell=True, executable=/bin/bash) của menu Python
	// tương đương bash -c, không phải login shell (-l).
	shellArgs := []string{"-c"}
	if shell, ok := task.Options["shell"].(map[string]any); ok {
		if value := stringValue(shell["executable"]); value != "" {
			shellExecutable, err = expandVariablesWithContext(value, workspace, inputs, variables, false)
			if err != nil {
				return Execution{}, fmt.Errorf("task %q: %w", task.Label, err)
			}
		}
		if values, valuesErr := stringArgsWithContext(shell["args"], workspace, inputs, variables); valuesErr != nil {
			return Execution{}, fmt.Errorf("task %q: %w", task.Label, valuesErr)
		} else if len(values) > 0 {
			shellArgs = values
		}
	}
	fullCommand := command
	if len(args) > 0 {
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = shellQuote(arg)
		}
		fullCommand += " " + strings.Join(quoted, " ")
	}
	execSpec.Command = shellExecutable
	execSpec.Args = append(shellArgs, fullCommand)
	execSpec.Preview = fullCommand
	return execSpec, nil
}

func stringArgs(value any, workspace string, inputs map[string]string) ([]string, error) {
	return stringArgsWithContext(value, workspace, inputs, VariableContext{})
}

func stringArgsWithContext(value any, workspace string, inputs map[string]string, variables VariableContext) ([]string, error) {
	values, ok := value.([]any)
	if !ok {
		if stringsValue, ok := value.([]string); ok {
			out := make([]string, len(stringsValue))
			for i, v := range stringsValue {
				expanded, err := expandVariablesWithContext(v, workspace, inputs, variables, false)
				if err != nil {
					return nil, err
				}
				out[i] = expanded
			}
			return out, nil
		}
		return nil, nil
	}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		switch v := raw.(type) {
		case string:
			expanded, err := expandVariablesWithContext(v, workspace, inputs, variables, false)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded)
		case map[string]any:
			if value := stringValue(v["value"]); value != "" {
				expanded, err := expandVariablesWithContext(value, workspace, inputs, variables, false)
				if err != nil {
					return nil, err
				}
				out = append(out, expanded)
			}
		default:
			out = append(out, fmt.Sprint(v))
		}
	}
	return out, nil
}

func expandVariables(value, workspace string, inputs map[string]string) (string, error) {
	return expandVariablesWithContext(value, workspace, inputs, VariableContext{}, false)
}

func mergeEnvironment(raw any, workspace string, inputs map[string]string) ([]string, error) {
	return mergeEnvironmentWithContext(raw, workspace, inputs, VariableContext{})
}

func mergeEnvironmentWithContext(raw any, workspace string, inputs map[string]string, variables VariableContext) ([]string, error) {
	values := map[string]string{}
	for _, item := range os.Environ() {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	if custom, ok := raw.(map[string]any); ok {
		for key, value := range custom {
			expanded, err := expandVariablesWithContext(fmt.Sprint(value), workspace, inputs, variables, false)
			if err != nil {
				return nil, err
			}
			values[key] = expanded
		}
	}
	if _, ok := values["TERM"]; !ok {
		values["TERM"] = "xterm-256color"
	}
	values["COLORTERM"] = "truecolor"
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out, nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '@' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func joinPreview(command string, args []string) string {
	parts := []string{shellQuote(command)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}
