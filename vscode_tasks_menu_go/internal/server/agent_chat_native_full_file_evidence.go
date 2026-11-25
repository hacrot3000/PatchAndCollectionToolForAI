package server

import (
 "archive/zip"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "strconv"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// The native Python collector writes a numbered file-reader report in its
// ZIP. Verify only this known format, without reading the workspace again
// or trusting the model's description. Oversized, redacted, partial or
// older/unrecognized reports simply do not acquire an automatic proof.
type nativeFullFileReadProof struct {
 Path string `json:"path"`
 ActionIndex int `json:"action_index"`
 TotalLines int `json:"total_lines"`
 ReportSHA256 string `json:"report_sha256"`
}

type nativeLineReadProof struct {
 Path string `json:"path"`
 ActionIndex int `json:"action_index"`
 Type string `json:"type"`
 TotalLines int `json:"total_lines"`
 StartLine int `json:"start_line"`
 EndLine int `json:"end_line"`
 SourceRevisionSHA256 string `json:"source_revision_sha256"`
 ReportSHA256 string `json:"report_sha256"`
}

func validNativeSHA256(s string)bool{
 return len(s)==64&&strings.Trim(s,"0123456789abcdef")==""
}

// Validate the optional native manifest proof against the exact numbered
// report bytes. It is not a request-side hint or permission to reuse a read.
func validateNativeLineReport(raw []byte,kind string,p nativeLineReadProof)(nativeLineReadProof,bool){
 var none nativeLineReadProof
 if len(raw)==0||len(raw)>nativeFullFileReportBudget||!utf8.Valid(raw)||!validNativeSHA256(p.SourceRevisionSHA256)||!validNativeSHA256(p.ReportSHA256){return none,false}
 switch kind{case "file","range","head","tail":default:return none,false}
 body:=string(raw)
 if strings.ContainsAny(body,"\x00\r\uFFFD")||strings.Contains(body,"<REDACTED")||strings.Contains(body,"[TRUNCATED")||strings.Contains(body,"[PARTIAL RESULT"){return none,false}
 if agentchat.ValidateTaskAutoCheck(&agentchat.TaskAutoCheck{Kind:"full_file_read",CallID:"native-report",Path:p.Path})!=nil{return none,false}
 if p.StartLine<1||p.EndLine<p.StartLine||p.TotalLines<p.EndLine||p.TotalLines>1<<30{return none,false}
 count:=p.EndLine-p.StartLine+1
 lines:=strings.Split(body,"\n")
 if count>len(lines)||len(lines)!=count+10||
  lines[0]!="# "+kind+" reader"||lines[1]!=""||
  lines[2]!="Path: `"+p.Path+"`"||lines[3]!=fmt.Sprintf("Total lines: %d",p.TotalLines)||
  lines[4]!=fmt.Sprintf("Selected: %d-%d",p.StartLine,p.EndLine)||lines[5]!=""||lines[6]!="```text"{return none,false}
 width:=len(strconv.Itoa(p.EndLine))
 for i:=0;i<count;i++{
  if !strings.HasPrefix(lines[7+i],fmt.Sprintf("%*d: ",width,p.StartLine+i)){return none,false}
 }
 if lines[7+count]!="```"||lines[8+count]!=""||lines[9+count]!=""{return none,false}
 digest:=sha256.Sum256(raw)
 if hex.EncodeToString(digest[:])!=p.ReportSHA256{return none,false}
 p.Type=kind
 return p,true
}

const nativeFullFileReportBudget=96<<10

func parseNativeFullFileReport(raw []byte,actionIndex int)(nativeFullFileReadProof,bool){
 var none nativeFullFileReadProof
 if len(raw)==0||len(raw)>nativeFullFileReportBudget||!utf8.Valid(raw){return none,false}
 body:=string(raw)
 if strings.ContainsAny(body,"\x00\r\uFFFD")||strings.Contains(body,"<REDACTED")||strings.Contains(body,"[TRUNCATED")||strings.Contains(body,"[PARTIAL RESULT"){return none,false}
 lines:=strings.Split(body,"\n")
 if len(lines)<10||lines[0]!="# file reader"||lines[1]!=""||
  !strings.HasPrefix(lines[2],"Path: `")||!strings.HasSuffix(lines[2],"`")||
  !strings.HasPrefix(lines[3],"Total lines: ")||lines[5]!=""||lines[6]!="```text"{return none,false}
 path:=strings.TrimSuffix(strings.TrimPrefix(lines[2],"Path: `"),"`")
 if agentchat.ValidateTaskAutoCheck(&agentchat.TaskAutoCheck{Kind:"full_file_read",CallID:"native-report",Path:path})!=nil{return none,false}
 total,e:=strconv.Atoi(strings.TrimPrefix(lines[3],"Total lines: "))
 if e!=nil||total<0||total>len(lines)||lines[3]!="Total lines: "+strconv.Itoa(total)||
  lines[4]!=fmt.Sprintf("Selected: 1-%d",total)||len(lines)!=total+10{return none,false}
 width:=len(strconv.Itoa(total))
 for i:=1;i<=total;i++{
  if !strings.HasPrefix(lines[6+i],fmt.Sprintf("%*d: ",width,i)){return none,false}
 }
 if lines[7+total]!="```"||lines[8+total]!=""||lines[9+total]!=""{return none,false}
 digest:=sha256.Sum256(raw)
 return nativeFullFileReadProof{Path:path,ActionIndex:actionIndex,TotalLines:total,ReportSHA256:hex.EncodeToString(digest[:])},true
}

// Inspect the exact native result artifact at execution time. Bound both
// each member and aggregate work; never inflate an entire collection ZIP.
func agentNativeFullFileReadProofs(root,relative string,identity patchtool.ProjectIdentity)[]nativeFullFileReadProof{
 full,_:=agentNativeReadProofs(root,relative,identity);return full
}

func agentNativeReadProofs(root,relative string,identity patchtool.ProjectIdentity)([]nativeFullFileReadProof,[]nativeLineReadProof){
 filename,err:=agentNativeCollectionArtifactPath(root,relative,".zip")
 if err!=nil{return nil,nil}
 z,err:=zip.OpenReader(filename);if err!=nil{return nil,nil};defer z.Close()
 members:=map[string]*zip.File{}
 for _,entry:=range z.File{
  if _,exists:=members[entry.Name];exists{return nil,nil}
  members[entry.Name]=entry
 }
 read:=func(name string,limit int64)([]byte,bool){
  entry:=members[name]
  if entry==nil||!entry.Mode().IsRegular()||entry.UncompressedSize64>uint64(limit){return nil,false}
  r,e:=entry.Open();if e!=nil{return nil,false}
  raw,e:=io.ReadAll(io.LimitReader(r,limit+1));closeErr:=r.Close()
  return raw,e==nil&&closeErr==nil&&int64(len(raw))<=limit&&uint64(len(raw))==entry.UncompressedSize64
 }
 raw,ok:=read("COLLECTION_MANIFEST.json",1<<20);if !ok{return nil,nil}
 var manifest struct{
  Format string `json:"format"`
  Version int `json:"format_version"`
  Project patchtool.ProjectIdentity `json:"project_identity"`
  Status string `json:"collection_status"`
  ActionCount int `json:"action_count"`
  Reports []struct{
   Action int `json:"action"`
   Type string `json:"type"`
   ArchivePath string `json:"archive_path"`
   Truncated *bool `json:"truncated"`
   LineProof *nativeLineReadProof `json:"line_proof"`
  } `json:"reports"`
 }
 if json.Unmarshal(raw,&manifest)!=nil||manifest.Format!="python-patch-tool-code-collection"||
  manifest.Version!=3||manifest.Project!=identity||manifest.Status!="PASS"||
  manifest.ActionCount<1||manifest.ActionCount>1024||len(manifest.Reports)>1024{return nil,nil}
 seen:=map[int]bool{}
 for _,report:=range manifest.Reports{
  if report.Action<1||report.Action>manifest.ActionCount||seen[report.Action]{return nil,nil}
  seen[report.Action]=true
 }
 var proofs []nativeFullFileReadProof
 var ranges []nativeLineReadProof
 inspected:=0
 remaining:=int64(4*nativeFullFileReportBudget)
 for _,report:=range manifest.Reports{
  switch report.Type{case "file","range","head","tail":default:continue}
  if report.Type!="file"&&report.LineProof==nil{continue}
  if report.Truncated==nil||*report.Truncated||
   report.ArchivePath!=fmt.Sprintf("reports/%03d_%s.md",report.Action,report.Type){continue}
  if inspected>=32||remaining<=0{break}
  inspected++
  limit:=int64(nativeFullFileReportBudget);if remaining<limit{limit=remaining}
  raw,ok:=read(report.ArchivePath,limit)
  if !ok{continue}
  remaining-=int64(len(raw))
  if report.Type=="file"{if proof,ok:=parseNativeFullFileReport(raw,report.Action);ok{proofs=append(proofs,proof)}}
  if report.LineProof!=nil{
   p:=*report.LineProof;p.ActionIndex=report.Action
   if proof,ok:=validateNativeLineReport(raw,report.Type,p);ok{ranges=append(ranges,proof)}
  }
 }
 return proofs,ranges
}
