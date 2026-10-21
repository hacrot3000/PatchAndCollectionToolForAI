package server

import (
    "strings"
    "testing"

    webassets "bletonfc/vscode_tasks_menu/web"
)

// Both in-place restart modes replace the backend session ID without replacing
// the terminal pane. File/URL detection must follow that ID change as well.
func TestRestartInPlaceRetargetsDetectionState(t *testing.T) {
    data, err := webassets.Files.ReadFile("features.js")
    if err != nil {
        t.Fatal(err)
    }
    js := string(data)
    for _, want := range []string{
        "window.addEventListener('taskmenu:session-retargeted'",
        "retargetDetectionState(event.detail)",
        "const state=states.get(previousID)",
        "if(!state||state.view!==view)return;",
        "const duplicate=states.get(nextID)",
        "duplicate.inputDisposable?.dispose()",
        "clearTimeout(state.timer)",
        "state.seq++; // Reject file-selection requests started by the old session.",
        "state.recent=''",
        "state.inputBuffer=''",
        "state.inputLines=[]",
        "state.suppressGitOutput=false",
        "state.gitTaskOutput=taskUsesGit(view)",
        "states.delete(previousID)",
        "states.set(nextID,state)",
        "saveIgnored(state)",
        "saveIgnoredURLs(state)",
        "saveFileDetectionEnabled(state)",
        "sessionStorage.removeItem('vscode-tasks-menu:ignored-files:'+previousID)",
        "sessionStorage.removeItem('vscode-tasks-menu:ignored-urls:'+previousID)",
        "sessionStorage.removeItem('vscode-tasks-menu:file-detection:'+previousID)",
        "render(state)",
        "taskmenu:file-detection-changed",
    } {
        if !strings.Contains(js, want) {
            t.Errorf("detector must follow an in-place task restart: missing %q", want)
        }
    }

    clearBlock := strings.Index(js, "if(clearLog){\n    state.files.clear();")
    rekey := strings.Index(js, "states.delete(previousID);\n  states.set(nextID,state);")
    if clearBlock < 0 || rekey < 0 || clearBlock > rekey ||
        !strings.Contains(js[clearBlock:rekey], "state.urls.clear()") ||
        !strings.Contains(js[clearBlock:rekey], "state.ignored.clear()") ||
        !strings.Contains(js[clearBlock:rekey], "state.ignoredURLs.clear()") {
        t.Error("Clear Log restart must drop prior files/URLs and old ignored entries before transferring state")
    }

    if !strings.Contains(indexHTML, "detail:{oldID,newID:meta.id,view,clearLog}") {
        t.Error("retarget event must provide clearLog to distinguish Keep Log and Clear Log")
    }

    controls, err := webassets.Files.ReadFile("featuremods/clearfiles.js")
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(string(controls), "taskmenu:file-detection-changed") ||
        !strings.Contains(string(controls), "bar?.querySelector('.detected-ignore')") {
        t.Error("Clear detected control must refresh against the retained detection bar")
    }
}
