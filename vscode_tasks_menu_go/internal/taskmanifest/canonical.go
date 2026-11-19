package taskmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type canonicalInput struct {
	SchemaVersion int    `json:"schema_version"`
	ManifestID    string `json:"manifest_id,omitempty"`
	Name          string `json:"name"`
	ProjectID     string `json:"project_id"`
	Tasks         []Task `json:"tasks"`
}

// ParseCanonicalJSON validates user-provided canonical JSON against the current
// schema and recomputes dependency diagnostics/graph. Client-supplied graph,
// valid flags, issues and executable fields are not accepted.
func ParseCanonicalJSON(raw, verifiedProjectID string) (Manifest, error) {
	if len(raw) > MaxMarkdownBytes {
		return Manifest{}, errors.New("canonical manifest exceeds 1 MiB limit")
	}
	if !utf8.ValidString(raw) {
		return Manifest{}, errors.New("canonical manifest must be valid UTF-8")
	}
	if strings.TrimSpace(verifiedProjectID) == "" {
		return Manifest{}, errors.New("verified project ID required")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var in canonicalInput
	if err := dec.Decode(&in); err != nil {
		return Manifest{}, fmt.Errorf("invalid canonical manifest: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("exactly one JSON object required")
	}
	if in.SchemaVersion != SchemaVersion {
		return Manifest{}, errors.New("unsupported manifest schema version")
	}
	if in.ProjectID != verifiedProjectID {
		return Manifest{}, errors.New("manifest project_id does not match the active project")
	}
	if in.Name == "" || len(in.Name) > MaxTaskTitle {
		return Manifest{}, errors.New("plan name must contain 1–200 characters")
	}
	if len(in.Tasks) > MaxTasks {
		return Manifest{}, fmt.Errorf("task count exceeds %d limit", MaxTasks)
	}
	sum := sha256.Sum256([]byte(raw))
	manifest := Manifest{
		SchemaVersion: SchemaVersion, ManifestID: "plan-" + hex.EncodeToString(sum[:12]),
		Name: strings.TrimSpace(in.Name), ProjectID: verifiedProjectID,
		SourceSHA256: hex.EncodeToString(sum[:]), Tasks: append([]Task(nil), in.Tasks...),
		Issues: []Issue{}, PlanningOnly: true,
	}
	if in.ManifestID != "" && in.ManifestID != manifest.ManifestID {
		manifest.Issues = append(manifest.Issues, Issue{Code: "manifest_id_mismatch", Message: "manifest_id is derived from canonical content and does not match"})
	}
	ids := make(map[string]int, len(in.Tasks))
	raws := make([]rawTask, len(in.Tasks))
	for i := range manifest.Tasks {
		task := &manifest.Tasks[i]
		if !taskIDPattern.MatchString(task.ID) {
			manifest.Issues = append(manifest.Issues, Issue{Code: "invalid_task_id", Message: "task ID contains unsupported characters", Line: task.SourceLine, TaskID: task.ID})
		}
		if task.Title == "" || len(task.Title) > MaxTaskTitle {
			manifest.Issues = append(manifest.Issues, Issue{Code: "invalid_title", Message: "task title must contain 1–200 characters", Line: task.SourceLine, TaskID: task.ID})
		}
		if len(task.Description) > MaxTaskTextBytes {
			manifest.Issues = append(manifest.Issues, Issue{Code: "task_text_too_large", Message: "task description exceeds 32 KiB", Line: task.SourceLine, TaskID: task.ID})
		}
		if first, ok := ids[task.ID]; ok {
			manifest.Issues = append(manifest.Issues, Issue{Code: "duplicate_id", Message: fmt.Sprintf("task ID duplicates item %d", first+1), Line: task.SourceLine, TaskID: task.ID})
		} else {
			ids[task.ID] = i
		}
		if task.Dependencies == nil {
			task.Dependencies = []string{}
		}
		if task.Acceptance == nil {
			task.Acceptance = []string{}
		}
		if len(task.Dependencies) > MaxDependencies {
			manifest.Issues = append(manifest.Issues, Issue{Code: "too_many_dependencies", Message: "task has more than 64 dependencies", Line: task.SourceLine, TaskID: task.ID})
		}
		if len(task.Acceptance) == 0 {
			manifest.Issues = append(manifest.Issues, Issue{Code: "missing_acceptance", Message: "task has no acceptance criteria", Line: task.SourceLine, TaskID: task.ID, Warning: true})
		}
		raws[i] = rawTask{Task: *task, DependencyLine: task.SourceLine}
	}
	for i := range manifest.Tasks {
		task := &manifest.Tasks[i]
		seen := map[string]bool{}
		for _, dep := range task.Dependencies {
			if dep == task.ID {
				manifest.Issues = append(manifest.Issues, Issue{Code: "self_dependency", Message: "task cannot depend on itself", Line: task.SourceLine, TaskID: task.ID})
				continue
			}
			if seen[dep] {
				manifest.Issues = append(manifest.Issues, Issue{Code: "duplicate_dependency", Message: "dependency is listed more than once", Line: task.SourceLine, TaskID: task.ID})
				continue
			}
			seen[dep] = true
			if _, ok := ids[dep]; !ok {
				manifest.Issues = append(manifest.Issues, Issue{Code: "unknown_dependency", Message: "dependency does not name a task in this manifest", Line: task.SourceLine, TaskID: task.ID})
			}
		}
	}
	manifest.Graph = buildGraph(manifest.Tasks, ids, &manifest.Issues, raws)
	manifest.Valid = true
	for _, issue := range manifest.Issues {
		if !issue.Warning {
			manifest.Valid = false
			break
		}
	}
	return manifest, nil
}
