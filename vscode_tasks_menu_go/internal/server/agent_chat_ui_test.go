package server

import (
 "strings"
 "testing"

 webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAgentChatUIHasNoExecutionAndWiresProviderSessions(t *testing.T){
 files:=[]string{"featuremods/agentchat.js","featuremods/next.js","featuremods/menus.js"}
 content:=map[string]string{}
 for _,path:=range files{
  b,err:=webassets.Files.ReadFile(path);if err!=nil{t.Fatal(err)}
  content[path]=string(b)
 }
 js:=content[files[0]]
 for _,needle:=range []string{
  "'/api/agent/providers'",
  "'/api/agent/providers/test'",
  "'/api/agent/sessions'",
  "'/messages'",
  "confirm_external:true",
  "expected_revision:s.revision",
  "Tool execution disabled",
  "globalThis.TaskMenuAgentChat=",
  "API key",
  "settingsOverlay",
 } {
  if !strings.Contains(js,needle){t.Errorf("Agent Chat UI lacks %q",needle)}
 }
 for _,unsafe:=range []string{"innerHTML","eval(","new Function(","/api/patch/paste","/api/terminal/command"}{
  if strings.Contains(js,unsafe){t.Errorf("Agent Chat must not execute commands or insert HTML through %q",unsafe)}
 }
 if !strings.Contains(content[files[1]],"import '/featuremods/agentchat.js';"){
  t.Error("Agent Chat JS not loaded into web frontend")
 }
 if !strings.Contains(content[files[2]],"Open AI Agent Chat")||!strings.Contains(content[files[2]],"AI Agent providers / API keys"){
  t.Error("Settings menu lacks Agent Chat launch actions")
 }
}
