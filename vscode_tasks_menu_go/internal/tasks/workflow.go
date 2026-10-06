package tasks

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	maxWorkflowTasks = 256
	maxWorkflowRetries = 10
	maxWorkflowTimeoutSeconds = 24 * 60 * 60
)

type WorkflowPolicy struct {
	Retry int `json:"retry"`
	TimeoutSeconds int `json:"timeout_seconds"`
	ContinueOnError bool `json:"continue_on_error"`
	Condition string `json:"condition,omitempty"`
	Outputs []string `json:"outputs,omitempty"`
}

type WorkflowNode struct {
	ID int `json:"id"`
	Label string `json:"label"`
	DependsOn []string `json:"depends_on,omitempty"`
	DependsOrder string `json:"depends_order,omitempty"`
	Policy WorkflowPolicy `json:"policy"`
}

type WorkflowGraph struct {
	Root string `json:"root,omitempty"`
	Nodes []WorkflowNode `json:"nodes"`
	Edges [][2]string `json:"edges"`
	Order []string `json:"order"`
}

func workflowStringList(value any) []string {
	var raw []string
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			raw = []string{strings.TrimSpace(v)}
		}
	case []any:
		for _, item := range v {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				raw = append(raw, text)
			}
		}
	case []string:
		for _, item := range v {
			if text := strings.TrimSpace(item); text != "" {
				raw = append(raw, text)
			}
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func workflowInt(value any, min, max int) int {
	switch v := value.(type) {
	case float64:
		if int(v) >= min && int(v) <= max {
			return int(v)
		}
	case int:
		if v >= min && v <= max {
			return v
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && parsed >= min && parsed <= max {
			return parsed
		}
	}
	return 0
}

func workflowBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	default:
		return false
	}
}

func WorkflowPolicyForTask(task Task) WorkflowPolicy {
	policy := WorkflowPolicy{}
	meta, _ := task.Raw["taskdeck"].(map[string]any)
	workflow, _ := meta["workflow"].(map[string]any)
	if workflow == nil {
		return policy
	}
	policy.Retry = workflowInt(workflow["retry"], 0, maxWorkflowRetries)
	policy.TimeoutSeconds = workflowInt(workflow["timeoutSeconds"], 0, maxWorkflowTimeoutSeconds)
	policy.ContinueOnError = workflowBool(workflow["continueOnError"])
	policy.Condition = strings.ToLower(strings.TrimSpace(fmt.Sprint(workflow["condition"])))
	if workflow["condition"] == nil {
		policy.Condition = ""
	}
	switch policy.Condition {
	case "", "success", "failure", "always":
	default:
		policy.Condition = ""
	}
	policy.Outputs = workflowStringList(workflow["outputs"])
	return policy
}

func WorkflowNodeForTask(task Task) WorkflowNode {
	order := strings.ToLower(strings.TrimSpace(fmt.Sprint(task.Raw["dependsOrder"])))
	if task.Raw["dependsOrder"] == nil {
		order = ""
	}
	switch order {
	case "sequence":
	default:
		order = "parallel"
	}
	return WorkflowNode{
		ID: task.ID,
		Label: task.Label,
		DependsOn: workflowStringList(task.Raw["dependsOn"]),
		DependsOrder: order,
		Policy: WorkflowPolicyForTask(task),
	}
}

func BuildWorkflowGraph(items []Task, rootLabel string) (WorkflowGraph, error) {
	if len(items) > maxWorkflowTasks {
		return WorkflowGraph{}, fmt.Errorf("task graph exceeds %d tasks", maxWorkflowTasks)
	}
	byLabel := make(map[string]Task, len(items))
	for _, task := range items {
		if _, exists := byLabel[task.Label]; exists {
			return WorkflowGraph{}, fmt.Errorf("duplicate task label %q", task.Label)
		}
		byLabel[task.Label] = task
	}
	rootLabel = strings.TrimSpace(rootLabel)
	if rootLabel != "" {
		if _, exists := byLabel[rootLabel]; !exists {
			return WorkflowGraph{}, fmt.Errorf("workflow root %q not found", rootLabel)
		}
	}

	visiting := map[string]bool{}
	visited := map[string]bool{}
	order := make([]string, 0, len(items))
	nodes := make([]WorkflowNode, 0, len(items))
	edges := make([][2]string, 0)
	var visit func(string) error
	visit = func(label string) error {
		if visited[label] {
			return nil
		}
		if visiting[label] {
			return fmt.Errorf("task dependency cycle detected at %q", label)
		}
		task, ok := byLabel[label]
		if !ok {
			return fmt.Errorf("task dependency %q not found", label)
		}
		visiting[label] = true
		node := WorkflowNodeForTask(task)
		for _, dep := range node.DependsOn {
			if dep == label {
				return fmt.Errorf("task %q cannot depend on itself", label)
			}
			if _, exists := byLabel[dep]; !exists {
				return fmt.Errorf("task %q depends on missing task %q", label, dep)
			}
			edges = append(edges, [2]string{dep, label})
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[label] = false
		visited[label] = true
		nodes = append(nodes, node)
		order = append(order, label)
		return nil
	}

	if rootLabel != "" {
		if err := visit(rootLabel); err != nil {
			return WorkflowGraph{}, err
		}
	} else {
		labels := make([]string, 0, len(byLabel))
		for label := range byLabel {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			if err := visit(label); err != nil {
				return WorkflowGraph{}, err
			}
		}
	}
	if len(nodes) == 0 {
		return WorkflowGraph{}, errors.New("task graph is empty")
	}
	return WorkflowGraph{Root: rootLabel, Nodes: nodes, Edges: edges, Order: order}, nil
}

func TaskByLabel(items []Task, label string) (Task, bool) {
	for _, task := range items {
		if task.Label == label {
			return task, true
		}
	}
	return Task{}, false
}
