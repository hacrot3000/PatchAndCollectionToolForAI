package server

import (
  "strings"
  "testing"

  webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitBackgroundRefreshUsesEventDrivenStrategy(t *testing.T) {
  asset,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
  if err!=nil {t.Fatal(err)}
  js:=string(asset)
  for _,want:=range []string{
    "GIT_OPEN_FALLBACK_MS=60000",
    "GIT_CLOSED_FALLBACK_MS=300000",
    "window.addEventListener('focus'",
    "document.addEventListener('visibilitychange'",
    "window.addEventListener('taskmenu:project-directory-changed'",
    "window.addEventListener('taskmenu:shell-integration'",
    "scheduleGitRefresh(900,2500)",
    "if(document.hidden)return",
    "if(refreshing){gitRefreshPending=true;return;}",
    "scheduleGitRefresh(1700,panel.classList.contains('visible')?4000:12000)",
  }{
    if !strings.Contains(js,want){t.Errorf("Git refresh scheduler missing %q",want)}
  }
  if strings.Contains(js,"setInterval(refresh,5000)") {
    t.Error("Five-second permanent Git status polling must not return")
  }
}

func TestProjectHealthNoLongerRunsFiveSecondGitStatusFallback(t *testing.T) {
  asset,err:=webassets.Files.ReadFile("featuremods/projecthealth.js")
  if err!=nil{t.Fatal(err)}
  js:=string(asset)
  if !strings.Contains(js,"},30000);"){
    t.Error("Project Health must use a 30-second visible fallback")
  }
  if strings.Contains(js,"},5000);"){
    t.Error("Project Health must not fetch Git status every 5 seconds")
  }
}
