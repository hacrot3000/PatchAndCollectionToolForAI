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

func chunkWindow(data []byte,offset int)remotetransport.FileWindow{
 sum:=sha256.Sum256(data)
 end:=offset+remotetransport.RemoteFileChunkMaxBytes
 if end>len(data){end=len(data)}
 return remotetransport.FileWindow{
  Offset:int64(offset),Size:int64(len(data)),
  FileSHA256:hex.EncodeToString(sum[:]),
  Data:append([]byte(nil),data[offset:end]...),
 }
}

func TestBulkDownloadPrivateResumeAndFileSHA(t *testing.T){
 data:=make([]byte,10000)
 for i:=range data{data[i]=byte(i%256)}
 destination:=filepath.Join(t.TempDir(),"full.bin")
 d,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 first,err:=d.AppendChunk(chunkWindow(data,0))
 if err!=nil||first.Offset!=4096||first.Complete{t.Fatalf("bad first bulk chunk %+v %v",first,err)}
 if err:=d.Close();err!=nil{t.Fatal(err)}
 d,err=Open(destination,testDeviceID,testSource)
 if err!=nil||d.Progress().Offset!=4096{t.Fatalf("bad checkpoint %+v %v",d,err)}
 defer d.Close()
 if _,err:=d.AppendChunk(chunkWindow(data,4096));err!=nil{t.Fatal(err)}
 last,err:=d.AppendChunk(chunkWindow(data,8192))
 if err!=nil||!last.Complete{t.Fatalf("bulk download failed: %+v %v",last,err)}
 read,err:=os.ReadFile(destination)
 if err!=nil||!bytes.Equal(read,data){t.Fatal("assembled full file failed SHA verification")}
 if _,err:=os.Stat(destination+partialSuffix);!os.IsNotExist(err){
  t.Fatal("completed partial checkpoint was not cleared")
 }
}

func TestBulkDownloadRejectsShortChunkAndVersionChange(t *testing.T){
 original:=bytes.Repeat([]byte("A"),8192)
 destination:=filepath.Join(t.TempDir(),"whole.bin")
 d,err:=Open(destination,testDeviceID,testSource)
 if err!=nil{t.Fatal(err)}
 defer d.Close()
 short:=chunkWindow(original,0)
 short.Data=short.Data[:4095]
 if _,err:=d.AppendChunk(short);err!=ErrInvalidWindow{t.Fatal("short bulk chunk accepted")}
 if _,err:=d.AppendChunk(chunkWindow(original,0));err!=nil{t.Fatal(err)}
 changed:=bytes.Repeat([]byte("B"),8192)
 if _,err:=d.AppendChunk(chunkWindow(changed,4096));err!=ErrChangedRemoteFile{
  t.Fatal("changed E2EE file version silently resumed")
 }
 if _,err:=os.Stat(destination);!os.IsNotExist(err){t.Fatal("unverified bytes made visible")}
}
