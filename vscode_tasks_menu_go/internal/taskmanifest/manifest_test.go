package taskmanifest

import (
	"strings"
	"testing"
)

func TestParseMarkdownCanonicalGraphAndAcceptance(t *testing.T) {
	md := `# Dashboard

## Task A — Backend API
Description of backend.
Depends on: None
Acceptance:
- Endpoint returns data
- Unit tests pass

## Task B — Frontend
Depends on: A
Acceptance:
- UI calls API

## Task C — Integration
Depends on: A, B
Acceptance:
- Integration test passes
`
	m, err := ParseMarkdown(md, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Valid || !m.PlanningOnly || m.SchemaVersion != 1 || m.Name != "Dashboard" || len(m.Tasks) != 3 {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if strings.Join(m.Graph.Roots, ",") != "A" || strings.Join(m.Graph.Blocked, ",") != "B,C" {
		t.Fatalf("unexpected roots/blocked: %+v", m.Graph)
	}
	if strings.Join(m.Graph.Topological, ",") != "A,B,C" {
		t.Fatalf("topological order: %v", m.Graph.Topological)
	}
	if m.Tasks[0].Title != "Backend API" || len(m.Tasks[0].Acceptance) != 2 || !strings.Contains(m.Tasks[0].Description, "Description") {
		t.Fatalf("task fields not parsed: %+v", m.Tasks[0])
	}
	if !strings.HasPrefix(m.ManifestID, "plan-") || len(m.SourceSHA256) != 64 {
		t.Fatalf("missing stable identity: %+v", m)
	}
	again, err := ParseMarkdown(md, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if again.ManifestID != m.ManifestID {
		t.Fatal("manifest ID is not deterministic")
	}
}

func TestParseMarkdownReportsDuplicateUnknownSelfAndCycles(t *testing.T) {
	md := `# Plan
## Task A — Alpha
Depends on: A
Acceptance:
- ready
## Task A — Duplicate
Depends on: Z
Acceptance:
- ready
## Task B — Beta
Depends on: C
Acceptance:
- ready
## Task C — Gamma
Depends on: B
Acceptance:
- ready
`
	m, err := ParseMarkdown(md, "p")
	if err != nil {
		t.Fatal(err)
	}
	if m.Valid {
		t.Fatal("invalid graph marked valid")
	}
	codes := map[string]bool{}
	for _, issue := range m.Issues {
		codes[issue.Code] = true
	}
	for _, code := range []string{"self_dependency", "duplicate_id", "unknown_dependency", "dependency_cycle"} {
		if !codes[code] {
			t.Errorf("missing diagnostic %s: %+v", code, m.Issues)
		}
	}
	if len(m.Graph.Cycles) == 0 || m.Graph.Topological != nil {
		t.Fatalf("cycle analysis missing: %+v", m.Graph)
	}
}

func TestParseMarkdownBoundsAndWarnings(t *testing.T) {
	if _, err := ParseMarkdown(strings.Repeat("x", MaxMarkdownBytes+1), "p"); err == nil {
		t.Fatal("oversize markdown accepted")
	}
	if _, err := ParseMarkdown("# plan\n## Task A — a\nAcceptance:\n- pass\n", ""); err == nil {
		t.Fatal("missing verified project ID accepted")
	}
	m, err := ParseMarkdown("# plan\n## Task A — a\nDescription only\n", "p")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Valid || len(m.Issues) != 1 || !m.Issues[0].Warning || m.Issues[0].Code != "missing_acceptance" {
		t.Fatalf("missing acceptance should be a warning: %+v", m)
	}
}

func TestParseMarkdownTaskCountLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("# plan\n")
	for i := 0; i < MaxTasks+1; i++ {
		b.WriteString("\n## Task T")
		b.WriteString(SlugID(string(rune('A' + i))))
		b.WriteString(" — title\nAcceptance:\n- ok\n")
	}
	if _, err := ParseMarkdown(b.String(), "p"); err == nil {
		t.Fatal("too many tasks accepted")
	}
}
