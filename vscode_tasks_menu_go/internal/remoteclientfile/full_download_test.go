package remoteclientfile

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func blockWindow(data []byte,offset int)remotetransport.FileWindow{
 sum:=sha256.Sum256(data)
 end:=offset+remotetransport.RemoteFileBlockMaxBytes
 if end>len(data){end=len(data)}
 return remotetransport.FileWindow{
  Offset:int64(offset),Size:int64(len(data)),FileSHA256:hex.EncodeToString(sum[:]),
  Data:append([]byte(nil),data[offset:end]...),
 }
}
func TestNativeResumeAndAtomicVerifyComplete2MiBIn64FileBlocks(t *testing.T){
 data:=make([]byte,2<<20)
 for i:=range data{data[i]=byte(i%251)}
 target:=filepath.Join(t.TempDir(),"full.bin")
 d,err:=Open(target,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 for offset:=0;offset<12*32768;offset+=32768{
  if _,err:=d.AppendBlock(blockWindow(data,offset));err!=nil{t.Fatal(err)}
 }
 if err:=d.Close();err!=nil{t.Fatal(err)}
 if _,err:=os.Stat(target);!os.IsNotExist(err){t.Fatal("premature file publish")}
 d,err=Open(target,testDeviceID,testSource)
 if err!=nil||d.Progress().Offset!=12*32768{t.Fatalf("checkpoint not resumed %+v %v",d,err)}
 defer d.Close()
 for offset:=12*32768;offset<len(data);offset+=32768{
  prog,err:=d.AppendBlock(blockWindow(data,offset))
  if err!=nil{t.Fatalf("block offset=%d denied: %v",offset,err)}
  if offset+32768==len(data)&&!prog.Complete{t.Fatal("full 2MiB file not published")}
 }
 obtained,err:=os.ReadFile(target)
 if err!=nil||!bytes.Equal(obtained,data){t.Fatal("full local E2EE file SHA mismatch")}
 info,err:=os.Stat(target)
 if err!=nil||info.Mode().Perm()!=0600{t.Fatalf("wrong file permissions %v",err)}
 if _,err:=os.Lstat(target+partialSuffix);!os.IsNotExist(err){t.Fatal("checkpoint still present")}
}

func TestNativeBlockRejectsShortDataAndMixedFileVersion(t *testing.T){
 original:=bytes.Repeat([]byte("a"),65536)
 d,err:=Open(filepath.Join(t.TempDir(),"out.bin"),testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 defer d.Close()
 invalid:=blockWindow(original,0)
 invalid.Data=invalid.Data[:32767]
 if _,err:=d.AppendBlock(invalid);err!=ErrInvalidWindow{
  t.Fatalf("short 32KiB block accepted %v",err)
 }
 if _,err:=d.AppendBlock(blockWindow(original,0));err!=nil{t.Fatal(err)}
 changed:=bytes.Repeat([]byte("b"),65536)
 if _,err:=d.AppendBlock(blockWindow(changed,32768));err!=ErrChangedRemoteFile{
  t.Fatal("changed source file accepted during file-only download")
 }
}
