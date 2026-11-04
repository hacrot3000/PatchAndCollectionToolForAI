package server

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/tasks"
)

func remoteTaskSnapshotFixture(t *testing.T, document string) (string, string, []tasks.Task, [32]byte) {
    t.Helper()
    workspace := t.TempDir()
    dir := filepath.Join(workspace, ".vscode")
    if err := os.Mkdir(dir, 0700); err != nil { t.Fatal(err) }
    source := filepath.Join(dir, "tasks.json")
    if err := os.WriteFile(source, []byte(document), 0600); err != nil { t.Fatal(err) }
    list, err := tasks.Load(workspace)
    if err != nil { t.Fatal(err) }
    canonical, err := filepath.EvalSymlinks(workspace)
    if err != nil { t.Fatal(err) }
    canonical, err = filepath.Abs(canonical)
    if err != nil { t.Fatal(err) }
    return workspace, source, list, remoteTaskSourceDigest(canonical, []byte(document))
}

func TestPrepareRemoteTaskExecutionFromAuthenticatedSnapshot(t *testing.T) {
    workspace, path, list, digest := remoteTaskSnapshotFixture(t,
        `{"version":"2.0.0","tasks":[{"label":"Build","type":"process","command":"printf","args":["ok"]}]}`)
    if len(list) != 1 { t.Fatalf("got %d tasks", len(list)) }
    spec, err := prepareRemoteTaskExecutionFromSnapshot(context.Background(), workspace, digest, list[0].ID)
    if err != nil || spec.TaskID != list[0].ID ||
        spec.Command != "printf" || len(spec.Args) != 1 || spec.Args[0] != "ok" {
        t.Fatalf("expected exact host-owned candidate, got %+v, %v", spec, err)
    }
    if _, err := prepareRemoteTaskExecutionFromSnapshot(context.Background(), workspace, [32]byte{}, list[0].ID); err == nil {
        t.Fatal("missing authenticated snapshot must deny")
    }
    if _, err := prepareRemoteTaskExecutionFromSnapshot(context.Background(), workspace, digest, 0); err == nil {
        t.Fatal("invalid ID must deny")
    }
    cancelled, cancel := context.WithCancel(context.Background())
    cancel()
    if _, err := prepareRemoteTaskExecutionFromSnapshot(cancelled, workspace, digest, list[0].ID); err == nil {
        t.Fatal("cancelled task preparation must deny")
    }
    // A stable ID cannot select an old task after its source changes.
    if err := os.WriteFile(path,
        []byte(`{"tasks":[{"label":"Build","type":"process","command":"printf","args":["modified"]}]}`), 0600); err != nil {
        t.Fatal(err)
    }
    if _, err := prepareRemoteTaskExecutionFromSnapshot(context.Background(), workspace, digest, list[0].ID); err == nil {
        t.Fatal("changed tasks.json must deny")
    }
}

func TestPrepareRemoteTaskExecutionDeniesUnsupportedAndAmbiguousTasks(t *testing.T) {
    cases := map[string]string{
        "input": `{"inputs":[{"id":"which","type":"promptString"}],"tasks":[{"label":"Prompt","command":"echo ${input:which}"}]}`,
        "secret": `{"tasks":[{"label":"Secret","command":"echo ok","taskdeckSecrets":{"ACCESS_TOKEN":"secret-id"}}]}`,
        "dependency": `{"tasks":[{"label":"Build","command":"echo ok","dependsOn":"Previous"}]}`,
        "duplicate": `{"tasks":[{"label":"Same","command":"echo ok"},{"label":"Same","command":"echo ok"}]}`,
        "hidden-label": `{"tasks":[{"label":"Invisible\u202e","command":"echo ok"}]}`,
    }
    for name, document := range cases {
        t.Run(name, func(t *testing.T) {
            workspace, _, list, digest := remoteTaskSnapshotFixture(t, document)
            if len(list) == 0 { t.Fatal("fixture must contain task") }
            if _, err := prepareRemoteTaskExecutionFromSnapshot(context.Background(), workspace, digest, list[0].ID); err == nil {
                t.Fatal("unreviewed task execution candidate must deny")
            }
        })
    }
}

func TestPrepareRemoteTaskExecutionRejectsSymlinkedTaskFile(t *testing.T) {
    workspace, source, list, digest := remoteTaskSnapshotFixture(t,
        `{"tasks":[{"label":"Check","command":"echo ok"}]}`)
    other := filepath.Join(t.TempDir(), "tasks.json")
    data, err := os.ReadFile(source)
    if err != nil { t.Fatal(err) }
    if err := os.WriteFile(other, data, 0600); err != nil { t.Fatal(err) }
    if err := os.Remove(source); err != nil { t.Fatal(err) }
    if err := os.Symlink(other, source); err != nil { t.Skipf("symlink not supported: %v", err) }
    if _, err := prepareRemoteTaskExecutionFromSnapshot(context.Background(), workspace, digest, list[0].ID); err == nil {
        t.Fatal("a matching-hash symlinked task file must deny")
    }
}

func TestRemoteTaskSourceDigestBindsPathAndExactContent(t *testing.T) {
    workspace, _, _, digest := remoteTaskSnapshotFixture(t,
        `{"tasks":[{"label":"A","command":"echo ok"}]}`)
    source := []byte(`{"tasks":[{"label":"A","command":"echo ok"}]}`)
    canonical, err := filepath.EvalSymlinks(workspace)
    if err != nil { t.Fatal(err) }
    canonical, err = filepath.Abs(canonical)
    if err != nil { t.Fatal(err) }
    if got := remoteTaskSourceDigest(canonical, source); got != digest {
        t.Fatal("task plan digest must match preview's canonical workspace commitment")
    }
    if remoteTaskSourceDigest(canonical+"-other", source) == digest {
        t.Fatal("same task file in a different workspace must not replay")
    }
    if remoteTaskSourceDigest(canonical, append(append([]byte{}, source...), ' ')) == digest {
        t.Fatal("even whitespace-only task source edits must invalidate selection")
    }
}
