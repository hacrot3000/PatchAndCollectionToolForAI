package server

import (
 "archive/zip"
 "bytes"
 "crypto/sha256"
 "fmt"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentNativeFullFileReportStrict(t *testing.T){
 full:="# file reader\n\nPath: `src/main.go`\nTotal lines: 2\nSelected: 1-2\n\n```text\n1: package main\n2: // end\n```\n\n"
 if proof,ok:=parseNativeFullFileReport([]byte(full),1);!ok||proof.TotalLines!=2||proof.Path!="src/main.go"||len(proof.ReportSHA256)!=64{t.Fatalf("valid report rejected: %+v",proof)}
 empty:="# file reader\n\nPath: `empty.txt`\nTotal lines: 0\nSelected: 1-0\n\n```text\n```\n\n"
 if proof,ok:=parseNativeFullFileReport([]byte(empty),2);!ok||proof.TotalLines!=0{t.Fatal("empty file rejected")}
 for _,bad:=range []string{
  strings.Replace(full,"Selected: 1-2","Selected: 2-2",1),
  strings.Replace(full,"1: package main\n","",1),
  strings.Replace(full,"2: // end","3: // end",1),
  strings.Replace(full,"package main","<REDACTED>",1),
  strings.Replace(full,"package main","[TRUNCATED]",1),
  strings.Replace(full,"src/main.go","../main.go",1),
  strings.Replace(full,"# file reader","# range reader",1),
  full+strings.Repeat("x",nativeFullFileReportBudget),
 }{
  if _,ok:=parseNativeFullFileReport([]byte(bad),1);ok{t.Fatal("partial or malformed native report accepted")}
 }
}

func TestAgentNativeFullFileArtifactReceipt(t *testing.T){
 root:=t.TempDir()
 identity:=patchtool.ProjectIdentity{Key:"fixture",ID:"fixture-id"}
 dir:=filepath.Join(root,"artifacts","patch_tool_code_collections")
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 const stem="CODE_COLLECTION_RESULT_exact_file"
 zipRel:="artifacts/patch_tool_code_collections/"+stem+".zip"
 txtRel:="artifacts/patch_tool_code_collections/"+stem+".txt"
 report:="# file reader\n\nPath: `main.go`\nTotal lines: 1\nSelected: 1-1\n\n```text\n1: package main\n```\n\n"
 if err:=os.WriteFile(filepath.Join(dir,stem+".txt"),[]byte(report),0600);err!=nil{t.Fatal(err)}
 write:=func(truncated,duplicate bool){
  var buffer bytes.Buffer
  z:=zip.NewWriter(&buffer)
  manifest:=map[string]any{"format":"python-patch-tool-code-collection","format_version":3,"project_identity":identity,"collection_status":"PASS","action_count":1,
   "reports":[]any{map[string]any{"action":1,"type":"file","archive_path":"reports/001_file.md","truncated":truncated}}}
  raw,_:=json.Marshal(manifest)
  put:=func(name string,data []byte){w,e:=z.Create(name);if e!=nil{t.Fatal(e)};if _,e=w.Write(data);e!=nil{t.Fatal(e)}}
  put("COLLECTION_MANIFEST.json",raw);put("reports/001_file.md",[]byte(report))
  if duplicate{put("reports/001_file.md",[]byte(report))}
  if err:=z.Close();err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(dir,stem+".zip"),buffer.Bytes(),0600);err!=nil{t.Fatal(err)}
 }
 rc:=0
 item:=session.ProtocolItemState{Name:"request.zip",RunID:"run-1",Status:"PASS",RC:&rc}
 for _,tc:=range []struct{truncated,duplicate,want bool}{{false,false,true},{true,false,false},{false,true,false}}{
  write(tc.truncated,tc.duplicate)
  raw:=agentNativeCollectionObservation(root,identity,"session-1",item,zipRel,txtRel)
  var got struct{Reads []nativeFullFileReadProof `json:"native_full_file_reads"`}
  if err:=json.Unmarshal([]byte(raw),&got);err!=nil{t.Fatal(err)}
  if (len(got.Reads)==1)!=tc.want{t.Fatalf("unexpected proof: %+v",got)}
 }
}

func TestAgentNativeLineProofStrict(t *testing.T){
 report:="# range reader\n\nPath: `src/main.go`\nTotal lines: 100\nSelected: 20-21\n\n```text\n20: a\n21: b\n```\n\n"
 p:=nativeLineReadProof{Path:"src/main.go",ActionIndex:1,TotalLines:100,StartLine:20,EndLine:21,SourceRevisionSHA256:strings.Repeat("a",64),ReportSHA256:fmt.Sprintf("%x",sha256.Sum256([]byte(report)))}
 if got,ok:=validateNativeLineReport([]byte(report),"range",p);!ok||got.Type!="range"{t.Fatalf("valid proof rejected: %+v",got)}
 for _,bad:=range []string{strings.Replace(report,"20: a","19: a",1),strings.Replace(report,"Selected: 20-21","Selected: 19-21",1),strings.Replace(report,"20: a","20: <REDACTED>",1),strings.Replace(report,"Path: `src/main.go`","Path: `other.go`",1)}{
  q:=p;q.ReportSHA256=fmt.Sprintf("%x",sha256.Sum256([]byte(bad)))
  if _,ok:=validateNativeLineReport([]byte(bad),"range",q);ok{t.Fatal("invalid proof accepted")}
 }
 if _,ok:=validateNativeLineReport([]byte(report+"x"),"range",p);ok{t.Fatal("digest mismatch accepted")}
 p.SourceRevisionSHA256=""
 if _,ok:=validateNativeLineReport([]byte(report),"range",p);ok{t.Fatal("missing revision accepted")}
}
