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

func TestPrepareRemoteTaskPickChoiceFromHostSnapshot(t *testing.T){
 source:=`{
  "inputs":[{"id":"flavor","type":"pickString","options":["debug","release"]}],
  "tasks":[{"label":"Param","type":"process","command":"printf","args":["${input:flavor}"]}]
 }`
 workspace,_,list,hash:=remoteTaskSnapshotFixture(t,source)
 if len(list)!=1{t.Fatal("expected a host task")}
 if _,err:=prepareRemoteTaskExecutionFromSnapshot(context.Background(),workspace,hash,list[0].ID);err==nil{
  t.Fatal("a task requiring a choice ran with no selected value")
 }
 inspection,err:=inspectRemoteTaskSelection(context.Background(),workspace,hash,list[0].ID)
 if err!=nil||!inspection.Eligible||len(inspection.Choices)!=2{
  t.Fatalf("fixed-choice task could not be inspected: %+v %v",inspection,err)
 }
 spec,_,err:=prepareRemoteTaskExecutionChoice(context.Background(),workspace,hash,list[0].ID,2,false)
 if err!=nil||len(spec.Args)!=1||spec.Args[0]!="release"{
  t.Fatalf("host choice was not resolved correctly: %+v %v",spec,err)
 }
 if _,_,err:=prepareRemoteTaskExecutionChoice(context.Background(),workspace,hash,list[0].ID,3,false);err==nil{
  t.Fatal("out-of-range choice became an arbitrary argument")
 }
}

func TestRemoteTaskPreparationRejectsReplacedVSCodeDirectoryEvenWithSameFileInode(t *testing.T){
 workspace,source,_,_:=remoteTaskSnapshotFixture(t,
  `{"tasks":[{"label":"One","type":"process","command":"/bin/echo","args":["OK"]}]}`)
 vscodeDir:=filepath.Join(workspace,".vscode")
 rootBefore,err:=os.Stat(workspace)
 if err!=nil{t.Fatal(err)}
 vscodeBefore,err:=os.Lstat(vscodeDir)
 if err!=nil{t.Fatal(err)}
 if !remoteTaskParentsStable(workspace,vscodeDir,rootBefore,vscodeBefore){
  t.Fatal("unchanged task parent directories rejected")
 }
 oldDir:=filepath.Join(workspace,".vscode-original")
 if err:=os.Rename(vscodeDir,oldDir);err!=nil{t.Fatal(err)}
 if err:=os.Mkdir(vscodeDir,0700);err!=nil{t.Fatal(err)}
 // Hard link the original task source into the replacement .vscode so
 // a naive os.SameFile(tasks.json) check still returns true.
 if err:=os.Link(filepath.Join(oldDir,"tasks.json"),source);err!=nil{t.Fatal(err)}
 before,err:=os.Stat(filepath.Join(oldDir,"tasks.json"))
 if err!=nil{t.Fatal(err)}
 after,err:=os.Stat(source)
 if err!=nil{t.Fatal(err)}
 if !os.SameFile(before,after){t.Fatal("hard link fixture lost file inode")}
 if remoteTaskParentsStable(workspace,vscodeDir,rootBefore,vscodeBefore){
  t.Fatal("replaced .vscode accepted despite matching task file inode")
 }
}
func TestRemoteTaskPreparationRejectsReplacedWorkspaceRoot(t *testing.T){
 workspace,_,_,_:=remoteTaskSnapshotFixture(t,
  `{"tasks":[{"label":"One","type":"process","command":"/bin/echo"}]}`)
 vscodeDir:=filepath.Join(workspace,".vscode")
 rootBefore,err:=os.Stat(workspace)
 if err!=nil{t.Fatal(err)}
 vscodeBefore,err:=os.Lstat(vscodeDir)
 if err!=nil{t.Fatal(err)}
 original:=workspace+"-original"
 if err:=os.Rename(workspace,original);err!=nil{t.Fatal(err)}
 if err:=os.Mkdir(workspace,0700);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(filepath.Join(original,".vscode"),vscodeDir);err!=nil{t.Fatal(err)}
 if remoteTaskParentsStable(workspace,vscodeDir,rootBefore,vscodeBefore){
  t.Fatal("replaced workspace root/symlink parent accepted")
 }
}
