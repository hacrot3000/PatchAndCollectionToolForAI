package server

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/json"
    "os"
    "strings"
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
    spec, _, err := prepareRemoteTaskExecutionChoice(ctx,workspace,expected,taskID,0,false)
    return spec,err
}

// inspectRemoteTaskSelection binds a one-choice task metadata response to
// the same fully verified source as the run preflight. A positive reply is
// NOT an execution grant. The native client may only select an index.
func inspectRemoteTaskSelection(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
) (remotetransport.RemoteTaskInspect,error) {
    spec, input, err := prepareRemoteTaskExecutionChoice(
        ctx,workspace,expected,taskID,0,true)
    if err!=nil {return remotetransport.RemoteTaskInspect{},err}
    return remotetransport.RemoteTaskInspect{
        TaskID:spec.TaskID,Eligible:true,InputID:input.ID,Choices:input.Options,
        InputType:func()string{
            if input.Type=="promptString"{return "promptString"}
            return ""
        }(),
    },nil
}

// Only a single host-authored pickString is allowed. Its value never
// traverses the untrusted relay: the encrypted client requests an index.
// Options and the exact source are reloaded before every run.
func prepareRemoteTaskExecutionChoice(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
    choice int, inspectOnly bool,
) (tasks.Execution,tasks.Input,error) {
    return prepareRemoteTaskExecutionInput(ctx,workspace,expected,taskID,choice,inspectOnly,"")
}

// The client never chooses an executable, cwd, environment variable name
// or arbitrary shell command. For promptString it may provide only one
// strictly validated token to a host-authored process argv template.
func prepareRemoteTaskExecutionInput(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
    choice int, inspectOnly bool, textInput string,
) (tasks.Execution,tasks.Input,error) {
    denied := remotetransport.ErrRemoteActionDenied
    if ctx == nil || ctx.Err() != nil || workspace == "" ||
        taskID < 1 || taskID > 0x7fffffff || expected == ([32]byte{}) {
        return tasks.Execution{},tasks.Input{},denied
    }
    root, err := filepath.EvalSymlinks(workspace)
    if err != nil { return tasks.Execution{},tasks.Input{},denied }
    root, err = filepath.Abs(root)
    if err != nil { return tasks.Execution{},tasks.Input{},denied }
    dirInfo, err := os.Stat(root)
    if err != nil || !dirInfo.IsDir() { return tasks.Execution{},tasks.Input{},denied }
    vscodeDir := filepath.Join(root, ".vscode")
    vscodeInfo, err := os.Lstat(vscodeDir)
    if err != nil || !vscodeInfo.IsDir() ||
        vscodeInfo.Mode()&os.ModeSymlink != 0 {
        return tasks.Execution{},tasks.Input{},denied
    }
    filename := filepath.Join(vscodeDir, "tasks.json")
    beforeInfo, err := os.Lstat(filename)
    if err != nil || !beforeInfo.Mode().IsRegular() ||
        beforeInfo.Size() < 2 ||
        beforeInfo.Size() > maxRemoteTasksPreviewSourceBytes {
        return tasks.Execution{},tasks.Input{},denied
    }
    before, err := os.ReadFile(filename)
    if err != nil || len(before) < 2 ||
        len(before) > maxRemoteTasksPreviewSourceBytes ||
        remoteTaskSourceDigest(root, before) != expected ||
        ctx.Err() != nil {
        return tasks.Execution{},tasks.Input{},denied
    }
    list, err := tasks.Load(root)
    if err != nil || len(list) > remotetransport.MaxRemoteTasksCount ||
        ctx.Err() != nil {
        return tasks.Execution{},tasks.Input{},denied
    }
    // Stable IDs are hashes of definitions, not of list position. Reject
    // ambiguous duplicate definitions even if tasks.Load permits them.
    var candidate *tasks.Task
    for i := range list {
        if list[i].ID == taskID {
            if candidate != nil { return tasks.Execution{},tasks.Input{},denied }
            candidate = &list[i]
        }
    }
    if candidate == nil ||
        safeRemoteTaskPreviewLabel(candidate.MenuLabel) == "" ||
        len(candidate.Inputs) > 1 ||
        len(candidate.SecretEnv) != 0 ||
        candidate.Raw["dependsOn"] != nil {
        return tasks.Execution{},tasks.Input{},denied
    }
    var picked tasks.Input
    var values map[string]string
    if len(candidate.Inputs)==1 {
        picked=candidate.Inputs[0]
        switch picked.Type {
        case "pickString":
            if textInput!=""||
                !remotetransport.ValidRemoteTaskPickInput(picked.ID,picked.Options) {
                return tasks.Execution{},tasks.Input{},denied
            }
            if inspectOnly {
                if choice!=0{return tasks.Execution{},tasks.Input{},denied}
                choice=1 // inspect with the first fixed choice, no execution
            } else if choice<1||choice>len(picked.Options) {
                return tasks.Execution{},tasks.Input{},denied
            }
            values=map[string]string{picked.ID:picked.Options[choice-1]}
        case "promptString":
            // No shell, interpreter, cwd/env expansion, or executable
            // interpolation may be influenced by a remotely typed token.
            // The host still owns the process executable and argument list.
            if choice!=0 || !remotetransport.ValidRemoteTaskInputID(picked.ID) ||
               len(picked.Options)!=0 || candidate.Type!="process" {
                return tasks.Execution{},tasks.Input{},denied
            }
            executable,ok:=candidate.Command.(string)
            if !ok||strings.Contains(executable,"${input:") {
                return tasks.Execution{},tasks.Input{},denied
            }
            switch strings.ToLower(filepath.Base(executable)) {
            case "sh","bash","dash","ksh","zsh","fish","csh","tcsh",
                 "pwsh","powershell":
                return tasks.Execution{},tasks.Input{},denied
            }
            optionJSON,err:=json.Marshal(candidate.Options)
            if err!=nil||bytes.Contains(optionJSON,[]byte("${input:")){
                return tasks.Execution{},tasks.Input{},denied
            }
            argsJSON,err:=json.Marshal(candidate.Args)
            if err!=nil||!bytes.Contains(argsJSON,[]byte("${input:"+picked.ID+"}")){
                return tasks.Execution{},tasks.Input{},denied
            }
            if inspectOnly {
                if textInput!=""{return tasks.Execution{},tasks.Input{},denied}
                textInput="TaskDeckPreview" // never executed on inspection
            } else if !remotetransport.ValidRemoteTaskPromptValue(textInput) {
                return tasks.Execution{},tasks.Input{},denied
            }
            values=map[string]string{picked.ID:textInput}
        default:
            return tasks.Execution{},tasks.Input{},denied
        }
    } else if choice!=0 || textInput!="" {
        return tasks.Execution{},tasks.Input{},denied
    }
    // The host chooses the exact approved value. No raw strings/arguments
    // are accepted from the remote client.
    execution, err := tasks.ResolveExecutionWithInputs(*candidate,root,values)
    if err != nil || execution.TaskID != taskID || ctx.Err() != nil {
        return tasks.Execution{},tasks.Input{},denied
    }
    after, err := os.ReadFile(filename)
    if err != nil || !bytes.Equal(before, after) ||
        remoteTaskSourceDigest(root, after) != expected {
        return tasks.Execution{},tasks.Input{},denied
    }
    afterInfo, err := os.Lstat(filename)
    if err != nil || !afterInfo.Mode().IsRegular() ||
        !os.SameFile(beforeInfo, afterInfo) ||
        ctx.Err() != nil {
        return tasks.Execution{},tasks.Input{},denied
    }
    return execution,picked,nil
}
