package agentchat

import (
 "bufio"
 "bytes"
 "context"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

const (
 forcedCrashChildEnv = "TASKDECK_AGENT_TEST_FORCE_CRASH_AFTER_SEGMENT"
 forcedCrashMarker = "TASKDECK_SEGMENT_STAGED_BEFORE_JOURNAL_COMMIT"
)

// Child process exercises the real fsync + rename segment writer, then pauses
// at the pre-journal-commit barrier. The parent sends a real OS kill signal.
// An in-process panic or a manually created orphan cannot test this boundary.
func taskPlanForcedCrashChild() {
 dir := os.Getenv("TASKDECK_AGENT_TEST_CRASH_DIR")
 id := os.Getenv("TASKDECK_AGENT_TEST_CRASH_SESSION")
 store, err := NewStore(dir)
 if err != nil {
  fmt.Fprintln(os.Stderr, err)
  os.Exit(2)
 }
 original, err := store.Get(id, "crash-owner")
 if err != nil || original.TaskPlan == nil {
  fmt.Fprintln(os.Stderr, "load original journal:", err)
  os.Exit(2)
 }
 candidate := original
 copied := *original.TaskPlan
 copied.Steps = append([]TaskPlanStep(nil), original.TaskPlan.Steps...)
 candidate.TaskPlan = &copied
 candidate.TaskPlan.Steps[129].Status = "done"
 candidate.TaskPlan.Steps[129].Evidence = []TaskPlanEvidence{{
  Kind:"manual", Note:"This uncommitted proof must never survive a crash",
 }}
 err = store.withLock(func() error {
  return store.stageTaskPlanSegments(&candidate, &original, original.Revision+1)
 })
 if err != nil {
  fmt.Fprintln(os.Stderr, "stage segment:", err)
  os.Exit(2)
 }
 // The durable page exists. The canonical journal still references only the
 // prior revision and the child MUST NOT call Store.Save.
 fmt.Fprintln(os.Stdout, forcedCrashMarker)
 time.Sleep(30 * time.Second) // parent must kill us before this returns
 os.Exit(3)
}

func TestTaskPlanForcedCrashAfterSegmentFsyncBeforeJournalCommit(t *testing.T) {
 if os.Getenv(forcedCrashChildEnv) == "1" {
  taskPlanForcedCrashChild()
  return
 }
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir)
 if err != nil { t.Fatal(err) }
 original, refs := testRecoveryPlan(t,store,"crash-owner","/workspace","project")
 if len(refs) != 5 { t.Fatalf("expected five plan segments; got %d", len(refs)) }
 before, err := os.ReadFile(store.file(original.ID))
 if err != nil { t.Fatal(err) }

 ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
 defer cancel()
 cmd := exec.CommandContext(ctx, os.Args[0],
  "-test.run=^TestTaskPlanForcedCrashAfterSegmentFsyncBeforeJournalCommit$")
 cmd.Env = append(os.Environ(), forcedCrashChildEnv+"=1",
  "TASKDECK_AGENT_TEST_CRASH_DIR="+dir,
  "TASKDECK_AGENT_TEST_CRASH_SESSION="+original.ID)
 var stderr bytes.Buffer
 cmd.Stderr = &stderr
 stdout, err := cmd.StdoutPipe()
 if err != nil { t.Fatal(err) }
 if err = cmd.Start(); err != nil { t.Fatal(err) }
 defer func() {
  if cmd.ProcessState == nil {
   _ = cmd.Process.Kill()
   _ = cmd.Wait()
  }
 }()

 scanner := bufio.NewScanner(stdout)
 staged := false
 for scanner.Scan() {
  if strings.TrimSpace(scanner.Text()) == forcedCrashMarker {
   staged = true
   break
  }
 }
 if !staged {
  t.Fatalf("child never staged a durable plan segment: scan=%v stderr=%s",
   scanner.Err(), stderr.String())
 }
 orphan := store.taskPlanSegmentPath(original.ID,
  TaskPlanSegmentRef{Revision:original.Revision+1,Index:5})
 if _,err = os.Stat(orphan);err != nil {
  t.Fatalf("child signalled without durable new segment: %v", err)
 }
 if err = cmd.Process.Kill();err != nil { t.Fatal(err) }
 if err = cmd.Wait();err == nil {
  t.Fatal("child exited normally; forced process-kill barrier was not exercised")
 }
 if ctx.Err() != nil {
  t.Fatalf("child exited by deadline rather than an intentional kill: %v", ctx.Err())
 }

 after, err := os.ReadFile(store.file(original.ID))
 if err != nil { t.Fatal(err) }
 if !bytes.Equal(before,after) {
  t.Fatal("crash before journal rename unexpectedly changed canonical bytes")
 }
 restarted, err := NewStore(dir)
 if err != nil { t.Fatal(err) }
 restored, err := restarted.Get(original.ID,"crash-owner")
 if err != nil { t.Fatal(err) }
 if restored.Revision != original.Revision || len(restored.TaskPlan.Steps) != 130 ||
  restored.TaskPlan.Steps[129].Status == "done" ||
  restored.TaskPlan.Steps[0].Evidence[0].Note != "Keep this owner-approved evidence" {
  t.Fatal("process kill lost approved evidence or adopted an uncommitted milestone")
 }
 if err = restarted.EnsureTaskPlanOrphanRecovery();err != nil { t.Fatal(err) }
 if _,err = os.Stat(orphan);!os.IsNotExist(err) {
  t.Fatalf("startup did not reclaim the real killed writer's orphan: %v", err)
 }
 for _,ref := range refs {
  if _,err = os.Stat(restarted.taskPlanSegmentPath(original.ID,ref));err != nil {
   t.Fatalf("startup deleted a committed segment: %v", err)
  }
 }
 latest, err := restarted.Get(original.ID,"crash-owner")
 if err != nil || latest.Revision != original.Revision {
  t.Fatalf("canonical plan unavailable after real process crash recovery: %v", err)
 }
}
