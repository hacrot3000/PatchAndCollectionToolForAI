package server

import (
    "bytes"
    "context"
    "crypto/sha256"
    "os"
    "path/filepath"

    "bletonfc/vscode_tasks_menu/internal/remotetransport"
    "bletonfc/vscode_tasks_menu/internal/tasks"
)

// remoteTaskSourceDigest matches the private host-owned pagination commitment.
// It binds the exact canonical workspace AND bytes of .vscode/tasks.json.
// Never send this digest, workspace path or an execution specification to
// PHP/Relay or an unauthenticated client.
func remoteTaskSourceDigest(root string, source []byte) [32]byte {
    h := sha256.New()
    _, _ = h.Write([]byte(root))
    _, _ = h.Write([]byte{0})
    _, _ = h.Write(source)
    var result [32]byte
    copy(result[:], h.Sum(nil))
    return result
}

// prepareRemoteTaskExecutionFromSnapshot is a HOST-ONLY staged prerequisite
// for a future explicit task.run action. It does NOT start a process or grant
// permission. A future caller MUST first authenticate an approved software
// key and TaskDeck password, check tasks.run + live Portal grant, obtain the
// exact selection from authenticated E2EE, and arrange cancellation/audit.
//
// The only selection input is the stable host-created numeric Task.ID.
// Reusing a stale preview after tasks.json or the workspace changes is
// forbidden. We intentionally do NOT support parameterized, secret-bearing
// or dependency tasks until those flows receive a separate review.
func prepareRemoteTaskExecutionFromSnapshot(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
) (tasks.Execution, error) {
    denied := remotetransport.ErrRemoteActionDenied
    if ctx == nil || ctx.Err() != nil || workspace == "" ||
        taskID < 1 || taskID > 0x7fffffff || expected == ([32]byte{}) {
        return tasks.Execution{}, denied
    }
    root, err := filepath.EvalSymlinks(workspace)
    if err != nil { return tasks.Execution{}, denied }
    root, err = filepath.Abs(root)
    if err != nil { return tasks.Execution{}, denied }
    dirInfo, err := os.Stat(root)
    if err != nil || !dirInfo.IsDir() { return tasks.Execution{}, denied }
    vscodeDir := filepath.Join(root, ".vscode")
    vscodeInfo, err := os.Lstat(vscodeDir)
    if err != nil || !vscodeInfo.IsDir() ||
        vscodeInfo.Mode()&os.ModeSymlink != 0 {
        return tasks.Execution{}, denied
    }
    filename := filepath.Join(vscodeDir, "tasks.json")
    beforeInfo, err := os.Lstat(filename)
    if err != nil || !beforeInfo.Mode().IsRegular() ||
        beforeInfo.Size() < 2 ||
        beforeInfo.Size() > maxRemoteTasksPreviewSourceBytes {
        return tasks.Execution{}, denied
    }
    before, err := os.ReadFile(filename)
    if err != nil || len(before) < 2 ||
        len(before) > maxRemoteTasksPreviewSourceBytes ||
        remoteTaskSourceDigest(root, before) != expected ||
        ctx.Err() != nil {
        return tasks.Execution{}, denied
    }
    list, err := tasks.Load(root)
    if err != nil || len(list) > remotetransport.MaxRemoteTasksCount ||
        ctx.Err() != nil {
        return tasks.Execution{}, denied
    }
    // Stable IDs are hashes of definitions, not of list position. Reject
    // ambiguous duplicate definitions even if tasks.Load permits them.
    var candidate *tasks.Task
    for i := range list {
        if list[i].ID == taskID {
            if candidate != nil { return tasks.Execution{}, denied }
            candidate = &list[i]
        }
    }
    if candidate == nil ||
        safeRemoteTaskPreviewLabel(candidate.MenuLabel) == "" ||
        len(candidate.Inputs) != 0 ||
        len(candidate.SecretEnv) != 0 ||
        candidate.Raw["dependsOn"] != nil {
        return tasks.Execution{}, denied
    }
    // Task commands and environment come exclusively from local, trusted
    // workspace definitions. They are deliberately not serialized here.
    execution, err := tasks.ResolveExecution(*candidate, root)
    if err != nil || execution.TaskID != taskID || ctx.Err() != nil {
        return tasks.Execution{}, denied
    }
    after, err := os.ReadFile(filename)
    if err != nil || !bytes.Equal(before, after) ||
        remoteTaskSourceDigest(root, after) != expected {
        return tasks.Execution{}, denied
    }
    afterInfo, err := os.Lstat(filename)
    if err != nil || !afterInfo.Mode().IsRegular() ||
        !os.SameFile(beforeInfo, afterInfo) ||
        ctx.Err() != nil {
        return tasks.Execution{}, denied
    }
    return execution, nil
}
