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

type workflowOutputRef struct {
	Label string
	Key string
	Env string
	Sentinel string
}

func workflowOutputEnvName(label, key string) string {
	sum := sha256.Sum256([]byte(label + "\x00" + key))
	return "TASKDECK_OUTPUT_" + strings.ToUpper(hex.EncodeToString(sum[:6]))
}

func workflowNodeID(label string) string {
	sum := sha256.Sum256([]byte(label))
	return strings.ToUpper(hex.EncodeToString(sum[:6]))
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

func replaceWorkflowString(value string, declared map[string]map[string]bool, refs map[string]workflowOutputRef) (string, error) {
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
		env := workflowOutputEnvName(label, key)
		sentinel := "TASKDECKOUTREF_" + strings.TrimPrefix(env, "TASKDECK_OUTPUT_")
		refs[sentinel] = workflowOutputRef{Label: label, Key: key, Env: env, Sentinel: sentinel}
		return sentinel
	})
	return value, firstErr
}

func replaceWorkflowValue(value any, declared map[string]map[string]bool, refs map[string]workflowOutputRef) (any, error) {
	switch v := value.(type) {
	case string:
		return replaceWorkflowString(v, declared, refs)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			next, err := replaceWorkflowValue(item, declared, refs)
			if err != nil {
				return nil, err
			}
			out[i] = next
		}
		return out, nil
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			next, err := replaceWorkflowString(item, declared, refs)
			if err != nil {
				return nil, err
			}
			out[i] = next
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			next, err := replaceWorkflowValue(item, declared, refs)
			if err != nil {
				return nil, err
			}
			out[key] = next
		}
		return out, nil
	default:
		return value, nil
	}
}

func prepareWorkflowTask(task Task, declared map[string]map[string]bool) (Task, []workflowOutputRef, error) {
	refs := map[string]workflowOutputRef{}
	command, err := replaceWorkflowValue(task.Command, declared, refs)
	if err != nil {
		return Task{}, nil, err
	}
	args, err := replaceWorkflowValue(task.Args, declared, refs)
	if err != nil {
		return Task{}, nil, err
	}
	optionsValue, err := replaceWorkflowValue(task.Options, declared, refs)
	if err != nil {
		return Task{}, nil, err
	}
	task.Command = command
	task.Args = args
	if optionsValue != nil {
		task.Options, _ = optionsValue.(map[string]any)
	}
	out := make([]workflowOutputRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sentinel < out[j].Sentinel })
	return task, out, nil
}

func injectWorkflowOutputExpansions(line string, refs []workflowOutputRef) string {
	for _, ref := range refs {
		line = strings.ReplaceAll(line, ref.Sentinel, "'\"${"+ref.Env+"}\"'")
	}
	return line
}

func workflowStatusFile(label string) string { return "$TD_DIR/status_" + workflowNodeID(label) }
func workflowLockDir(label string) string { return "$TD_DIR/lock_" + workflowNodeID(label) }
func workflowOutputFile(label, key string) string { return "$TD_DIR/output_" + workflowNodeID(label+"\x00"+key) }

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
			if key = strings.TrimSpace(key); key != "" {
				declared[node.Label][key] = true
			}
		}
	}

	type compiledNode struct {
		node WorkflowNode
		line string
		refs []workflowOutputRef
	}
	compiled := map[string]compiledNode{}
	for _, label := range graph.Order {
		task, ok := TaskByLabel(items, label)
		if !ok {
			return Execution{}, fmt.Errorf("task %q disappeared while compiling workflow", label)
		}
		prepared, refs, err := prepareWorkflowTask(task, declared)
		if err != nil {
			return Execution{}, fmt.Errorf("task %q: %w", label, err)
		}
		spec, err := ResolveExecutionWithInputs(prepared, workspace, inputs)
		if err != nil {
			return Execution{}, err
		}
		line := injectWorkflowOutputExpansions(workflowCommandLine(spec), refs)
		compiled[label] = compiledNode{node: nodes[label], line: line, refs: refs}
	}

	var script strings.Builder
	script.WriteString("set -u\n")
	script.WriteString("TD_DIR=$(mktemp -d); export TD_DIR\n")
	script.WriteString("trap 'rm -rf \"$TD_DIR\"' EXIT INT TERM\n")
	script.WriteString("td_read_status(){ if [ -f \"$1\" ]; then cat \"$1\"; else echo 125; fi; }\n")
	script.WriteString("td_wait_status(){ while [ ! -f \"$1\" ]; do sleep 0.05; done; cat \"$1\"; }\n")
	script.WriteString("td_exec(){ local retries=\"$1\" timeout_s=\"$2\"; shift 2; local attempt=0 rc=0; while :; do attempt=$((attempt+1)); if [ \"$timeout_s\" -gt 0 ]; then if ! command -v timeout >/dev/null 2>&1; then echo '[TaskDeck workflow] timeout command is unavailable' >&2; return 127; fi; timeout --foreground \"${timeout_s}s\" \"$@\"; rc=$?; else \"$@\"; rc=$?; fi; if [ $rc -eq 0 ] || [ $attempt -gt $retries ]; then return $rc; fi; echo \"[TaskDeck workflow] retry $attempt/$((retries+1))\"; done; }\n")

	for _, label := range graph.Order {
		entry := compiled[label]
		node := entry.node
		fn := "td_node_" + workflowNodeID(label)
		statusPath := workflowStatusFile(label)
		lockPath := workflowLockDir(label)
		script.WriteString(fn + "(){\n")
		script.WriteString(" local status=\"" + statusPath + "\" lock=\"" + lockPath + "\" existing rc=0 dep_rc=0 failed=0\n")
		script.WriteString(" if [ -f \"$status\" ]; then existing=$(cat \"$status\"); ")
		if node.Policy.ContinueOnError {
			script.WriteString("return 0")
		} else {
			script.WriteString("return \"$existing\"")
		}
		script.WriteString("; fi\n")
		script.WriteString(" if ! mkdir \"$lock\" 2>/dev/null; then existing=$(td_wait_status \"$status\"); ")
		if node.Policy.ContinueOnError {
			script.WriteString("return 0")
		} else {
			script.WriteString("return \"$existing\"")
		}
		script.WriteString("; fi\n")
		script.WriteString(" echo " + shellQuote("[TaskDeck workflow] "+label) + "\n")

		if len(node.DependsOn) > 0 {
			if node.DependsOrder == "sequence" {
				for _, dep := range node.DependsOn {
					script.WriteString(" td_node_" + workflowNodeID(dep) + " || true\n")
				}
			} else {
				script.WriteString(" local pids=()\n")
				for _, dep := range node.DependsOn {
					script.WriteString(" td_node_" + workflowNodeID(dep) + " & pids+=($!)\n")
				}
				script.WriteString(" local pid; for pid in \"${pids[@]}\"; do wait \"$pid\" || true; done\n")
			}
			for _, dep := range node.DependsOn {
				script.WriteString(" dep_rc=$(td_read_status \"" + workflowStatusFile(dep) + "\"); if [ \"$dep_rc\" -ne 0 ]; then failed=1; fi\n")
			}
			condition := node.Policy.Condition
			if condition == "" {
				condition = "success"
			}
			switch condition {
			case "success":
				script.WriteString(" if [ \"$failed\" -ne 0 ]; then echo '[TaskDeck workflow] skipped: dependency failed'; echo 1 > \"$status\"; rmdir \"$lock\"; return 1; fi\n")
			case "failure":
				script.WriteString(" if [ \"$failed\" -eq 0 ]; then echo '[TaskDeck workflow] skipped: failure condition not met'; echo 0 > \"$status\"; rmdir \"$lock\"; return 0; fi\n")
			case "always":
			}
		}

		for _, ref := range entry.refs {
			outFile := workflowOutputFile(ref.Label, ref.Key)
			script.WriteString(" if [ ! -f \"" + outFile + "\" ]; then echo " + shellQuote("[TaskDeck workflow] missing output "+ref.Label+"."+ref.Key) + " >&2; echo 125 > \"$status\"; rmdir \"$lock\"; return 125; fi\n")
			script.WriteString(" export " + ref.Env + "=$(cat \"" + outFile + "\")\n")
		}

		logPath := "$TD_DIR/log_" + workflowNodeID(label)
		command := entry.line
		if len(node.Policy.Outputs) > 0 {
			command = "{ " + command + "; } 2>&1 | tee \"" + logPath + "\"; exit ${PIPESTATUS[0]}"
		}
		script.WriteString(" td_exec " + fmt.Sprint(node.Policy.Retry) + " " + fmt.Sprint(node.Policy.TimeoutSeconds) + " /bin/bash -c " + shellQuote(command) + "; rc=$?\n")
		if len(node.Policy.Outputs) > 0 {
			for _, key := range node.Policy.Outputs {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				prefix := "::taskdeck-output " + key + "="
				outFile := workflowOutputFile(label, key)
				script.WriteString(" value=$(grep -F " + shellQuote(prefix) + " \"" + logPath + "\" | tail -n 1 | sed " + shellQuote("s/^"+regexp.QuoteMeta(prefix)+"//") + " || true); printf '%s' \"$value\" > \"" + outFile + "\"\n")
			}
		}
		script.WriteString(" echo \"$rc\" > \"$status\"; rmdir \"$lock\"\n")
		if node.Policy.ContinueOnError {
			script.WriteString(" return 0\n")
		} else {
			script.WriteString(" return \"$rc\"\n")
		}
		script.WriteString("}\n")
	}

	rootFn := "td_node_" + workflowNodeID(rootLabel)
	script.WriteString(rootFn + "; TD_ROOT_RC=$?\n")
	script.WriteString("exit \"$TD_ROOT_RC\"\n")

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
