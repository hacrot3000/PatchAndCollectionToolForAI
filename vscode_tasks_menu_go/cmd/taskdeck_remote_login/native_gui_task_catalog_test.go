package main

import (
 "strings"
 "testing"
)

func TestNativeGUIApprovedTaskCardsStayReviewOnly(t *testing.T){
 html,err:=trustedGUIAssets.ReadFile("native_gui.html")
 if err!=nil{t.Fatal(err)}
 js,err:=trustedGUIAssets.ReadFile("native_gui.js")
 if err!=nil{t.Fatal(err)}
 text:=string(js)
 for _,expected:=range []string{
  `id="task-catalog"`,`id="task-catalog-count"`,
  `data-op="tasks"`,`data-op="task_review"`,
  `id="task-id"`,`id="execute-task"`,`id="task-lifecycle"`,`id="task-verify-stop"`,
 } {
  if !strings.Contains(string(html),expected){
   t.Errorf("missing trusted native GUI control: %q",expected)
  }
 }
 for _,expected:=range []string{
  `function renderTaskCatalog(report)`,
  `function presentManagedTaskReceipt(report,handle,operation)`,
  `state==="cancel_requested"`,
  `"task-verify-stop"`,
  `new TextEncoder().encode(report).length>4096`,
  `rows.length>64`,
  `new TextEncoder().encode(name).length>120`,
  `seen.has(id)`,
  `button.textContent=`,
  `document.createElement("button")`,
  `$("task-id").value=String(item.id)`,
  `review=null;`,
  `$("approval").classList.add("hidden")`,
  `if(result&&operation==="tasks"){renderTaskCatalog(result.text||"");}`,
  `if(used>=max)$("status").textContent`,
 }{
  if !strings.Contains(text,expected){
   t.Errorf("approved-task cards miss security/UX behavior: %q",expected)
  }
 }
 for _,unsafe:=range []string{
  ".innerHTML=", "insertAdjacentHTML", "document.write(",
  "eval(", "new Function(", "window.open(", "location.href=",
 }{
  if strings.Contains(text,unsafe){
   t.Errorf("native TaskDeck task catalog contains unsafe browser execution primitive: %q",unsafe)
  }
 }
}
