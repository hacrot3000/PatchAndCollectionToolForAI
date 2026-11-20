package server

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io"
    "strings"
    "unicode/utf8"

    "bletonfc/vscode_tasks_menu/internal/agentchat"
    "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Text-only models cannot emit a function_call. A standalone, project-bound
// patch proposal is therefore converted into a LOCAL review item, never an
// executable side effect. Only the ordinary native Patch approval API can run it.
const agentTextPatchProposalType = "taskdeck_patch_proposal"

type agentTextPatchEnvelope struct {
    Type string `json:"type"`
    Project patchtool.ProjectIdentity `json:"project"`
    Name string `json:"name"`
    PatchPython string `json:"patch_python"`
}

func parseAgentTextPatchProposal(raw string, identity patchtool.ProjectIdentity) (agentPatchArguments, error) {
    if len(raw) == 0 || len(raw) > 512<<10 || !utf8.ValidString(raw) {
        return agentPatchArguments{}, errors.New("empty or oversized text Patch proposal")
    }
    // A lone Markdown code fence is allowed; arbitrary prose with an embedded
    // JSON object is NOT allowed to create an approval record.
    raw = unwrapPasteFence(raw)
    if !strings.HasPrefix(raw, "{") || !strings.HasSuffix(raw, "}") {
        return agentPatchArguments{}, errors.New("text Patch proposal must be a standalone JSON object")
    }
    dec := json.NewDecoder(strings.NewReader(raw))
    dec.DisallowUnknownFields()
    var envelope agentTextPatchEnvelope
    if err := dec.Decode(&envelope); err != nil {
        return agentPatchArguments{}, errors.New("invalid text Patch proposal JSON")
    }
    var extra any
    if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
        return agentPatchArguments{}, errors.New("text Patch proposal contains trailing data")
    }
    if envelope.Type != agentTextPatchProposalType || envelope.Project != identity {
        return agentPatchArguments{}, errors.New("text Patch proposal type or project identity mismatch")
    }
    args := agentPatchArguments{Name: strings.TrimSpace(envelope.Name), PatchPython: envelope.PatchPython}
    encoded, err := json.Marshal(args)
    if err != nil { return agentPatchArguments{}, err }
    if _, err := parseAgentPatchArguments(string(encoded)); err != nil {
        return agentPatchArguments{}, err
    }
    // Validate the same identity headers and native cleartext Patch package
    // contract that the approval endpoint will validate again on execution.
    if _, err := pastedPythonPackage(args.PatchPython, identity, patchPasteSlug(args.Name)); err != nil {
        return agentPatchArguments{}, err
    }
    return args, nil
}

func agentHasTextPatchShape(raw string) bool {
    text := strings.TrimSpace(unwrapPasteFence(raw))
    if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") { return false }
    var node struct { Type string `json:"type"` }
    return json.Unmarshal([]byte(text), &node) == nil && node.Type == agentTextPatchProposalType
}

// The provider never emitted this tool call. Mark it explicitly as synthetic
// and exclude it from provider-visible history. It only makes the existing
// per-call approval UI and native execution gateway available for review.
func appendAgentTextPatchProposal(x *agentchat.Session, result agentchat.ResponsesResult, identity patchtool.ProjectIdentity) bool {
    if x == nil || len(result.Calls) != 0 || !agentHasTextPatchShape(result.Text) { return false }
    args, err := parseAgentTextPatchProposal(result.Text, identity)
    if err != nil { return false }
    rawArgs, err := json.Marshal(args)
    if err != nil { return false }
    digest := sha256.Sum256([]byte(x.ID + "\x00" + result.ResponseID + "\x00" + string(rawArgs)))
    callID := "taskdeck_text_" + hex.EncodeToString(digest[:16])
    for _, item := range x.History {
        if item.Type == "function_call" && item.CallID == callID { return false }
    }
    x.History = append(x.History, agentchat.HistoryItem{
        Type: "function_call", CallID: callID, Name: "taskdeck_submit_patch", Arguments: string(rawArgs),
    })
    x.ToolCalls = append(x.ToolCalls, agentchat.ToolCallRecord{
        CallID: callID, Name: "taskdeck_submit_patch", Status: "pending",
        ModelTurnID: result.ResponseID, ArgumentsSHA256: agentPatchPayloadSHA256(string(rawArgs)),
        TextFallback: true,
    })
    x.Status = "waiting_tool"
    return true
}

// Only the provider-visible copy of historic observations can be compacted.
// The canonical session retains full verified results for UI, audit and future
// targeted re-reads. Keep function_call / function_call_output pairs intact,
// including call_id, while replacing old *read-only output* with an explicit
// archive notice when stateless replay would exceed a bounded context budget.
func agentModelVisibleHistory(x agentchat.Session) []agentchat.HistoryItem {
    synthetic := map[string]bool{}
    collectionCall := map[string]bool{}
    for _, call := range x.ToolCalls {
        if call.TextFallback { synthetic[call.CallID] = true }
    }
    for _, item := range x.History {
        if item.Type == "function_call" && item.Name == "taskdeck_collect_workspace" {
            collectionCall[item.CallID] = true
        }
    }
    out := make([]agentchat.HistoryItem, 0, len(x.History))
    size := 0
    for _, item := range x.History {
        if (item.Type == "function_call" || item.Type == "function_call_output") && synthetic[item.CallID] {continue}
        out = append(out, item)
        size += len(item.Content) + len(item.Arguments)
    }
    const modelHistoryBudget = 384 << 10
    const recentUnmodifiedItems = 10
    if size <= modelHistoryBudget {return out}
    for i := 0; i < len(out)-recentUnmodifiedItems && size > modelHistoryBudget; i++ {
        item := &out[i]
        if item.Type != "function_call_output" || !collectionCall[item.CallID] || len(item.Content) < 1024 {continue}
        digest := sha256.Sum256([]byte(item.Content))
        archive, _ := json.Marshal(map[string]any{
            "archived_read_only_observation": true,
            "original_bytes": len(item.Content),
            "sha256": hex.EncodeToString(digest[:]),
            "note": "Full verified Collection observation is preserved in TaskDeck session history. For exact file contents, request a scoped read with start_line/max_lines; do not assume archived contents.",
        })
        size -= len(item.Content) - len(archive)
        item.Content = string(archive)
    }
    return out
}
