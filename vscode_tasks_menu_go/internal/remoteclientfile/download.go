// Package remoteclientfile assembles authenticated E2EE file windows into
// private, crash-resumable local files. PHP/Relay never sees this package.
package remoteclientfile

import (
 "bytes"
 "crypto/rand"
 "crypto/sha256"
 "encoding/binary"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "syscall"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const headerSize=384
const headerMagic="taskdeck-download-v1"
const partialSuffix=".taskdeck-part"
var ErrUnsafeOutput=errors.New("unsafe local E2EE download destination")
var ErrChangedRemoteFile=errors.New("remote file changed during authenticated download")
var ErrInvalidWindow=errors.New("invalid authenticated file window")

type manifest struct {
 DeviceID string
 RemotePath string
 SHA256 string
 Size int64
}
type Download struct {
 file *os.File
 partial string
 destination string
 expected manifest
 initialized bool
 offset int64
 closed bool
 completed bool
}
type Progress struct {
 Offset int64
 Size int64
 SHA256 string
 Complete bool
}

func validSHA(s string)bool{
 if len(s)!=64{return false}
 if _,err:=hex.DecodeString(s);err!=nil{return false}
 for _,c:=range s{if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}}
 return true
}
func validRemotePath(path string)bool{
 if len(path)==0||len(path)>180||strings.HasPrefix(path,"/")||
  strings.Contains(path,"..")||strings.ContainsAny(path,"\\\r\n\x00:"){
  return false
 }
 for _,component:=range strings.Split(path,"/"){
  if component==""||strings.HasPrefix(component,"."){return false}
 }
 return true
}
func validateDestination(path string)error{
 if !filepath.IsAbs(path)||filepath.Clean(path)!=path||
  filepath.Base(path)=="."||filepath.Base(path)==".."||
  strings.HasSuffix(path,partialSuffix){return ErrUnsafeOutput}
 parent:=filepath.Dir(path)
 canonical,err:=filepath.EvalSymlinks(parent)
 if err!=nil||canonical!=parent{return ErrUnsafeOutput}
 info,err:=os.Lstat(parent)
 if err!=nil||!info.IsDir()||info.Mode().Perm()&0022!=0{return ErrUnsafeOutput}
 stat,ok:=info.Sys().(*syscall.Stat_t)
 if !ok||stat.Uid!=uint32(os.Geteuid()){return ErrUnsafeOutput}
 if _,err:=os.Lstat(path);err==nil{return ErrUnsafeOutput
 }else if !os.IsNotExist(err){return ErrUnsafeOutput}
 return nil
}
func openNew(path string)(*os.File,error){
 fd,err:=syscall.Open(path,syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|
  syscall.O_CLOEXEC|syscall.O_NOFOLLOW,0600)
 if err!=nil{return nil,err}
 return os.NewFile(uintptr(fd),path),nil
}
func openOld(path string)(*os.File,error){
 before,err:=os.Lstat(path)
 if os.IsNotExist(err){return nil,err}
 if err!=nil||!before.Mode().IsRegular()||before.Mode().Perm()&0077!=0{
  return nil,ErrUnsafeOutput
 }
 fd,err:=syscall.Open(path,syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,0)
 if err!=nil{return nil,ErrUnsafeOutput}
 f:=os.NewFile(uintptr(fd),path)
 after,err:=f.Stat()
 if err!=nil||!os.SameFile(before,after)||!after.Mode().IsRegular()||
  after.Mode().Perm()&0077!=0{
  _=f.Close();return nil,ErrUnsafeOutput
 }
 owner,ok:=after.Sys().(*syscall.Stat_t)
 if !ok||owner.Uid!=uint32(os.Geteuid())||owner.Nlink!=1{
  _=f.Close();return nil,ErrUnsafeOutput
 }
 return f,nil
}
// Fixed-size canonical binary header has no executable content and no
// adjustable parser grammar: magic, pinned device, full-file hash, size,
// source path length and path. Remaining bytes MUST be zero.
func encodeHeader(m manifest)([]byte,error){
 if !validSHA(m.DeviceID)||!validSHA(m.SHA256)||
  !validRemotePath(m.RemotePath)||m.Size<0||
  m.Size>remotetransport.RemoteFileWindowMaxSize{return nil,ErrUnsafeOutput}
 b:=make([]byte,headerSize)
 copy(b[:32],headerMagic)
 copy(b[32:96],m.DeviceID)
 copy(b[96:160],m.SHA256)
 binary.BigEndian.PutUint64(b[160:168],uint64(m.Size))
 binary.BigEndian.PutUint16(b[168:170],uint16(len(m.RemotePath)))
 copy(b[170:350],m.RemotePath)
 return b,nil
}
func decodeHeader(b []byte)(manifest,error){
 var m manifest
 if len(b)!=headerSize||string(bytes.TrimRight(b[:32],"\x00"))!=headerMagic{
  return m,ErrUnsafeOutput
 }
 n:=int(binary.BigEndian.Uint16(b[168:170]))
 if n<1||n>180{return m,ErrUnsafeOutput}
 m=manifest{DeviceID:string(b[32:96]),SHA256:string(b[96:160]),
  Size:int64(binary.BigEndian.Uint64(b[160:168])),RemotePath:string(b[170:170+n])}
 canonical,err:=encodeHeader(m)
 if err!=nil||!bytes.Equal(canonical,b){return m,ErrUnsafeOutput}
 return m,nil
}

// Open always refuses existing final output. The partial file is local,
// owner-only, nofollow and exclusively locked for the entire login session.
func Open(destination,deviceID,remotePath string)(*Download,error){
 if !validSHA(deviceID)||!validRemotePath(remotePath)||
  validateDestination(destination)!=nil{return nil,ErrUnsafeOutput}
 part:=destination+partialSuffix
 f,err:=openOld(part)
 if os.IsNotExist(err){f,err=openNew(part)}
 if err!=nil{return nil,ErrUnsafeOutput}
 if err:=syscall.Flock(int(f.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{
  _=f.Close();return nil,ErrUnsafeOutput
 }
 fail:=func(e error)(*Download,error){
  _=syscall.Flock(int(f.Fd()),syscall.LOCK_UN);_ =f.Close()
  return nil,e
 }
 info,err:=f.Stat()
 if err!=nil||info.Size()<0||
  info.Size()>headerSize+remotetransport.RemoteFileWindowMaxSize{
  return fail(ErrUnsafeOutput)
 }
 d:=&Download{file:f,partial:part,destination:destination,
  expected:manifest{DeviceID:deviceID,RemotePath:remotePath}}
 if info.Size()==0{return d,nil}
 if info.Size()<headerSize{return fail(ErrUnsafeOutput)}
 raw:=make([]byte,headerSize)
 if _,err:=f.ReadAt(raw,0);err!=nil{return fail(ErrUnsafeOutput)}
 m,err:=decodeHeader(raw)
 if err!=nil||m.DeviceID!=deviceID||m.RemotePath!=remotePath{
  return fail(ErrUnsafeOutput)
 }
 offset:=info.Size()-headerSize
 if offset>m.Size{return fail(ErrUnsafeOutput)}
 d.expected=m
 d.initialized=true
 d.offset=offset
 return d,nil
}
func (d *Download) Progress()Progress{
 if d==nil{return Progress{}}
 return Progress{Offset:d.offset,Size:d.expected.Size,
  SHA256:d.expected.SHA256,Complete:d.completed}
}
func (d *Download) Close()error{
 if d==nil||d.closed{return nil}
 d.closed=true
 _=syscall.Flock(int(d.file.Fd()),syscall.LOCK_UN)
 return d.file.Close()
}
func (d *Download) Append(w remotetransport.FileWindow)(Progress,error){
 return d.appendBounded(w,remotetransport.RemoteFileWindowMaxBytes)
}
// AppendChunk enforces an EXACT 4096-byte authenticated window unless EOF
// arrives earlier. It shares the existing private, SHA-pinned checkpoint and
// atomic no-overwrite publication with the legacy 128-byte transfer.
func (d *Download) AppendChunk(w remotetransport.FileWindow)(Progress,error){
 return d.appendBounded(w,remotetransport.RemoteFileChunkMaxBytes)
}
func (d *Download) appendBounded(w remotetransport.FileWindow,limit int)(Progress,error){
 if d==nil||d.closed||d.completed||d.file==nil{return Progress{},ErrUnsafeOutput}
 if limit!=remotetransport.RemoteFileWindowMaxBytes&&
  limit!=remotetransport.RemoteFileChunkMaxBytes{return d.Progress(),ErrInvalidWindow}
 remaining:=w.Size-w.Offset
 expectedLength:=int64(limit)
 if remaining<expectedLength{expectedLength=remaining}
 if w.Offset!=d.offset||w.Size<0||w.Size>remotetransport.RemoteFileWindowMaxSize||
  !validSHA(w.FileSHA256)||len(w.Data)>limit||
  remaining<0||int64(len(w.Data))!=expectedLength{
  return d.Progress(),ErrInvalidWindow
 }
 if d.initialized{
  if d.expected.SHA256!=w.FileSHA256||d.expected.Size!=w.Size{
   return d.Progress(),ErrChangedRemoteFile
  }
 }else{
  m:=manifest{DeviceID:d.expected.DeviceID,RemotePath:d.expected.RemotePath,
   Size:w.Size,SHA256:w.FileSHA256}
  raw,err:=encodeHeader(m)
  if err!=nil{return d.Progress(),err}
  if n,err:=d.file.WriteAt(raw,0);err!=nil||n!=len(raw){
   return d.Progress(),ErrUnsafeOutput
  }
  if err:=d.file.Sync();err!=nil{return d.Progress(),err}
  d.initialized=true
  d.expected=m
 }
 if len(w.Data)>0{
  n,err:=d.file.WriteAt(w.Data,headerSize+d.offset)
  if err!=nil||n!=len(w.Data){return d.Progress(),ErrUnsafeOutput}
  if err:=d.file.Sync();err!=nil{return d.Progress(),err}
  d.offset+=int64(n)
 }
 if d.offset==d.expected.Size{
  if err:=d.publish();err!=nil{return d.Progress(),err}
  d.completed=true
 }
 return d.Progress(),nil
}

// Publish final data ONLY after verifying the complete assembled SHA-256.
// A random private ready file and atomic hard-link create avoid overwriting
// existing paths even if another process races to claim the destination.
func (d *Download) publish()error{
 if d.closed||d.file==nil{return ErrUnsafeOutput}
 if err:=validateDestination(d.destination);err!=nil{return err}
 var random [16]byte
 if _,err:=rand.Read(random[:]);err!=nil{return err}
 ready:=d.destination+".taskdeck-ready-"+hex.EncodeToString(random[:])
 output,err:=openNew(ready)
 if err!=nil{return ErrUnsafeOutput}
 defer func(){_ =output.Close();_ =os.Remove(ready)}()
 body:=io.NewSectionReader(d.file,headerSize,d.expected.Size)
 digest:=sha256.New()
 copied,err:=io.Copy(io.MultiWriter(output,digest),body)
 if err!=nil||copied!=d.expected.Size||
  hex.EncodeToString(digest.Sum(nil))!=d.expected.SHA256{
  return ErrChangedRemoteFile
 }
 if err:=output.Sync();err!=nil{return err}
 if err:=os.Link(ready,d.destination);err!=nil{return ErrUnsafeOutput}
 _=os.Remove(d.partial)
 return nil
}
