package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProtocolPlanSnapshotRetainsOnlyValidatedProjection(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	line := []byte(`{
		"protocol":"taskdeck.patch","version":1,"type":"plan_snapshot","seq":2,
		"status":"ready","failure_policy":"continue_independent","transaction_policy":"patch",
		"items":[{"index":1,"name":"demo.zip","patch_id":"patch.demo","package_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","target_count":2,"depends_on":["patch.base"],"id_reuse_count":1,"raw_internal":"drop"}],
		"previous_failure_action":{"action":"retry_before","reason":"related predecessor","private":"drop"},
		"static_conflicts":[{"left":"demo.zip","left_patch_id":"patch.demo","right":"other.zip","right_patch_id":"patch.other","relation":"order_dependent_overlap","dependency_ordered":false,"overlap":["src/a.ts"],"private":"drop"}],
		"resources":{"status":"PASS","actual_project_free_bytes":1000,"required_project_free_bytes":100,"actual_temp_free_bytes":900,"required_temp_free_bytes":50,"private":"drop"},
		"previews":[{"name":"demo.zip","status":"PASS","rc":0,"stage":"preview","diagnosis_kind":"ready_to_apply","message":"project unchanged","target_count":2,"private":"drop"}],
		"warnings":["notice"],"raw_planner":{"secret":true}
	}`)
	s.applyProtocolLine(line)
	if s.protocol.Error != "" || s.protocol.PlanSnapshot == nil {
		t.Fatalf("valid Plan snapshot rejected: %#v", s.protocol)
	}
	got := s.protocol.PlanSnapshot
	if got.Status != "ready" || len(got.Items) != 1 || len(got.StaticConflicts) != 1 || len(got.Previews) != 1 {
		t.Fatalf("unexpected Plan projection: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil { t.Fatal(err) }
	for _, forbidden := range []string{"raw_internal","raw_planner",`\"private\"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("Plan state leaked unknown planner field %q: %s", forbidden, encoded)
		}
	}
}

func TestProtocolPlanSnapshotRejectsInvalidBoundsAndPolicies(t *testing.T) {
	cases := []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"plan_snapshot","seq":1,"status":"other","failure_policy":"continue_independent","transaction_policy":"patch"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"plan_snapshot","seq":1,"status":"ready","failure_policy":"other","transaction_policy":"patch"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"plan_snapshot","seq":1,"status":"ready","failure_policy":"continue_independent","transaction_policy":"other"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"plan_snapshot","seq":1,"status":"ready","failure_policy":"continue_independent","transaction_policy":"patch","items":[{"index":0,"name":"x","patch_id":"id","target_count":0,"id_reuse_count":0}]}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"plan_snapshot","seq":1,"status":"ready","failure_policy":"continue_independent","transaction_policy":"patch","resources":{"status":"PASS","actual_project_free_bytes":-1}}`,
	}
	for _, line := range cases {
		s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.PlanSnapshot != nil || s.protocol.Error == "" {
			t.Fatalf("invalid Plan snapshot accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolPlanSnapshotCloneIsIndependent(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{
		PlanSnapshot: &ProtocolPlanSnapshotState{
			Status:"ready", FailurePolicy:"continue_independent", TransactionPolicy:"patch",
			Items:[]ProtocolPlanItemState{{Index:1,Name:"a.zip",PatchID:"a",DependsOn:[]string{"base"}}},
			StaticConflicts:[]ProtocolPlanConflictState{{Left:"a.zip",Right:"b.zip",Relation:"order_dependent_overlap",Overlap:[]string{"src/a"}}},
			Warnings:[]string{"one"},
		},
	}}
	clone := cloneProtocolState(s.protocol)
	clone.PlanSnapshot.Items[0].DependsOn[0] = "changed"
	clone.PlanSnapshot.StaticConflicts[0].Overlap[0] = "changed"
	clone.PlanSnapshot.Warnings[0] = "changed"
	if s.protocol.PlanSnapshot.Items[0].DependsOn[0] != "base" ||
		s.protocol.PlanSnapshot.StaticConflicts[0].Overlap[0] != "src/a" ||
		s.protocol.PlanSnapshot.Warnings[0] != "one" {
		t.Fatal("Plan snapshot clone shares nested slices")
	}
}
