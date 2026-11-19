package taskmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion    = 1
	MaxMarkdownBytes = 1 << 20
	MaxTasks         = 100
	MaxTaskTitle     = 200
	MaxTaskTextBytes = 32 << 10
	MaxDependencies  = 64
)

var taskHeading = regexp.MustCompile(`^##\s+Task\s+([A-Za-z0-9][A-Za-z0-9_.-]{0,63})\s*(?:[—–:-]\s*)?(.+?)\s*$`)
var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type Manifest struct {
	SchemaVersion int     `json:"schema_version"`
	ManifestID    string  `json:"manifest_id"`
	Name          string  `json:"name"`
	ProjectID     string  `json:"project_id"`
	SourceSHA256  string  `json:"source_sha256"`
	Tasks         []Task  `json:"tasks"`
	Issues        []Issue `json:"issues"`
	Graph         Graph   `json:"graph"`
	Valid         bool    `json:"valid"`
	PlanningOnly  bool    `json:"planning_only"`
}

type Task struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Dependencies []string `json:"dependencies"`
	Acceptance   []string `json:"acceptance_criteria"`
	SourceLine   int      `json:"source_line"`
}

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	Warning bool   `json:"warning,omitempty"`
}

type Graph struct {
	Roots       []string   `json:"roots"`
	Blocked     []string   `json:"blocked"`
	Topological []string   `json:"topological_order,omitempty"`
	Cycles      [][]string `json:"cycles,omitempty"`
}

type rawTask struct {
	Task
	DependencyLine int
}

// ParseMarkdown produces a canonical, read-only task manifest. It never reads
// project files, executes commands, calls a model, writes to disk, or silently
// repairs a dependency graph.
func ParseMarkdown(markdown, projectID string) (Manifest, error) {
	if len(markdown) > MaxMarkdownBytes {
		return Manifest{}, errors.New("markdown exceeds 1 MiB limit")
	}
	if strings.TrimSpace(projectID) == "" || len(projectID) > 128 {
		return Manifest{}, errors.New("verified project ID required")
	}
	if !utf8.ValidString(markdown) {
		return Manifest{}, errors.New("markdown must be valid UTF-8")
	}
	if strings.ContainsRune(markdown, '\x00') {
		return Manifest{}, errors.New("markdown contains NUL")
	}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	name := "Untitled plan"
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			if name == "" {
				name = "Untitled plan"
			}
			_ = i
			break
		}
	}
	manifest := Manifest{SchemaVersion: SchemaVersion, Name: name, ProjectID: projectID, Tasks: []Task{}, Issues: []Issue{}, PlanningOnly: true}
	sum := sha256.Sum256([]byte(markdown))
	manifest.SourceSHA256 = hex.EncodeToString(sum[:])
	manifest.ManifestID = "plan-" + hex.EncodeToString(sum[:12])

	var raws []rawTask
	current := -1
	inAcceptance := false
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if match := taskHeading.FindStringSubmatch(trimmed); match != nil {
			if len(raws) >= MaxTasks {
				return Manifest{}, fmt.Errorf("task count exceeds %d limit", MaxTasks)
			}
			raws = append(raws, rawTask{Task: Task{ID: match[1], Title: strings.TrimSpace(match[2]), Dependencies: []string{}, Acceptance: []string{}, SourceLine: lineNo}})
			current = len(raws) - 1
			inAcceptance = false
			continue
		}
		if current < 0 {
			continue
		}
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "depends on:") {
			if raws[current].DependencyLine != 0 {
				manifest.Issues = append(manifest.Issues, Issue{Code: "duplicate_dependencies", Message: "task has more than one Depends on field", Line: lineNo, TaskID: raws[current].ID})
				continue
			}
			raws[current].DependencyLine = lineNo
			value := strings.TrimSpace(trimmed[len("Depends on:"):])
			raws[current].Dependencies = parseDependencies(value)
			inAcceptance = false
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(trimmed, ":"), "Acceptance") || strings.EqualFold(strings.TrimSuffix(trimmed, ":"), "Acceptance criteria") {
			inAcceptance = true
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			inAcceptance = false
			continue
		}
		if inAcceptance {
			item := strings.TrimSpace(strings.TrimLeft(trimmed, "- *+\t"))
			if item != "" {
				raws[current].Acceptance = append(raws[current].Acceptance, item)
			}
			continue
		}
		if len(raws[current].Description)+len(trimmed) > MaxTaskTextBytes {
			manifest.Issues = append(manifest.Issues, Issue{Code: "task_text_too_large", Message: "task description exceeds 32 KiB", Line: lineNo, TaskID: raws[current].ID})
			continue
		}
		if raws[current].Description != "" {
			raws[current].Description += "\n"
		}
		raws[current].Description += trimmed
	}

	if len(raws) == 0 {
		manifest.Issues = append(manifest.Issues, Issue{Code: "no_tasks", Message: "no task headings found"})
	}
	if len(raws) > MaxTasks {
		return Manifest{}, fmt.Errorf("task count exceeds %d limit", MaxTasks)
	}
	ids := make(map[string]int, len(raws))
	for i, raw := range raws {
		if !taskIDPattern.MatchString(raw.ID) {
			manifest.Issues = append(manifest.Issues, Issue{Code: "invalid_task_id", Message: "task ID contains unsupported characters", Line: raw.SourceLine, TaskID: raw.ID})
		}
		if raw.Title == "" || len(raw.Title) > MaxTaskTitle {
			manifest.Issues = append(manifest.Issues, Issue{Code: "invalid_title", Message: "task title must contain 1–200 characters", Line: raw.SourceLine, TaskID: raw.ID})
		}
		if first, exists := ids[raw.ID]; exists {
			manifest.Issues = append(manifest.Issues, Issue{Code: "duplicate_id", Message: fmt.Sprintf("task ID duplicates task at source line %d", raws[first].SourceLine), Line: raw.SourceLine, TaskID: raw.ID})
		} else {
			ids[raw.ID] = i
		}
		if len(raw.Dependencies) > MaxDependencies {
			manifest.Issues = append(manifest.Issues, Issue{Code: "too_many_dependencies", Message: "task has more than 64 dependencies", Line: raw.DependencyLine, TaskID: raw.ID})
		}
		if len(raw.Acceptance) == 0 {
			manifest.Issues = append(manifest.Issues, Issue{Code: "missing_acceptance", Message: "task has no acceptance criteria", Line: raw.SourceLine, TaskID: raw.ID, Warning: true})
		}
		manifest.Tasks = append(manifest.Tasks, raw.Task)
	}

	for i := range manifest.Tasks {
		task := &manifest.Tasks[i]
		seenDeps := make(map[string]bool)
		for _, dep := range task.Dependencies {
			if dep == task.ID {
				manifest.Issues = append(manifest.Issues, Issue{Code: "self_dependency", Message: "task cannot depend on itself", Line: raws[i].DependencyLine, TaskID: task.ID})
				continue
			}
			if seenDeps[dep] {
				manifest.Issues = append(manifest.Issues, Issue{Code: "duplicate_dependency", Message: "dependency is listed more than once", Line: raws[i].DependencyLine, TaskID: task.ID})
				continue
			}
			seenDeps[dep] = true
			if _, exists := ids[dep]; !exists {
				manifest.Issues = append(manifest.Issues, Issue{Code: "unknown_dependency", Message: "dependency does not name a task in this manifest", Line: raws[i].DependencyLine, TaskID: task.ID})
			}
		}
	}
	manifest.Graph = buildGraph(manifest.Tasks, ids, &manifest.Issues, raws)
	for _, issue := range manifest.Issues {
		switch issue.Code {
		case "unknown_dependency", "self_dependency", "duplicate_dependency", "duplicate_id", "dependency_cycle":
			// Do not present a topological order as actionable when dependency
			// references are invalid or cyclic.
			manifest.Graph.Topological = nil
		}
	}
	manifest.Valid = true
	for _, issue := range manifest.Issues {
		if !issue.Warning {
			manifest.Valid = false
			break
		}
	}
	return manifest, nil
}

func parseDependencies(value string) []string {
	if value == "" || strings.EqualFold(value, "none") || value == "-" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		dep := strings.TrimSpace(part)
		dep = strings.Trim(dep, "`[]")
		if dep != "" {
			out = append(out, dep)
		}
	}
	return out
}

func buildGraph(tasks []Task, ids map[string]int, issues *[]Issue, raws []rawTask) Graph {
	graph := Graph{Roots: []string{}, Blocked: []string{}}
	indegree := make([]int, len(tasks))
	children := make([][]int, len(tasks))
	for i, task := range tasks {
		if len(task.Dependencies) == 0 {
			graph.Roots = append(graph.Roots, task.ID)
		}
		for _, dep := range task.Dependencies {
			if depIdx, ok := ids[dep]; ok && depIdx != i {
				indegree[i]++
				children[depIdx] = append(children[depIdx], i)
			}
		}
		if len(task.Dependencies) > 0 {
			graph.Blocked = append(graph.Blocked, task.ID)
		}
	}
	ready := make([]int, 0)
	for i, d := range indegree {
		if d == 0 {
			ready = append(ready, i)
		}
	}
	for len(ready) > 0 {
		i := ready[0]
		ready = ready[1:]
		graph.Topological = append(graph.Topological, tasks[i].ID)
		for _, child := range children[i] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if len(graph.Topological) != len(tasks) {
		cycles := findCycles(tasks, ids)
		graph.Cycles = cycles
		for _, cycle := range cycles {
			line := 0
			if len(cycle) > 0 {
				if idx, ok := ids[cycle[0]]; ok {
					line = raws[idx].DependencyLine
				}
			}
			*issues = append(*issues, Issue{Code: "dependency_cycle", Message: "dependency cycle: " + strings.Join(cycle, " -> "), Line: line})
		}
		graph.Topological = nil
	}
	return graph
}

func findCycles(tasks []Task, ids map[string]int) [][]string {
	state := make([]uint8, len(tasks))
	stack := []int{}
	cycles := [][]string{}
	seen := make(map[string]bool)
	var visit func(int)
	visit = func(i int) {
		state[i] = 1
		stack = append(stack, i)
		for _, dep := range tasks[i].Dependencies {
			j, ok := ids[dep]
			if !ok {
				continue
			}
			if state[j] == 0 {
				visit(j)
			} else if state[j] == 1 {
				start := 0
				for k, node := range stack {
					if node == j {
						start = k
						break
					}
				}
				cycle := make([]string, 0, len(stack)-start+1)
				for _, node := range stack[start:] {
					cycle = append(cycle, tasks[node].ID)
				}
				cycle = append(cycle, tasks[j].ID)
				key := strings.Join(cycle, "\x00")
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[i] = 2
	}
	for i := range tasks {
		if state[i] == 0 {
			visit(i)
		}
	}
	return cycles
}

// SlugID is exposed for clients that want a stable suggested task ID before
// they add explicit IDs to Markdown. Parsing itself never silently rewrites IDs.
func SlugID(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-.")
}
