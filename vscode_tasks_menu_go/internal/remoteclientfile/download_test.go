package remoteclientfile

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const testDeviceID="aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
const testSource="project/visible.bin"

func windowFor(data []byte,offset int)remotetransport.FileWindow{
 sum:=sha256.Sum256(data)
 end:=offset+remotetransport.RemoteFileWindowMaxBytes
 if end>len(data){end=len(data)}
 return remotetransport.FileWindow{Offset:int64(offset),Size:int64(len(data)),
  FileSHA256:hex.EncodeToString(sum[:]),Data:append([]byte(nil),data[offset:end]...)}
}

func TestDownloadResumePrivateAndAtomicComplete(t *testing.T){
 content:=make([]byte,624)
 for i:=range content{content[i]=byte(i)}
 destination:=filepath.Join(t.TempDir(),"final.bin")
 partial:=destination+partialSuffix
 d,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 for offset:=0;offset<256;offset+=128{
  progress,err:=d.Append(windowFor(content,offset))
  if err!=nil||progress.Offset!=int64(offset+128)||progress.Complete{
   t.Fatalf("unexpected partial progress %+v %v",progress,err)
  }
 }
 if _,err:=os.Stat(destination);!os.IsNotExist(err){
  t.Fatal("partial bytes published before whole-file hash verification")
 }
 info,err:=os.Stat(partial)
 if err!=nil||info.Mode().Perm()!=0600{t.Fatalf("staging not owner-only: %v %v",info,err)}
 if err:=d.Close();err!=nil{t.Fatal(err)}
 resumed,err:=Open(destination,testDeviceID,testSource)
 if err!=nil||resumed.Progress().Offset!=256{t.Fatalf("failed resume: %+v %v",resumed,err)}
 defer resumed.Close()
 for offset:=256;offset<len(content);offset+=128{
  progress,err:=resumed.Append(windowFor(content,offset))
  if err!=nil{t.Fatalf("resume offset=%d: %v",offset,err)}
  if offset+128>=len(content)&&!progress.Complete{
   t.Fatalf("complete result not published: %+v",progress)
  }
 }
 raw,err:=os.ReadFile(destination)
 if err!=nil||!bytes.Equal(content,raw){t.Fatalf("wrong assembled final file: %v",err)}
 info,err=os.Stat(destination)
 if err!=nil||info.Mode().Perm()!=0600{t.Fatalf("final file not owner-only: %v %v",info,err)}
 if _,err:=os.Lstat(partial);!os.IsNotExist(err){t.Fatal("staging file persisted after success")}
}

func TestDownloadRejectsChangedVersionWithoutPublishing(t *testing.T){
 data:=bytes.Repeat([]byte("A"),256)
 destination:=filepath.Join(t.TempDir(),"output.bin")
 d,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 if _,err:=d.Append(windowFor(data,0));err!=nil{t.Fatal(err)}
 if err:=d.Close();err!=nil{t.Fatal(err)}
 changed:=bytes.Repeat([]byte("B"),256)
 resumed,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 defer resumed.Close()
 if _,err:=resumed.Append(windowFor(changed,128));err!=ErrChangedRemoteFile{
  t.Fatalf("version mismatch accepted: %v",err)
 }
 if resumed.Progress().Offset!=128{t.Fatal("version mismatch advanced checkpoint")}
 if _,err:=os.Stat(destination);!os.IsNotExist(err){t.Fatal("changed file exposed to caller")}
}

func TestDownloadDetectsCorruptedLocalPartialAtFinish(t *testing.T){
 data:=bytes.Repeat([]byte("hello"),40)
 destination:=filepath.Join(t.TempDir(),"whole.bin")
 d,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 if _,err:=d.Append(windowFor(data,0));err!=nil{t.Fatal(err)}
 if err:=d.Close();err!=nil{t.Fatal(err)}
 part:=destination+partialSuffix
 f,err:=os.OpenFile(part,os.O_WRONLY,0600)
 if err!=nil{t.Fatal(err)}
 if _,err:=f.WriteAt([]byte("bad"),headerSize);err!=nil{t.Fatal(err)}
 f.Close()
 resumed,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 defer resumed.Close()
 if _,err:=resumed.Append(windowFor(data,128));err!=ErrChangedRemoteFile{
  t.Fatalf("modified local prefix accepted: %v",err)
 }
 if _,err:=os.Stat(destination);!os.IsNotExist(err){t.Fatal("tampered data published")}
}

func TestDownloadRejectsLocalSymlinkAndCrossIdentityResume(t *testing.T){
 dir:=t.TempDir()
 final:=filepath.Join(dir,"output")
 if err:=os.Symlink(filepath.Join(dir,"secret"),final+partialSuffix);err!=nil{t.Fatal(err)}
 if _,err:=Open(final,testDeviceID,testSource);err==nil{
  t.Fatal("local partial symlink was followed")
 }
 if err:=os.Remove(final+partialSuffix);err!=nil{t.Fatal(err)}
 d,err:=Open(final,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 data:=bytes.Repeat([]byte("a"),256)
 if _,err:=d.Append(windowFor(data,0));err!=nil{t.Fatal(err)}
 if _,err:=Open(final,testDeviceID,testSource);err==nil{
  t.Fatal("concurrent reader bypassed exclusive staging lock")
 }
 d.Close()
 for _,pair:=range [][2]string{
  {strings.Repeat("f",64),testSource},
  {testDeviceID,"different/path"},
 }{
  if _,err:=Open(final,pair[0],pair[1]);err==nil{
   t.Fatal("staged data resumed with wrong host identity or remote path")
  }
 }
}

func TestDownloadNoClobberAndHeaderValidation(t *testing.T){
 dir:=t.TempDir()
 final:=filepath.Join(dir,"output")
 if err:=os.WriteFile(final,[]byte("must stay"),0600);err!=nil{t.Fatal(err)}
 if _,err:=Open(final,testDeviceID,testSource);err==nil{t.Fatal("existing destination overwritten")}
 old,err:=os.ReadFile(final)
 if err!=nil||string(old)!="must stay"{t.Fatal("existing file was damaged")}
 os.Remove(final)
 d,err:=Open(final,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 if _,err:=d.Append(windowFor(bytes.Repeat([]byte("x"),256),0));err!=nil{t.Fatal(err)}
 d.Close()
 part:=final+partialSuffix
 f,err:=os.OpenFile(part,os.O_WRONLY,0600)
 if err!=nil{t.Fatal(err)}
 if _,err:=f.WriteAt([]byte("evil"),0);err!=nil{t.Fatal(err)}
 f.Close()
 if _,err:=Open(final,testDeviceID,testSource);err==nil{
  t.Fatal("tampered resume manifest accepted")
 }
}

func TestDownloadLocalDestinationAndWindowFailsClosed(t *testing.T){
 for _,source:=range []string{"../secret",".git/config","/etc/passwd","","one\\two"}{
  if _,err:=Open(filepath.Join(t.TempDir(),"out"),testDeviceID,source);
   err==nil&&source!="" { // dot path is host-checked but incomplete paths may pass here
   if source=="../secret"||source=="/etc/passwd"||source=="one\\two"{
    t.Fatalf("unsafe source was accepted: %q",source)
   }
  }
 }
 d,err:=Open(filepath.Join(t.TempDir(),"output"),testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 defer d.Close()
 data:=bytes.Repeat([]byte("a"),256)
 w:=windowFor(data,0)
 w.Offset=128
 if _,err:=d.Append(w);err==nil{t.Fatal("out-of-order window accepted")}
 w=windowFor(data,0)
 w.Data=make([]byte,129)
 if _,err:=d.Append(w);err==nil{t.Fatal("oversized window accepted")}
 w=windowFor(data,0)
 w.FileSHA256="not-a-sha"
 if _,err:=d.Append(w);err==nil{t.Fatal("invalid full-file hash accepted")}
 if _,err:=d.Append(windowFor(data,0));err!=nil{t.Fatal(err)}
 // No success can publish without complete bytes and an exact full-file hash.
 if _,err:=os.Stat(d.destination);!os.IsNotExist(err){t.Fatal("partial exposed")}
}
