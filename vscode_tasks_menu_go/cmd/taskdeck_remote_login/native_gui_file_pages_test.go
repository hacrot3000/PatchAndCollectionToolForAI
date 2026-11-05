package main

import (
 "strings"
 "testing"
)

func TestNativeGUIFileLinesRequireCompleteAuthenticatedSHAEnvelope(t *testing.T) {
 g,f:=newTestNativeGUI()
 f.linePageOverride=guiLinePage(6,"valid source")
 code,res:=guiRequestTest(t,g,guiRequest{
  Operation:"file_lines",Path:"src/example.go",Offset:6,
 },g.origin,g.key)
 if code!=200||!res.OK||res.Text!=f.linePageOverride||
  res.Used!=1||len(f.calls)!=1||f.calls[0]!="lines:src/example.go"{
  t.Fatalf("valid complete page was lost: %d %+v %+v",code,res,f.calls)
 }
}
func TestNativeGUIFileLinesNeverExposeMalformedUntrustedPage(t *testing.T) {
 variants:=[]string{
  "unverified direct text",
  guiLinePage(1,"payload")+"extra unverified bytes",
  strings.Replace(guiLinePage(1,"payload"),"START_LINE 1","START_LINE 2",1),
  strings.Replace(guiLinePage(1,"payload"),"NEXT_LINE 0","NEXT_LINE 2",1),
  strings.Repeat("X",4097),
  strings.Replace(guiLinePage(1,"payload"),strings.Repeat("a",64),"not-a-sha",1),
 }
 for i,v:=range variants{
  g,f:=newTestNativeGUI()
  f.linePageOverride=v
  code,res:=guiRequestTest(t,g,guiRequest{
   Operation:"file_lines",Path:"src/notes.txt",Offset:1,
  },g.origin,g.key)
  if code!=400||res.OK||res.Text!=""||res.Used!=1||len(f.calls)!=1{
   t.Fatalf("malformed source page %d leaked: %d %+v %v",i,code,res,f.calls)
  }
 }
}
func TestNativeGUIFileLinesCannotBypassHostOriginOrQuota(t *testing.T){
 g,f:=newTestNativeGUI()
 f.linePageOverride=guiLinePage(1,"line")
 code,res:=guiRequestTest(t,g,guiRequest{Operation:"file_lines",Path:"safe.txt",Offset:1},
  "https://hacked-php.example",g.key)
 if code!=403||res.OK||len(f.calls)!=0{
  t.Fatal("PHP origin accessed E2EE source page")
 }
 for i:=0;i<6;i++{
  guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 }
 code,res=guiRequestTest(t,g,guiRequest{Operation:"file_lines",Path:"safe.txt",Offset:1},g.origin,g.key)
 if code!=400||res.OK||len(f.calls)!=6{
  t.Fatalf("source page bypassed six-round E2EE quota: %d %+v",code,res)
 }
}
