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
type remoteTaskInputSelection struct {
 First tasks.Input
 Second tasks.Input
}

func inspectRemoteTaskSelection(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
) (remotetransport.RemoteTaskInspect,error) {
    spec, input, err := prepareRemoteTaskExecutionChoice(
        ctx,workspace,expected,taskID,0,true)
    if err!=nil {return remotetransport.RemoteTaskInspect{},err}
    return remotetransport.RemoteTaskInspect{
        TaskID:spec.TaskID,Eligible:true,InputID:input.First.ID,
        Choices:input.First.Options,
        InputType:func()string{
            if input.First.Type=="promptString"{return "promptString"}
            return ""
        }(),
        InputID2:input.Second.ID,Choices2:input.Second.Options,
    },nil
}

// Only a single host-authored pickString is allowed. Its value never
// traverses the untrusted relay: the encrypted client requests an index.
// Options and the exact source are reloaded before every run.
func prepareRemoteTaskExecutionChoice(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
    choice int, inspectOnly bool,
) (tasks.Execution,remoteTaskInputSelection,error) {
    return prepareRemoteTaskExecutionInput(ctx,workspace,expected,taskID,choice,inspectOnly,"")
}

// The client never chooses an executable, cwd, environment variable name
// or arbitrary shell command. For promptString it may provide only one
// strictly validated token to a host-authored process argv template.
func prepareRemoteTaskExecutionInput(
    ctx context.Context, workspace string, expected [32]byte, taskID int,
    choice int, inspectOnly bool, textInput string,
) (tasks.Execution,remoteTaskInputSelection,error) {
    denied := remotetransport.ErrRemoteActionDenied
    if ctx == nil || ctx.Err() != nil || workspace == "" ||
        taskID < 1 || taskID > 0x7fffffff || expected == ([32]byte{}) {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    root, err := filepath.EvalSymlinks(workspace)
    if err != nil { return tasks.Execution{},remoteTaskInputSelection{},denied }
    root, err = filepath.Abs(root)
    if err != nil { return tasks.Execution{},remoteTaskInputSelection{},denied }
    dirInfo, err := os.Stat(root)
    if err != nil || !dirInfo.IsDir() { return tasks.Execution{},remoteTaskInputSelection{},denied }
    vscodeDir := filepath.Join(root, ".vscode")
    vscodeInfo, err := os.Lstat(vscodeDir)
    if err != nil || !vscodeInfo.IsDir() ||
        vscodeInfo.Mode()&os.ModeSymlink != 0 {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    filename := filepath.Join(vscodeDir, "tasks.json")
    beforeInfo, err := os.Lstat(filename)
    if err != nil || !beforeInfo.Mode().IsRegular() ||
        beforeInfo.Size() < 2 ||
        beforeInfo.Size() > maxRemoteTasksPreviewSourceBytes {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    before, err := os.ReadFile(filename)
    if err != nil || len(before) < 2 ||
        len(before) > maxRemoteTasksPreviewSourceBytes ||
        remoteTaskSourceDigest(root, before) != expected ||
        ctx.Err() != nil {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    list, err := tasks.Load(root)
    if err != nil || len(list) > remotetransport.MaxRemoteTasksCount ||
        ctx.Err() != nil {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    // Stable IDs are hashes of definitions, not of list position. Reject
    // ambiguous duplicate definitions even if tasks.Load permits them.
    var candidate *tasks.Task
    for i := range list {
        if list[i].ID == taskID {
            if candidate != nil { return tasks.Execution{},remoteTaskInputSelection{},denied }
            candidate = &list[i]
        }
    }
    if candidate == nil ||
        safeRemoteTaskPreviewLabel(candidate.MenuLabel) == "" ||
        len(candidate.Inputs) > 2 ||
        len(candidate.SecretEnv) != 0 ||
        candidate.Raw["dependsOn"] != nil {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    var picked remoteTaskInputSelection
    var values map[string]string
    if len(candidate.Inputs)==2 {
        first,second:=candidate.Inputs[0],candidate.Inputs[1]
        if textInput!=""||first.ID==second.ID||
           first.Type!="pickString"||second.Type!="pickString"||
           !remotetransport.ValidRemoteTaskPickInput(first.ID,first.Options)||
           !remotetransport.ValidRemoteTaskPickInput(second.ID,second.Options){
           return tasks.Execution{},remoteTaskInputSelection{},denied
        }
        total:=len(first.Options)*len(second.Options)
        if inspectOnly {
            if choice!=0{return tasks.Execution{},remoteTaskInputSelection{},denied}
            choice=1
        }else if choice<1||choice>total{
            return tasks.Execution{},remoteTaskInputSelection{},denied
        }
        // One 1-based authenticated index encodes a row-major ordered
        // pair; no client input values or arbitrary arguments are trusted.
        firstIndex:=(choice-1)/len(second.Options)
        secondIndex:=(choice-1)%len(second.Options)
        values=map[string]string{
            first.ID:first.Options[firstIndex],
            second.ID:second.Options[secondIndex],
        }
        picked.First=first
        picked.Second=second
    }else if len(candidate.Inputs)==1 {
        picked.First=candidate.Inputs[0]
        single:=picked.First
        switch single.Type {
        case "pickString":
            if textInput!=""||
                !remotetransport.ValidRemoteTaskPickInput(single.ID,single.Options) {
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            if inspectOnly {
                if choice!=0{return tasks.Execution{},remoteTaskInputSelection{},denied}
                choice=1 // inspect with the first fixed choice, no execution
            } else if choice<1||choice>len(single.Options) {
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            values=map[string]string{single.ID:single.Options[choice-1]}
        case "promptString":
            // No shell, interpreter, cwd/env expansion, or executable
            // interpolation may be influenced by a remotely typed token.
            // The host still owns the process executable and argument list.
            if choice!=0 || !remotetransport.ValidRemoteTaskInputID(single.ID) ||
               len(single.Options)!=0 || candidate.Type!="process" {
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            executable,ok:=candidate.Command.(string)
            if !ok||strings.Contains(executable,"${input:") {
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            switch strings.ToLower(filepath.Base(executable)) {
            case "sh","bash","dash","ksh","zsh","fish","csh","tcsh",
                 "pwsh","powershell":
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            optionJSON,err:=json.Marshal(candidate.Options)
            if err!=nil||bytes.Contains(optionJSON,[]byte("${input:")){
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            argsJSON,err:=json.Marshal(candidate.Args)
            if err!=nil||!bytes.Contains(argsJSON,[]byte("${input:"+single.ID+"}")){
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            if inspectOnly {
                if textInput!=""{return tasks.Execution{},remoteTaskInputSelection{},denied}
                textInput="TaskDeckPreview" // never executed on inspection
            } else if !remotetransport.ValidRemoteTaskPromptValue(textInput) {
                return tasks.Execution{},remoteTaskInputSelection{},denied
            }
            values=map[string]string{single.ID:textInput}
        default:
            return tasks.Execution{},remoteTaskInputSelection{},denied
        }
    } else if choice!=0 || textInput!="" {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    // The host chooses the exact approved value. No raw strings/arguments
    // are accepted from the remote client.
    execution, err := tasks.ResolveExecutionWithInputs(*candidate,root,values)
    if err != nil || execution.TaskID != taskID || ctx.Err() != nil {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    after, err := os.ReadFile(filename)
    if err != nil || !bytes.Equal(before, after) ||
        remoteTaskSourceDigest(root, after) != expected {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    afterInfo, err := os.Lstat(filename)
    if err != nil || !afterInfo.Mode().IsRegular() ||
        !os.SameFile(beforeInfo, afterInfo) ||
        ctx.Err() != nil {
        return tasks.Execution{},remoteTaskInputSelection{},denied
    }
    return execution,picked,nil
}
