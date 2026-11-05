package main

import (
 "bytes"
 "context"
 "os/exec"
 "runtime"
 "strings"
 "testing"
 "time"
)

// Exercise the bundled native JS, not a duplicate browser implementation.
// The fixture models only local DOM/network behavior: no Portal or Relay.
func TestTrustedGUITaskDraftsAndLongPollDOMStability(t *testing.T) {
 if runtime.GOOS!="linux" && runtime.GOOS!="darwin" {
  t.Skip("native Remote GUI development targets Linux/macOS")
 }
 node,err:=exec.LookPath("node")
 if err!=nil {t.Skip("Node.js is required for the native browser execution fixture")}
 ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,node,"trusted_gui_task_drafts_test.cjs")
 cmd.Stdin=strings.NewReader(trustedGUIJavaScript)
 var output bytes.Buffer
 cmd.Stdout=&output
 cmd.Stderr=&output
 if err:=cmd.Run();err!=nil {
  t.Fatalf("native task GUI draft/poll regression: %v\n%s",err,output.String())
 }
}
