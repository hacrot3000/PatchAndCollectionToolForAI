package server

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

var errRemoteDirectorySymlink=errors.New("symlink entry is not remotely enumerable")

// readRemoteDirectory is a small, authenticated FilesRead-only discovery
// primitive. It is not a general-purpose filesystem walk. Hidden entries,
// unsafe names, links, sockets, devices and other non-regular files never
// leave the host. No pagination/cursor or file handle crosses the E2EE
// boundary. A large directory fails closed instead of sending partial data.
func (s *Server) readRemoteDirectory(ctx context.Context, requested string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil {return nil,denied}
 var dir *os.File
 var rootPath,relativeDir string
 if requested=="." {
  // Primary workspace root is independently configured on the host.
  root,err:=s.primaryWorkspaceRoot()
  if err!=nil {return nil,denied}
  dir,err=os.Open(root.Path)
  if err!=nil{return nil,denied}
  rootPath,relativeDir=root.Path,"."
 }else{
  // Reuse the existing canonical visible-source path boundary: disallow
  // dot files/directories, attached roots, parent escape, symlinks to
  // protected directories and references outside the primary workspace.
  // The existing restrictedSourcePath is FILE-only: it calls
  // resolveProjectPath(...,wantDir=false), which intentionally denies
  // directories. Preserve its hidden/realpath safety rules, but resolve
  // specifically as a directory instead of weakening file permissions.
  relative,err:=cleanProjectRelativePath(requested,false)
  if err!=nil||!restrictedVisibleRelativePath(relative)||
   strings.HasPrefix(relative,"@root/"){return nil,denied}
  path,err:=s.resolveProjectPath(relative,false,true)
  if err!=nil{return nil,denied}
  view,_,err:=s.projectRootForVirtualPath(relative)
  if err!=nil||!view.Primary{return nil,denied}
  root,err:=filepath.EvalSymlinks(view.Path)
  if err!=nil{return nil,denied}
  actual,err:=filepath.EvalSymlinks(path)
  if err!=nil||!pathWithin(root,actual){return nil,denied}
  within,err:=filepath.Rel(root,actual)
  if err!=nil||!restrictedVisibleRelativePath(within){return nil,denied}
  rootPath,relativeDir=root,within
  pinned,err:=openProjectPinnedFile(root,actual)
  if err!=nil{return nil,denied}
  defer pinned.close()
  dir,err=pinned.openCurrent()
  if err!=nil{return nil,denied}
 }
 defer dir.Close()
 info,err:=dir.Stat()
 if err!=nil||!info.IsDir(){return nil,fmt.Errorf("remote directory stat: %w",denied)}
 // Bounded directory enumeration. An attacker cannot force unbounded
 // allocation or partial success with arbitrarily many long names.
 entries,err:=dir.ReadDir(65)
 if (err!=nil&&!errors.Is(err,io.EOF))||len(entries)>64{return nil,fmt.Errorf("remote directory entries: %w",denied)}
 lines:=make([]string,0,len(entries))
 for _,entry:=range entries{
  if ctx.Err()!=nil{return nil,denied}
  name:=entry.Name()
  if len(name)==0||len(name)>80||!utf8.ValidString(name)||
   strings.HasPrefix(name,".")||name==".."||
   strings.ContainsAny(name,"/\\\x00\r\n\t") {continue}
  typ:=entry.Type()
  if typ==0 {
   // An unknown d_type (including ordinary regular files) must be
   // inspected via the PINNED directory descriptor. DirEntry.Info()
   // may resolve the relative fd name against TaskDeck's daemon cwd.
   // A no-follow symlink is safely omitted, never dereferenced.
   checked,checkErr:=remotePinnedDirectoryEntryMode(dir,name,rootPath,relativeDir)
   if errors.Is(checkErr,errRemoteDirectorySymlink){continue}
   if checkErr!=nil{return nil,fmt.Errorf("remote directory entry metadata: %w",denied)}
   typ=checked
  }
  kind:=""
  if typ.IsDir(){kind="D"}else if typ.IsRegular(){kind="F"}
  if kind==""{continue}
  lines=append(lines,kind+"\t"+name)
 }
 sort.Strings(lines)
 result:=[]byte(strings.Join(lines,"\n"))
 if len(result)>192||!utf8.Valid(result){return nil,fmt.Errorf("remote directory result bound: %w",denied)}
 if ctx.Err()!=nil{return nil,denied}
 // The authorization boundary rechecks TaskDeck-local rights, current
 // Portal grant, client key possession and session lifetime before AEAD
 // sealing and forwarding this bounded result.
 return result,nil
}

