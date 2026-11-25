package server

import (
 "encoding/json"
 "io"
 "os"
 "path/filepath"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Read only the document beside the exact runtime used by native preflight.
// Reload per payload so self-update does not leave stale schema cache.
func agentInstalledCollectionContract(workspace string) string {
 exe,err:=os.Executable();if err!=nil{return ""}
 runtime,err:=patchtool.Resolve(workspace,exe);if err!=nil||runtime.Kind==patchtool.KindLegacyLauncher{return ""}
 f,err:=os.Open(filepath.Join(filepath.Dir(runtime.Path),"_patch_lib","docs","COLLECT_ACTION_SCHEMA.json"))
 if err!=nil{return ""}
 defer f.Close()
 raw,err:=io.ReadAll(io.LimitReader(f,(128<<10)+1))
 if err!=nil||len(raw)>128<<10||!json.Valid(raw){return ""}
 return string(raw)
}
