package server

import (
 "strings"
 "testing"

 webassets "bletonfc/vscode_tasks_menu/web"
)

func directoryCompareSource(t *testing.T,name string) string {
 t.Helper()
 body,err:=webassets.Files.ReadFile("featuremods/"+name)
 if err!=nil{t.Fatal(err)}
 return string(body)
}
func directoryCompareRequire(t *testing.T,text string,keys ...string){
 t.Helper()
 for _,key:=range keys{
  if !strings.Contains(text,key){t.Errorf("Directory Compare contract missing %q",key)}
 }
}
func TestDirectoryCompareLoadsAsIndependentWorkspaceTab(t *testing.T){
 next:=directoryCompareSource(t,"next.js")
 files:=strings.Index(next,"/featuremods/filecompare.js")
 dirs:=strings.Index(next,"/featuremods/directorycompare.js")
 if files<0||dirs<0||files>=dirs{t.Fatal("Folder Compare must be loaded after File Compare")}
 js:=directoryCompareSource(t,"directorycompare.js")
 directoryCompareRequire(t,js,
  "TAB_ID='directory-compare'",
  "tab.className='tab dircmp-tab'",
  "tab.dataset.viewKind='directory-compare'",
  "app.activateExternalView(TAB_ID,{force:true})",
  "window.addEventListener('taskmenu:view-activated'",
  "TaskMenuTabContext?.registerTab?.(tab)",
  "globalThis.TaskMenuDirectoryCompare={open,openSources,select,compareWithSelected",
 )
}
func TestDirectoryCompareScansStructureBeforeHashing(t *testing.T){
 js:=directoryCompareSource(t,"directorycompare.js")
 directoryCompareRequire(t,js,
  "const MAX_ENTRIES=10000,MAX_DEPTH=64",
  "async function scanTree(source,signal,which)",
  "async function listDir(s,path,signal,handle)",
  "/api/project/tree?path=",
  "/api/file-transfer/list",
  "source.kind==='browser'&&source.rootHandle",
  "const [a,b]=await Promise.all([scanTree(session.left",
  "session.mode='structure'",
  "const paths=new Set([...a.keys(),...b.keys()])",
  "cancelWork();const generation=++serial",
 )
 scan:=js[strings.Index(js,"async function structureScan()"):strings.Index(js,"function chooseMode()")]
 if strings.Contains(scan,"digest(")||strings.Contains(scan,"detailDiff("){t.Fatal("Structure-only scan must not hash or read file contents")}
}
func TestDirectoryCompareHashPickerAndContentClassification(t *testing.T){
 js:=directoryCompareSource(t,"directorycompare.js")
 directoryCompareRequire(t,js,
  "['crc32','CRC32",
  "['md5','MD5",
  "['sha256','SHA-256",
  "['content','Content Compare",
  "async function digest(src,entry,algo,signal)",
  "/api/directory-compare/project-hash",
  "/api/directory-compare/remote-hash",
  "crypto.subtle.digest('SHA-256',bytes)",
  "row.stats=await detailDiff(row,ctrl.signal)",
  "return api.analyzeTexts(",
  "row.state=row.stats.important?'important':row.stats.unimportant?'unimportant':'changed'",
  "showIdentical=false;showCheck.checked=false;render();touch()",
 )
 fileCompare:=directoryCompareSource(t,"filecompare.js")
 directoryCompareRequire(t,fileCompare,"function analyzeCompareTexts(leftText,rightText,path='')","analyzeTexts:analyzeCompareTexts")
}
func TestDirectoryCompareCopyAndFileDiff(t *testing.T){
 js:=directoryCompareSource(t,"directorycompare.js")
 directoryCompareRequire(t,js,
  "node.ondblclick=()=>Promise.resolve(openFileDiff(row))",
  "return globalThis.TaskMenuFileCompare.open({title:'Directory diff'",
  "async function copyRow(row,from)",
  "if(!confirm('Copy '+file.path+' → '+target+'?",
  "/api/project/mutate",
  "/api/file-transfer/host-to-remote",
  "/api/file-transfer/remote-to-host",
  "/api/file-transfer/upload",
  "/api/file-transfer/download-ticket",
  "await writeBlobDestination(dst,row,await downloadSourceBlob(src,file))",
  "Browser-local to Host copying requires Transfer Queue",
 )
}
func TestDirectoryCompareAppearsInContextMenusAndRestores(t *testing.T){
 explorer:=directoryCompareSource(t,"explorer.js")
 transfer:=directoryCompareSource(t,"filetransfer.js")
 tabs:=directoryCompareSource(t,"workspacetabs.js")
 colors:=directoryCompareSource(t,"tabcolors.js")
 directoryCompareRequire(t,explorer,"Compare selected directories","Select directory for compare","Compare with selected directory")
 directoryCompareRequire(t,transfer,"Compare with selected remote directory","Compare with selected left directory",
 "directoryCompareLeftSource(view,entry)","directoryCompareRemoteSource(view,entry)")
 directoryCompareRequire(t,tabs,"if(tab.classList.contains('dircmp-tab'))return {kind:'directory-compare'}",
 "directoryCompare:globalThis.TaskMenuDirectoryCompare?.snapshotState?.()||null",
 "TaskMenuDirectoryCompare?.restoreState?.(saved.directoryCompare)")
 directoryCompareRequire(t,colors,"data-taskdeck-tab-type=\"directory-compare\"","if(tab.classList.contains('dircmp-tab'))return 'directory-compare'")
}
