package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"

	"bletonfc/vscode_tasks_menu/internal/session"
)

var errAgentPatchPromptNotReady = errors.New("native Patch selection prompt is not ready")

// agentExactPatchSelection selects exactly the approved package from a native
// queue snapshot. Never run "all selected" or trust model-provided indexes.
// It returns the prompt ID and item index so the caller can bind the final
// result to this exact approval.
func agentExactPatchSelection(state session.ProtocolState, name string) (patchPromptResponseRequest, string, int, error) {
	if !state.CommandsEnabled || !state.Available {
		return patchPromptResponseRequest{}, "", 0, errAgentPatchPromptNotReady
	}
	if len(state.Prompt) == 0 {
		return patchPromptResponseRequest{}, "", 0, errAgentPatchPromptNotReady
	}
	var prompt struct {
		Protocol   string   `json:"protocol"`
		Version    int      `json:"version"`
		Type       string   `json:"type"`
		PromptID   string   `json:"prompt_id"`
		PromptKind string   `json:"prompt_kind"`
		Actions    []string `json:"actions"`
		Items      []struct {
			Index      int    `json:"index"`
			Name       string `json:"name"`
			Kind       string `json:"kind"`
			Selectable *bool  `json:"selectable"`
		} `json:"items"`
	}
	if err := json.Unmarshal(state.Prompt, &prompt); err != nil {
		return patchPromptResponseRequest{}, "", 0, errors.New("unreadable native Patch prompt")
	}
	if prompt.Protocol != "taskdeck.patch" || prompt.Version != 1 || prompt.Type != "prompt" || prompt.PromptKind != "queue_selection" || prompt.PromptID == "" {
		return patchPromptResponseRequest{}, "", 0, errors.New("native Patch session has an unexpected active prompt")
	}
	allowed := false
	for _, v := range prompt.Actions {
		if v == "select" {
			allowed = true
			break
		}
	}
	if !allowed {
		return patchPromptResponseRequest{}, "", 0, errors.New("native Patch prompt does not advertise select")
	}
	selected := 0
	for _, item := range prompt.Items {
		if item.Name != name {
			continue
		}
		if selected != 0 {
			return patchPromptResponseRequest{}, "", 0, errors.New("native Patch prompt contains duplicate approved item names")
		}
		if item.Index < 1 || strings.ToUpper(item.Kind) != "PATCH" || (item.Selectable != nil && !*item.Selectable) {
			return patchPromptResponseRequest{}, "", 0, errors.New("approved Patch package is not selectable")
		}
		selected = item.Index
	}
	if selected == 0 {
		return patchPromptResponseRequest{}, "", 0, errAgentPatchPromptNotReady
	}
	return patchPromptResponseRequest{PromptID: prompt.PromptID, Action: "select", Indexes: []int{selected}}, prompt.PromptID, selected, nil
}

func (s *Server) agentVerifyQueuedPackage(receipt patchUploadResult) error {
	if !strings.HasPrefix(receipt.QueuePath, "patchs/") || filepath.Base(receipt.QueuePath) != receipt.Name {
		return errors.New("unsafe native Patch receipt path")
	}
	dir, err := s.patchUploadDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, receipt.Name)
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 || fi.Size() != receipt.Size {
		return errors.New("native Patch queue package was changed or replaced")
	}
	if receipt.Size > 2<<20 {
		return errors.New("agent-generated Patch package exceeds 2 MiB")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != receipt.SHA256 {
		return errors.New("native Patch queue package digest mismatch")
	}
	return nil
}

// agentStartNativePatch launches the PREEXISTING /api/sessions kind=patch
// handler, retaining native owner/permission/session identity and locks.
func (s *Server) agentStartNativePatch(r *http.Request) (string, error) {
	data := []byte(`{"kind":"patch","patch_mode":"queue","patch_ui":"native"}`)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/api/sessions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	s.sessionsRoot(response, request)
	if response.Code != http.StatusCreated {
		return "", fmt.Errorf("native Patch session start refused (HTTP %d)", response.Code)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		return "", err
	}
	if result.ID == "" || len(result.ID) > 128 {
		return "", errors.New("native Patch session ID missing")
	}
	return result.ID, nil
}

// agentSelectNativePatch uses the SAME session prompt-response HTTP handler
// used by the Patch Panel, including native prompt ID validation,
// reservePatchWorkKind and workspace mutation leases. It returns the accepted
// prompt ID and index so the ledger can bind the eventual final result.
func (s *Server) agentSelectNativePatch(ctx context.Context, r *http.Request, sessionID, name string) (string, int, error) {
	provider, ok := s.Sessions.(session.ProtocolStateProvider)
	if !ok {
		return "", 0, errors.New("native Patch protocol state unavailable")
	}
	if _, ok := s.Sessions.(session.ProtocolCommandWriter); !ok {
		return "", 0, errors.New("native Patch command channel unavailable")
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timeout, cancel := context.WithTimeout(ctx, 16*time.Second)
	defer cancel()
	for {
		state, err := provider.ProtocolState(sessionID)
		if err == nil {
			selection, promptID, index, selErr := agentExactPatchSelection(state, name)
			if selErr == nil {
				payload, _ := json.Marshal(selection)
				endpoint := "/api/sessions/" + sessionID + "/prompt-response"
				request, e := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
				if e != nil {
					return "", 0, e
				}
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				s.sessionItem(response, request)
				if response.Code != http.StatusOK {
					return "", 0, fmt.Errorf("native Patch selection refused (HTTP %d)", response.Code)
				}
				return promptID, index, nil
			}
			if !errors.Is(selErr, errAgentPatchPromptNotReady) {
				return "", 0, selErr
			}
			if len(state.LastEvent) > 0 && bytes.Contains(state.LastEvent, []byte(`"run_finished"`)) {
				return "", 0, errors.New("native Patch session finished before approved queue package was selected")
			}
		}
		select {
		case <-timeout.Done():
			return "", 0, errors.New("native Patch selection timeout; inspect existing session, never re-submit automatically")
		case <-ticker.C:
		}
	}
}

// agentPatchExpectation binds a final protocol observation to the exact queue
// item and prompt a human approved. A missing index/run is not proof of PASS.
type agentPatchExpectation struct {
	Name      string
	PromptID  string
	ItemIndex int
}

// agentPatchLogExcerpt sends the recent end of the native log, after
// credential redaction. The last lines often hold nested test exit codes,
// which can differ from the enclosing Patch rc=0.
func agentPatchLogExcerpt(raw string, maxBytes int) string {
 if maxBytes < 1 || strings.TrimSpace(raw)=="" { return "" }
 clean := scrubAIContext(raw, len(raw)+1)
 if len(clean) <= maxBytes { return clean }
 tail := strings.ToValidUTF8(clean[len(clean)-maxBytes:], "")
 return "[EARLIER LOG TRUNCATED; LAST OUTPUT FOLLOWS]\n" + tail
}

// agentPatchResultFromProtocol is strictly evidence-based: only a matching
// native item with a final status can satisfy the model tool call. Name-only
// matching is insufficient because a reused/renamed queue item would be
// attributed to the wrong approval.
func agentPatchResultFromProtocol(state session.ProtocolState, expect agentPatchExpectation) (string, string, string, bool) {
	for _, item := range state.Items {
		if item.Name != expect.Name || strings.ToUpper(item.Kind) != "PATCH" {
			continue
		}
		if expect.ItemIndex > 0 && item.Index != expect.ItemIndex {
			continue
		}
		if strings.TrimSpace(item.RunID) == "" {
			return "", "", "", false
		}
		status := strings.ToUpper(item.Status)
		switch status {
		case "PASS", "FAIL", "PREFLIGHT_FAIL", "INCOMPLETE":
			commands := make([]map[string]any, 0, len(item.Commands))
			for _, c := range item.Commands {
				command := map[string]any{
					"index": c.Index, "stage": c.Stage, "status": c.Status, "timed_out": c.TimedOut,
				}
				if c.RC != nil {
					command["rc"] = *c.RC
				}
				commands = append(commands, command)
				if len(commands) >= 24 {
					break
				}
			}
			out := map[string]any{
				"status": status, "name": item.Name, "run_id": item.RunID,
				"failure_reason":  scrubAIContext(item.FailureReason, 400),
				"diagnosis_kind":  item.DiagnosisKind,
				"elapsed_seconds": item.ElapsedSeconds,
				"commands":        commands,
				"stdout_tail":     agentPatchLogExcerpt(item.OutputTail, 1800),
				"test_verification": "Patch PASS / rc=0 only establishes that the Patch wrapper succeeded. Independently executed scripts can fail: inspect stdout_tail and any nested exit codes before asserting tests passed.",
				"project_unchanged": status=="PREFLIGHT_FAIL",
				"automatic_retry": false,
			}
			if status=="PREFLIGHT_FAIL" && item.DiagnosisKind=="insufficient_disk_space" {
				out["remediation"]="Free space in the project and temporary directories, then request a fresh Patch. The current Patch was NOT applied."
			}
			if item.RC != nil {
				out["rc"] = *item.RC
			}
			b, _ := json.Marshal(out)
			if len(b) > 4096 {
				b = []byte(`{"status":"INCOMPLETE","reason":"result exceeded data limit"}`)
			}
			if status == "PASS" {
				return "done", string(b), item.RunID, true
			}
			return "error", string(b), item.RunID, true
		}
	}
	return "", "", "", false
}

// Native batch/resource preflight can fail before queue-selection acknowledgement.
// Match ONLY a final PREFLIGHT_FAIL for this exact immutable package in this
// native session. This cannot establish PASS or authorize a new execution.
func (s *Server) agentVerifiedPreflight(record agentchat.ToolCallRecord, state session.ProtocolState) (string, string, string, bool) {
 if s.agentVerifyFinalPackage(record)!=nil {return "","","",false}
 name:=filepath.Base(record.QueuePath)
 var matched *session.ProtocolItemState
 for i:=range state.Items {
  item:=&state.Items[i]
  if item.Name!=name || strings.ToUpper(item.Kind)!="PATCH" {continue}
  if matched!=nil {return "","","",false} // duplicate names are ambiguous
  matched=item
 }
 if matched==nil || strings.ToUpper(matched.Status)!="PREFLIGHT_FAIL" ||
   strings.TrimSpace(matched.RunID)=="" ||
   (record.SelectedIndex>0 && record.SelectedIndex!=matched.Index) ||
   (record.RunID!="" && record.RunID!=matched.RunID) ||
   (strings.TrimSpace(matched.DiagnosisKind)=="" && strings.TrimSpace(matched.FailureReason)=="") {
  return "","","",false
 }
 status,result,runID,ok:=agentPatchResultFromProtocol(state,agentPatchExpectation{
  Name:name,ItemIndex:matched.Index,
 })
 if !ok || status!="error" || runID!=matched.RunID {return "","","",false}
 return status,result,runID,true
}

// Callers may present status but MUST NOT invent success when the process
// terminates without a final per-item protocol state.
func (s *Server) agentReconcilePatchRecord(x *agentToolReconcileView) {
 if x==nil || x.Record.PatchSessionID=="" {return}
 switch x.Record.Status {
 case "running","dispatching","unknown":
 default: return
 }
 var status,result,runID string
 complete,missingSession:=false,false
 if s.Sessions!=nil {
  if _,ok:=s.Sessions.Metadata(x.Record.PatchSessionID); !ok {
   missingSession=true
  } else if provider,ok:=s.Sessions.(session.ProtocolStateProvider); ok {
   if state,err:=provider.ProtocolState(x.Record.PatchSessionID); err==nil {
    // A previously selected item can finish with PASS or FAIL; a failed
    // batch preflight before selection can ONLY produce PREFLIGHT_FAIL.
    if x.Record.Status=="running" && x.Record.SelectedIndex>0 {
     status,result,runID,complete=agentPatchResultFromProtocol(state,agentPatchExpectation{
      Name:filepath.Base(x.Record.QueuePath),PromptID:x.Record.PromptID,ItemIndex:x.Record.SelectedIndex,
     })
    }
    if !complete {
     status,result,runID,complete=s.agentVerifiedPreflight(x.Record,state)
    }
   }
  }
 }
 if !complete {status,result,runID,complete=s.agentFinalResultFromHistory(x.Record)}
 if !complete {
  if missingSession && x.Record.Status!="unknown" {
   x.Record.Status="unknown"
   x.Record.Result="Native session expired without unique project-bound final evidence. Check Patch History; never rerun automatically."
   x.Changed=true
  }
  return
 }
 if err:=s.agentVerifyFinalPackage(x.Record); err!=nil {
  if x.Record.Status!="unknown" {
   x.Record.Status="unknown"
   x.Record.Result="Native Patch evidence could not be matched to approved queue/archive SHA-256. Inspect existing run; do not retry automatically."
   x.Changed=true
  }
  return
 }
 if x.Record.RunID!="" && runID!=x.Record.RunID {return}
 x.Record.Status=status
 x.Record.Result=result
 x.Record.RunID=runID
 x.Changed=true
}

type agentToolReconcileView struct {
	Record  agentchat.ToolCallRecord
	Changed bool
}
