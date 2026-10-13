package server

import (
 "strings"
 "testing"
 "net/http"
 "net/http/httptest"

 "bletonfc/vscode_tasks_menu/internal/identity"
 webassets "bletonfc/vscode_tasks_menu/web"
)
func TestAISmartTerminalUIWiresSettingsAndSuggestOnlyPanel(t *testing.T){
 files:=[]string{"featuremods/aiassistant.js","featuremods/aiplans.js","featuremods/menus.js","featuremods/next.js","featuremods/terminalpalette.js"}
 content:=map[string]string{}
 for _,p:=range files{b,e:=webassets.Files.ReadFile(p);if e!=nil{t.Fatal(e)};content[p]=string(b)}
 ai:=content[files[0]]
 for _,want:=range []string{
  "async function getSettings()",
  "async function saveSettings()",
  "conf.api_key.autocomplete='new-password'",
  "getSettings();",
  "'/api/ai/settings'",
  "'/api/ai/assist'",
  "data.confirm_external=true",
  "window.confirm('Send the previewed terminal context",
  "function previewContext()",
  "async function verifiedPreviewContext()",
  "'/api/ai/session-context?session_id='",
  "data.remote=Boolean(verified.remote)",
  "function makeContext()",
  "include_history",
  "include_git",
  "include_cwd",
  "redactPreview(text)",
  "TaskDeckShellIntegration?.getCommands",
  "mode.value",
  "getLastPlan()",
  "globalThis.TaskMenuAI=",
 }{
  if !strings.Contains(ai,want){t.Errorf("AI assistant UI missing %q",want)}
 }
 for _,notAllowed:=range []string{"innerHTML","eval(", "new Function(", "fetch('https://api.openai.com"}{
  if strings.Contains(ai,notAllowed){t.Errorf("AI UI must not use %s",notAllowed)}
 }
 plans:=content[files[1]]
 for _,want:=range []string{
  "execution disabled", "Approve proposal","Reject",
  "'/api/ai/plans'","'/api/ai/plans/decision'",
  "'/api/ai/plans/validate'", "Check read-only candidates",
  "terminalTarget()","latest.target","dialog.addEventListener('keydown'",
 }{if !strings.Contains(plans,want){t.Errorf("AI plan review missing %q",want)}}
 if strings.Contains(plans,"/api/sessions/")||strings.Contains(plans,"exec.Command")||strings.Contains(plans,"innerHTML"){
  t.Error("L3 review UI must never execute shell commands")
 }
 menus:=content[files[2]]
 for _,want:=range []string{"'AI ASSISTANT'","'AI Assistant settings…'","'Open AI Smart Terminal'","'Guided AI plans (review only)…'"}{
  if !strings.Contains(menus,want){t.Errorf("AI Settings menu missing %q",want)}
 }
 next:=content[files[3]]
 if strings.Index(next,"import '/featuremods/aiassistant.js';")<0||strings.Index(next,"import '/featuremods/aiplans.js';")<0||
  strings.Index(next,"import '/featuremods/aiassistant.js';")>strings.Index(next,"import '/featuremods/aiplans.js';"){t.Error("AI modules not loaded in correct order")}
 palette:=content[files[4]]
 if !strings.Contains(palette,"'ai-settings'")||!strings.Contains(palette,"'ai-open'"){t.Fatal("AI actions missing from Command Palette")}
}

func TestAIRoutesNeedSettingsReadAndTerminalView(t *testing.T){
 for _,path:=range []string{"/api/ai/assist","/api/ai/session-context","/api/ai/plans","/api/ai/plans/decision","/api/ai/plans/validate"}{
  permissions:=sharedRoutePermissions(httptest.NewRequest(http.MethodPost,path,nil))
  for _,required:=range []string{identity.PermissionTerminalViewOwn,identity.PermissionSettingsRead}{
   seen:=false
   for _,p:=range permissions{if p==required{seen=true}}
   if !seen{t.Fatalf("%s requires %s, received %v",path,required,permissions)}
  }
 }
 for _,m:=range []struct{method,permission string}{{http.MethodGet,identity.PermissionSettingsRead},{http.MethodPut,identity.PermissionSettingsWrite}}{
  perms:=sharedRoutePermissions(httptest.NewRequest(m.method,"/api/ai/settings",nil))
  if len(perms)!=1||perms[0]!=m.permission{t.Fatalf("%s ai/settings: %v",m.method,perms)}
 }
}
