package server

import (
 "crypto/sha256"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "time"
)

// AI Debug has a separate, durable display-reset watermark. The canonical
// conversation journal and native Patch/Collection evidence are NEVER deleted
// by Clear Debug; after restart the cleared historical report must not return.
type agentDebugClearCursor struct {
 HistoryItems int `json:"history_items"`
 ToolCalls int `json:"tool_calls"`
 EventSeq uint64 `json:"event_seq"`
}

type agentDebugClearMarker struct {
 At time.Time `json:"at"`
 Sessions map[string]agentDebugClearCursor `json:"sessions"`
}

func (s *Server) agentDebugClearMarkerPath(owner string) (string,error) {
 private,err:=aiPrivatePath(s.Workspace)
 if err!=nil{return "",err}
 ownerDigest:=sha256.Sum256([]byte(owner))
 filename:=fmt.Sprintf("agent-debug-reset-%s-%x.json",
  aiWorkspaceHash(s.Workspace),ownerDigest[:12])
 return filepath.Join(filepath.Dir(private),filename),nil
}
func (s *Server) readAgentDebugClearMarker(owner string) (agentDebugClearMarker,error){
 path,err:=s.agentDebugClearMarkerPath(owner)
 if err!=nil{return agentDebugClearMarker{},err}
 raw,err:=os.ReadFile(path)
 if errors.Is(err,os.ErrNotExist){return agentDebugClearMarker{},nil}
 if err!=nil{return agentDebugClearMarker{},err}
 if len(raw)>128<<10{return agentDebugClearMarker{},fmt.Errorf("debug clear marker too large")}
 var marker agentDebugClearMarker
 if err=json.Unmarshal(raw,&marker);err!=nil{return agentDebugClearMarker{},err}
 return marker,nil
}
func (s *Server) writeAgentDebugClearMarker(owner string,marker agentDebugClearMarker)error{
 path,err:=s.agentDebugClearMarkerPath(owner)
 if err!=nil{return err}
 raw,err:=json.Marshal(marker)
 if err!=nil{return err}
 if len(raw)>128<<10{return fmt.Errorf("debug clear marker too large")}
 dir:=filepath.Dir(path)
 if err=os.MkdirAll(dir,0700);err!=nil{return err}
 temp,err:=os.CreateTemp(dir,".agent-debug-reset-*")
 if err!=nil{return err}
 defer os.Remove(temp.Name())
 if err=temp.Chmod(0600);err!=nil{_ = temp.Close();return err}
 if _,err=temp.Write(raw);err!=nil{_ = temp.Close();return err}
 if err=temp.Sync();err!=nil{_ = temp.Close();return err}
 if err=temp.Close();err!=nil{return err}
 if err=os.Rename(temp.Name(),path);err!=nil{return err}
 handle,err:=os.Open(dir)
 if err!=nil{return fmt.Errorf("debug clear durability unknown: %w",err)}
 defer handle.Close()
 if err=handle.Sync();err!=nil{
  return fmt.Errorf("debug clear durability unknown: %w",err)
 }
 return nil
}
