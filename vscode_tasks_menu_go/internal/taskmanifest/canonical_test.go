package taskmanifest

import (
	"strings"
	"testing"
)

func TestParseCanonicalJSONValidatesSchemaProjectAndGraph(t *testing.T) {
	raw := `{"schema_version":1,"name":"plan","project_id":"p1","tasks":[{"id":"A","title":"alpha","dependencies":[],"acceptance_criteria":["ok"],"source_line":1},{"id":"B","title":"beta","dependencies":["A"],"acceptance_criteria":["pass"],"source_line":2}]}`
	m, err := ParseCanonicalJSON(raw, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Valid || !m.PlanningOnly || strings.Join(m.Graph.Topological, ",") != "A,B" {
		t.Fatalf("unexpected canonical manifest: %+v", m)
	}
	if _, err := ParseCanonicalJSON(raw, "other"); err == nil {
		t.Fatal("cross-project manifest accepted")
	}
}

func TestParseCanonicalJSONRejectsUnknownFieldsVersionAndTrailingJSON(t *testing.T) {
	base := `{"schema_version":1,"name":"p","project_id":"p1","tasks":[]}`
	for _, raw := range []string{
		strings.Replace(base, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(base, `"tasks":[]`, `"tasks":[],"run_all":true`, 1),
		base + ` {}`,
	} {
		if _, err := ParseCanonicalJSON(raw, "p1"); err == nil {
			t.Errorf("accepted invalid canonical input: %s", raw)
		}
	}
}
