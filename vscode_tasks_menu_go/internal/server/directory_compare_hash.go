package server

import (
 "context"
 "crypto/md5"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "hash"
 "hash/crc32"
 "io"
 "net/http"
 "os"
 pathpkg "path"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/filetransferprofile"
 "bletonfc/vscode_tasks_menu/internal/ftpclient"
 "bletonfc/vscode_tasks_menu/internal/sftpclient"
)

type directoryCompareContextReader struct {ctx context.Context;r io.Reader}
func (reader directoryCompareContextReader) Read(p []byte)(int,error){
 select{case <-reader.ctx.Done():return 0,reader.ctx.Err();default:return reader.r.Read(p)}
}

// Hashing is read-only and streamed/bounded. MD5/CRC32 are compatibility checks,
// never authentication or security integrity decisions.
type directoryCompareHashRequest struct {
 Source string `json:"source"`
 ProfileID string `json:"profile_id"`
 Path string `json:"path"`
 Algorithm string `json:"algorithm"`
}
func directoryCompareHasher(algorithm string) (hash.Hash,error){
 switch algorithm {
 case "sha256":return sha256.New(),nil
 case "md5":return md5.New(),nil
 case "crc32":return crc32.NewIEEE(),nil
 default:return nil,errors.New("unsupported directory comparison algorithm")
 }
}
func (s *Server) directoryCompareHash(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 var req directoryCompareHashRequest
 if err:=decodeFileTransferJSON(w,r,&req);err!=nil {http.Error(w,err.Error(),http.StatusBadRequest);return}
 req.Path=strings.TrimSpace(req.Path);req.ProfileID=strings.TrimSpace(req.ProfileID)
 digester,err:=directoryCompareHasher(strings.ToLower(strings.TrimSpace(req.Algorithm)))
 if err!=nil||req.Path=="" {http.Error(w,"valid path and algorithm required",http.StatusBadRequest);return}
 source:=strings.ToLower(strings.TrimSpace(req.Source))
 if (r.URL.Path=="/api/directory-compare/project-hash"&&source!="project")||
    (r.URL.Path=="/api/directory-compare/remote-hash"&&source!="remote"){
    http.Error(w,"checksum source does not match authorized route",http.StatusBadRequest);return
 }
 var size int64
 switch source {
 case "project":
  resolved,resolveErr:=s.resolveProjectPath(req.Path,false,false)
  if resolveErr!=nil{http.Error(w,"project file not available inside workspace",http.StatusNotFound);return}
  var file *os.File
  file,err=os.Open(resolved)
  if err==nil{
   var info os.FileInfo
   info,err=file.Stat()
   if err==nil&&(!info.Mode().IsRegular()||info.Size()>maxFileTransferBytes){err=errors.New("not a regular file or file exceeds hash size limit")}
   if err==nil{size,err=io.Copy(digester,io.LimitReader(directoryCompareContextReader{ctx:r.Context(),r:file},maxFileTransferBytes+1));if size>maxFileTransferBytes{err=errors.New("hash size limit exceeded")}}
   _=file.Close()
  }
 case "remote":
  if req.ProfileID=="" {http.Error(w,"remote profile id required",http.StatusBadRequest);return}
  var profile filetransferprofile.Profile
  profile,err=s.resolveFileTransferProfile(req.ProfileID)
  if err!=nil{http.Error(w,"file-transfer profile not found",http.StatusNotFound);return}
  var release func()
  var ok bool
  release,ok=s.tryAcquireFileTransferBrowse(profile.ID)
  if !ok {w.Header().Set("Retry-After","1");http.Error(w,"FTP/SFTP pool full",http.StatusTooManyRequests);return}
  defer release()
  size,err=s.directoryCompareRemoteHash(r.Context(),profile,req.Path,digester)
 default:http.Error(w,"unknown directory compare source",http.StatusBadRequest);return
 }
 if err!=nil{http.Error(w,err.Error(),http.StatusBadGateway);return}
 writeJSON(w,http.StatusOK,map[string]any{"algorithm":req.Algorithm,"digest":hex.EncodeToString(digester.Sum(nil)),"size":size})
}
func (s *Server) directoryCompareRemoteHash(ctx context.Context,profile filetransferprofile.Profile,path string,digester hash.Hash)(int64,error){
 var count int64
 switch profile.Protocol {
 case filetransferprofile.ProtocolFTP:
  limiter:=&transferLimitWriter{dst:digester,remaining:maxFileTransferBytes}
  err:=s.withFTPClient(ctx,profile,nil,func(client *ftpclient.Client)error{
   return client.Retrieve(ctx,path,limiter)
  })
  return maxFileTransferBytes-limiter.remaining,err
 case filetransferprofile.ProtocolSFTP:
  // Reject known oversize files before invoking the external SFTP GET.
  entries,listErr:=s.backgroundListRemote(ctx,profile.ID,pathpkg.Dir(path))
  if listErr!=nil{return 0,listErr}
  expectedName:=pathpkg.Base(path);found:=false
  for _,entry:=range entries{
   if entry.Name==expectedName{
    found=true
    if entry.Size>maxFileTransferBytes{return 0,errors.New("remote file exceeds hashing limit")}
    if entry.Type!="file"{return 0,errors.New("remote hash target is not a regular file")}
    break
   }
  }
  if !found{return 0,os.ErrNotExist}
  dir,err:=os.MkdirTemp("","taskdeck-directory-hash-*")
  if err!=nil{return 0,err}
  defer os.RemoveAll(dir)
  if err=os.Chmod(dir,0o700);err!=nil{return 0,err}
  temp:=filepath.Join(dir,"content")
  command,err:=sftpclient.GetCommand(path,temp)
  if err!=nil{return 0,err}
  if _,err=s.runSFTP(ctx,profile,command+"quit\n",64<<10);err!=nil{return 0,err}
  file,err:=os.Open(temp)
  if err!=nil{return 0,err}
  defer file.Close()
  info,err:=file.Stat()
  if err!=nil{return 0,err}
  if !info.Mode().IsRegular()||info.Size()>maxFileTransferBytes{return 0,fmt.Errorf("remote file exceeds safe hash limit")}
  count,err=io.Copy(digester,io.LimitReader(file,maxFileTransferBytes+1))
  if count>maxFileTransferBytes{return 0,errors.New("remote hash size limit exceeded")}
  return count,err
 default:return 0,fmt.Errorf("unsupported transfer protocol: %s",profile.Protocol)
 }
}
