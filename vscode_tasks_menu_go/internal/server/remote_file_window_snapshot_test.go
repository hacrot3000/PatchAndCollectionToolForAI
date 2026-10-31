package server

import (
 "os"
 "path/filepath"
 "testing"
 "time"
)

// Real filesystem metadata snapshots, rather than invented FileInfo fields,
// exercise the check used after an E2EE window's descriptor-pinned read.
func TestRemoteFileWindowSnapshotStableAndRejectsTruncation(t *testing.T){
 path:=filepath.Join(t.TempDir(),"visible.bin")
 data:=[]byte("known-version")
 if err:=os.WriteFile(path,data,0600);err!=nil{t.Fatal(err)}
 before,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 after,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if !remoteFileWindowSnapshotStable(before,after,len(data)){
  t.Fatal("unchanged descriptor-pinned file was refused")
 }
 if remoteFileWindowSnapshotStable(before,after,len(data)-1){
  t.Fatal("a truncated or partial read was accepted as a full-file digest")
 }
 if err:=os.Truncate(path,3);err!=nil{t.Fatal(err)}
 after,err=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if remoteFileWindowSnapshotStable(before,after,len(data)){
  t.Fatal("concurrent truncate did not invalidate the file window")
 }
}

func TestRemoteFileWindowSnapshotRejectsSameSizeRewriteAndSwap(t *testing.T){
 path:=filepath.Join(t.TempDir(),"file.bin")
 if err:=os.WriteFile(path,[]byte("AAAA"),0600);err!=nil{t.Fatal(err)}
 before,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("BBBB"),0600);err!=nil{t.Fatal(err)}
 // Set a specific later timestamp so this test cannot depend on the host
 // filesystem's timestamp granularity.
 changedTime:=before.ModTime().Add(3*time.Second)
 if err:=os.Chtimes(path,changedTime,changedTime);err!=nil{t.Fatal(err)}
 after,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if remoteFileWindowSnapshotStable(before,after,4){
  t.Fatal("same-size in-place rewrite was treated as an unchanged version")
 }

 original:=filepath.Join(filepath.Dir(path),"old.bin")
 if err:=os.Rename(path,original);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("AAAA"),0600);err!=nil{t.Fatal(err)}
 replacement,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if remoteFileWindowSnapshotStable(before,replacement,4){
  t.Fatal("same-size leaf replacement bypassed pinned-inode check")
 }
}

func TestRemoteFileWindowSnapshotRejectsModeChange(t *testing.T){
 path:=filepath.Join(t.TempDir(),"visible.bin")
 if err:=os.WriteFile(path,[]byte("payload"),0600);err!=nil{t.Fatal(err)}
 before,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if err:=os.Chmod(path,0640);err!=nil{t.Fatal(err)}
 after,err:=os.Stat(path)
 if err!=nil{t.Fatal(err)}
 if remoteFileWindowSnapshotStable(before,after,len("payload")){
  t.Fatal("permission change while reading did not invalidate the snapshot")
 }
 if remoteFileWindowSnapshotStable(nil,after,len("payload"))||
  remoteFileWindowSnapshotStable(before,nil,len("payload")){
  t.Fatal("missing metadata was accepted")
 }
}
