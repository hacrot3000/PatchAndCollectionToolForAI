package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
)

func TestVerifiedAppendWriterNeverMixesChangedPrefix(t *testing.T) {
	var out bytes.Buffer
	writer:=&verifiedAppendWriter{dst:&out,tail:[]byte("original-line\n")}
	for _,part:=range [][]byte{[]byte("origi"),[]byte("nal-line\nappend"),[]byte("-50\n")} {
		n,err:=writer.Write(part)
		if err!=nil||n!=len(part){t.Fatalf("append %d bytes failed: %d %v",len(part),n,err)}
	}
	if !writer.validated()||out.String()!="append-50\n" {t.Fatalf("unexpected verified append: %q",out.String())}
	out.Reset()
	changed:=&verifiedAppendWriter{dst:&out,tail:[]byte("original-line\n")}
	_,err:=changed.Write([]byte("totally-new-log-150-lines\n"))
	if !errors.Is(err,errLiveSourceChanged){t.Fatalf("expected replacement detection: %v",err)}
	if out.Len()!=0{t.Fatalf("changed bytes polluted saved file: %q",out.String())}
	short:=&verifiedAppendWriter{dst:&out,tail:[]byte("old-long-header")}
	_,err=short.Write([]byte("old"))
	if err!=nil||short.validated(){t.Fatal("truncated overlap must not be considered verified")}
}

func TestPartialOverlapHandlesSparseMultiGigabyteFile(t *testing.T) {
	file,err:=os.Create(filepath.Join(t.TempDir(),"large.part"))
	if err!=nil{t.Fatal(err)}
	defer file.Close()
	const size int64=3<<30
	if err=file.Truncate(size);err!=nil{t.Fatal(err)}
	tail,start,err:=readPartialOverlap(file,size)
	if err!=nil{t.Fatal(err)}
	if len(tail)!=int(liveFileOverlap)||start!=size-liveFileOverlap {t.Fatalf("unexpected sparse overlap: %d %d",len(tail),start)}
}

func makeFakeLiveLogSFTP(t *testing.T,source,appendMarker string) string {
	t.Helper()
	if runtime.GOOS=="windows"{t.Skip("POSIX sftp fixture")}
	script:=filepath.Join(t.TempDir(),"fake-live-sftp")
	body:=fmt.Sprintf("#!/bin/sh\nremote=%q\nmarker=%q\ninput=$(cat)\ncase \"$input\" in\n  *\"ls -lan \"*)\n    size=$(wc -c < \"$remote\")\n    printf '%%s\\n' \"-rw-r--r-- 1 1000 1000 $size Oct 12 14:00 live.log\"\n    exit 0\n    ;;\nesac\nlocal_path=$(printf '%%s\\n' \"$input\" | awk -F '\"' '/^get / {print $4}')\nif [ -n \"$local_path\" ]; then\n  cp \"$remote\" \"$local_path\" || exit 1\n  if [ ! -e \"$marker\" ]; then\n    printf 'appended-after-first-download\\n' >> \"$remote\"\n    : > \"$marker\"\n  fi\n  exit 0\nfi\nlocal_path=$(printf '%%s\\n' \"$input\" | awk -F '\"' '/^reget / {print $4}')\nif [ -n \"$local_path\" ]; then\n  offset=$(wc -c < \"$local_path\")\n  dd if=\"$remote\" of=\"$local_path\" bs=1 skip=\"$offset\" seek=\"$offset\" conv=notrunc 2>/dev/null\n  exit $?\nfi\nexit 1\n",source,appendMarker)
	if err:=os.WriteFile(script,[]byte(body),0700);err!=nil{t.Fatal(err)}
	return script
}

func liveLogTransferFixture(t *testing.T,initial []byte,preventAppend bool) (*Server,string,string) {
	t.Helper()
	s,store,_:=newFileTransferProfileAPITestServer(t)
	profile,err:=store.Create(filetransferprofile.Profile{
		ID:"live-log",Name:"Live log",Protocol:filetransferprofile.ProtocolSFTP,SSHProfileID:"ssh-prod",
	})
	if err!=nil{t.Fatal(err)}
	s.Workspace=t.TempDir()
	if err=os.Mkdir(filepath.Join(s.Workspace,"downloads"),0700);err!=nil{t.Fatal(err)}
	remote:=filepath.Join(t.TempDir(),"remote.log")
	if err=os.WriteFile(remote,initial,0600);err!=nil{t.Fatal(err)}
	marker:=filepath.Join(t.TempDir(),"first-get-done")
	if preventAppend{
		if err=os.WriteFile(marker,[]byte("yes"),0600);err!=nil{t.Fatal(err)}
	}
	s.SFTPExecutable=makeFakeLiveLogSFTP(t,remote,marker)
	return s,profile.ID,remote
}

func TestSFTPDownloadFollowsAppendedGameLog(t *testing.T) {
	original:=bytes.Repeat([]byte("game-event-0001\n"),7)
	s,profileID,remote:=liveLogTransferFixture(t,original,false)
	var progress []int64
	err:=s.backgroundRemoteToHostGrowing(context.Background(),profileID,
		"/game/live.log","downloads/live.log",false,int64(len(original)),
		func(n int64){progress=append(progress,n)})
	if err!=nil{t.Fatal(err)}
	got,err:=os.ReadFile(filepath.Join(s.Workspace,"downloads","live.log"))
	if err!=nil{t.Fatal(err)}
	want:=append(append([]byte(nil),original...),[]byte("appended-after-first-download\n")...)
	if !bytes.Equal(got,want){t.Fatalf("growing log got=%q want=%q",got,want)}
	actualRemote,err:=os.ReadFile(remote)
	if err!=nil{t.Fatal(err)}
	if !bytes.Equal(got,actualRemote){t.Fatal("host copy differs from append-only remote log")}
	if len(progress)==0||progress[len(progress)-1]!=int64(len(want)){t.Fatalf("progress not updated after growth: %v",progress)}
}

func TestSFTPDownloadRotatedLogNeverMixesOldAndNew(t *testing.T) {
	newContent:=[]byte("new-log-line-1\nnew-log-line-2\nnew-log-line-3\n")
	s,profileID,_:=liveLogTransferFixture(t,newContent,true)
	host:=filepath.Join(s.Workspace,"downloads","live.log")
	partial:=fileTransferPartialHostPath(host,"/game/live.log")
	if err:=os.WriteFile(partial,[]byte("completely-old-log\n"),0600);err!=nil{t.Fatal(err)}
	var sawRestart bool
	err:=s.backgroundRemoteToHostGrowing(context.Background(),profileID,
		"/game/live.log","downloads/live.log",false,0,
		func(n int64){if n==-1{sawRestart=true}})
	if err!=nil{t.Fatal(err)}
	got,err:=os.ReadFile(host)
	if err!=nil{t.Fatal(err)}
	if !bytes.Equal(got,newContent){t.Fatalf("rotated file was mixed: %q",got)}
	if !sawRestart {t.Fatal("rotated log did not trigger clean retry")}
}

func TestSFTPDownloadNoBlindMergeAfterRepeatedRotation(t *testing.T) {
	file:=filepath.Join(t.TempDir(),"part")
	if err:=os.WriteFile(file,[]byte("previous-data"),0600);err!=nil{t.Fatal(err)}
	f,err:=os.OpenFile(file,os.O_RDWR,0600)
	if err!=nil{t.Fatal(err)}
	defer f.Close()
	tail,_,err:=readPartialOverlap(f,13)
	if err!=nil{t.Fatal(err)}
	var got bytes.Buffer
	w:=&verifiedAppendWriter{dst:&got,tail:tail}
	_,err=w.Write([]byte("another-wholly-new-file"))
	if !errors.Is(err,errLiveSourceChanged)||got.Len()!=0{t.Fatalf("unsafe resume: %v %q",err,got.String())}
	if !strings.Contains(err.Error(),"rotated"){t.Fatal("error should explain why appending was rejected")}
}

func TestSFTPUploadResumeRequiresMatchingRemoteTail(t *testing.T){
	remoteBytes:=[]byte("first-half-of-file")
	s,profileID,remote:=liveLogTransferFixture(t,remoteBytes,true)
	profile,err:=s.resolveFileTransferProfile(profileID)
	if err!=nil{t.Fatal(err)}
	source:=filepath.Join(t.TempDir(),"upload-source")
	if err=os.WriteFile(source,append(append([]byte(nil),remoteBytes...),[]byte("-more-data")...),0600);err!=nil{t.Fatal(err)}
	offset:=int64(len(remoteBytes))
	ok,err:=s.uploadResumeTailMatches(context.Background(),profile,"/game/live.log",source,offset)
	if err!=nil||!ok{t.Fatalf("unchanged remote upload tail must allow resume: ok=%v err=%v",ok,err)}
	if err=os.WriteFile(remote,[]byte("completely-different"),0600);err!=nil{t.Fatal(err)}
	ok,err=s.uploadResumeTailMatches(context.Background(),profile,"/game/live.log",source,offset)
	if err!=nil{t.Fatal(err)}
	if ok{t.Fatal("different remote upload bytes must force full overwrite, never blind resume")}
	if err=os.WriteFile(remote,append(append([]byte(nil),remoteBytes...),[]byte("extra")...),0600);err!=nil{t.Fatal(err)}
	ok,err=s.uploadResumeTailMatches(context.Background(),profile,"/game/live.log",source,offset)
	if err!=nil{t.Fatal(err)}
	if ok{t.Fatal("remote grew unexpectedly; previous resume offset is no longer valid")}
}
