package tasks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var workflowOutputReferencePattern = regexp.MustCompile("\\$\\{output:([^}.]+)\\.([^}]+)\\}")

func workflowOutputEnvName(label, key string) string {
	sum := sha256.Sum256([]byte(label + "\x00" + key))
	return "TASKDECK_OUTPUT_" + strings.ToUpper(hex.EncodeToString(sum[:6]))
}

func workflowEnvDelta(env []string) []string {
	base := map[string]string{}
	for _, item := range os.Environ() {
		if key, value, ok := strings.Cut(item, "="); ok {
			base[key] = value
		}
	}
	out := []string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if current, exists := base[key]; exists && current == value {
			continue
		}
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func workflowCommandLine(spec Execution) string {
	parts := []string{}
	for _, item := range workflowEnvDelta(spec.Env) {
		key, value, _ := strings.Cut(item, "=")
		parts = append(parts, key+"="+shellQuote(value))
	}
	parts = append(parts, shellQuote(spec.Command))
	for _, arg := range spec.Args {
		parts = append(parts, shellQuote(arg))
	}
	line := strings.Join(parts, " ")
	if spec.Cwd != "" {
		line = "cd " + shellQuote(spec.Cwd) + " && " + line
	}
	return line
}

func replaceWorkflowOutputRefs(value string, declared map[string]map[string]bool) (string, error) {
	var firstErr error
	value = workflowOutputReferencePattern.ReplaceAllStringFunc(value, func(token string) string {
		match := workflowOutputReferencePattern.FindStringSubmatch(token)
		if len(match) != 3 {
			return token
		}
		label := strings.TrimSpace(match[1])
		key := strings.TrimSpace(match[2])
		if !declared[label][key] {
			if firstErr == nil {
				firstErr = fmt.Errorf("output dependency %s.%s is not declared", label, key)
			}
			return token
		}
		return "${" + workflowOutputEnvName(label, key) + "}"
	})
	return value, firstErr
}

func ResolveWorkflowExecution(items []Task, rootLabel, workspace string, inputs map[string]string) (Execution, error) {
	graph, err := BuildWorkflowGraph(items, rootLabel)
	if err != nil {
		return Execution{}, err
	}
	if len(graph.Nodes) == 1 && len(graph.Nodes[0].DependsOn) == 0 {
		task, _ := TaskByLabel(items, rootLabel)
		return ResolveExecutionWithInputs(task, workspace, inputs)
	}
	declared := map[string]map[string]bool{}
	nodes := map[string]WorkflowNode{}
	for _, node := range graph.Nodes {
		nodes[node.Label] = node
		declared[node.Label] = map[string]bool{}
		for _, key := range node.Policy.Outputs {
			declared[node.Label][key] = true
		}
	}

	var script strings.Builder
	script.WriteString("set -u\n")
	script.WriteString("declare -A TD_STATUS\n")
	script.WriteString("td_run(){ local name=\"$1\" retries=\"$2\" timeout_s=\"$3\" continue_err=\"$4\"; shift 4; local attempt=0 rc=0; while :; do attempt=$((attempt+1)); echo \"[TaskDeck workflow] $name · attempt $attempt/$((retries+1))\"; set +e; if [ \"$timeout_s\" -gt 0 ]; then timeout --foreground \"${timeout_s}s\" \"$@\"; rc=$?; else \"$@\"; rc=$?; fi; set -e; if [ $rc -eq 0 ] || [ $attempt -gt $retries ]; then break; fi; done; TD_STATUS[\"$name\"]=$rc; if [ $rc -ne 0 ] && [ \"$continue_err\" != \"1\" ]; then return $rc; fi; return 0; }\n")
	script.WriteString("set -e\n")

	for _, label := range graph.Order {
		task, ok := TaskByLabel(items, label)
		if !ok {
			return Execution{}, fmt.Errorf("task %q disappeared while compiling workflow", label)
		}
		node := nodes[label]
		spec, err := ResolveExecutionWithInputs(task, workspace, inputs)
		if err != nil {
			return Execution{}, err
		}
		line := workflowCommandLine(spec)
		line, err = replaceWorkflowOutputRefs(line, declared)
		if err != nil {
			return Execution{}, fmt.Errorf("task %q: %w", label, err)
		}

		condition := node.Policy.Condition
		if condition == "" {
			condition = "success"
		}
		if len(node.DependsOn) > 0 {
			checks := []string{}
			for _, dep := range node.DependsOn {
				switch condition {
				case "failure":
					checks = append(checks, "[ \"${TD_STATUS["+shellQuote(dep)+"]:-0}\" -ne 0 ]")
				case "always":
				// No gate.
				default:
					checks = append(checks, "[ \"${TD_STATUS["+shellQuote(dep)+"]:-0}\" -eq 0 ]")
				}
			}
			if len(checks) > 0 {
				joiner := " && "
				if condition == "failure" {
					joiner = " || "
				}
				script.WriteString("if " + strings.Join(checks, joiner) + "; then\n")
			}
		}

		logVar := "TD_LOG_" + workflowOutputEnvName(label, "log")
		if len(node.Policy.Outputs) > 0 {
			script.WriteString(logVar + "=$(mktemp)\n")
			line = "{ " + line + "; } 2>&1 | tee \"$" + logVar + "\""
		}
		script.WriteString("td_run " + shellQuote(label) + " " + fmt.Sprint(node.Policy.Retry) + " " + fmt.Sprint(node.Policy.TimeoutSeconds) + " " + bool01(node.Policy.ContinueOnError) + " /bin/bash -c " + shellQuote(line) + "\n")
		if len(node.Policy.Outputs) > 0 {
			for _, key := range node.Policy.Outputs {
				envName := workflowOutputEnvName(label, key)
				prefix := "::taskdeck-output " + key + "="
				script.WriteString(envName + "=$(grep -F " + shellQuote(prefix) + " \"$" + logVar + "\" | tail -n 1 | sed " + shellQuote("s/^"+regexp.QuoteMeta(prefix)+"//") + " || true)\n")
				script.WriteString("export " + envName + "\n")
			}
			script.WriteString("rm -f \"$" + logVar + "\"\n")
		}
		if len(node.DependsOn) > 0 && condition != "always" {
			script.WriteString("else echo " + shellQuote("[TaskDeck workflow] "+label+" skipped by condition "+condition) + "; TD_STATUS[" + shellQuote(label) + "]=0; fi\n")
		}
	}

	rootTask, _ := TaskByLabel(items, rootLabel)
	return Execution{
		TaskID: rootTask.ID,
		Label: rootTask.Label + " · workflow",
		Detail: "TaskDeck dependency graph workflow",
		Command: "/bin/bash",
		Args: []string{"-c", script.String()},
		Cwd: workspace,
		Env: os.Environ(),
		Preview: "Task workflow · " + strings.Join(graph.Order, " → "),
	}, nil
}

func bool01(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
