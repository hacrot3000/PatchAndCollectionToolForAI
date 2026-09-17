package state

import "testing"

func TestRemoveIfPIDPreservesReplacementDaemonState(t *testing.T) {
	workspace := t.TempDir()
	old := State{PID: 1001, Workspace: workspace, URL: "http://127.0.0.1:1001", HealthURL: "http://127.0.0.1:1001", Address: "127.0.0.1:1001"}
	if err := Save(old); err != nil { t.Fatal(err) }
	RemoveIfPID(workspace, 999)
	if got, err := Load(workspace); err != nil || got.PID != old.PID {
		t.Fatalf("unrelated pid removed state: %#v err=%v", got, err)
	}
	replacement := old
	replacement.PID = 2002
	if err := Save(replacement); err != nil { t.Fatal(err) }
	RemoveIfPID(workspace, old.PID)
	if got, err := Load(workspace); err != nil || got.PID != replacement.PID {
		t.Fatalf("old daemon removed replacement state: %#v err=%v", got, err)
	}
	RemoveIfPID(workspace, replacement.PID)
	if _, err := Load(workspace); err == nil {
		t.Fatal("matching daemon pid did not remove state")
	}
}
