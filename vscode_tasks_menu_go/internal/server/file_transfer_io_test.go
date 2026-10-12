package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
)

func TestRemoteDownloadName(t *testing.T) {
	for input, want := range map[string]string{
		"/srv/app/file.txt": "file.txt",
		"C:\\drop\\file.zip": "file.zip",
		"/": "download",
	} {
		if got := remoteDownloadName(input); got != want {
			t.Fatalf("remoteDownloadName(%q)=%q want %q", input, got, want)
		}
	}
}

func TestFileTransferSFTPDownloadTicketWorksWithActiveBrowserLease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-download", Name: "SFTP Download", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ninput=$(cat)\nlocal_path=$(printf '%s\\n' \"$input\" | sed -n 's/^get \"[^\"]*\" \"\\([^\"]*\\)\"$/\\1/p')\nprintf 'hello-sftp' > \"$local_path\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	leaseReq := httptest.NewRequest(http.MethodPost, "/api/browser/lease", nil)
	leaseRR := httptest.NewRecorder()
	h.ServeHTTP(leaseRR, leaseReq)
	if leaseRR.Code != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", leaseRR.Code, leaseRR.Body.String())
	}
	var lease struct {
		Lease string `json:"lease"`
	}
	if err := json.Unmarshal(leaseRR.Body.Bytes(), &lease); err != nil {
		t.Fatal(err)
	}
	if lease.Lease == "" {
		t.Fatal("empty browser lease")
	}

	ticketReq := httptest.NewRequest(http.MethodPost, "/api/file-transfer/download-ticket", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID, "path": "/srv/app/hello.txt",
	}))
	ticketReq.Header.Set("Content-Type", "application/json")
	ticketReq.Header.Set(browserLeaseHeader, lease.Lease)
	ticketRR := httptest.NewRecorder()
	h.ServeHTTP(ticketRR, ticketReq)
	if ticketRR.Code != http.StatusCreated {
		t.Fatalf("ticket status=%d body=%s", ticketRR.Code, ticketRR.Body.String())
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(ticketRR.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	if ticket.Ticket == "" {
		t.Fatal("empty download ticket")
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/file-transfer/download?ticket="+ticket.Ticket, nil)
	downloadRR := httptest.NewRecorder()
	h.ServeHTTP(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", downloadRR.Code, downloadRR.Body.String())
	}
	if downloadRR.Body.String() != "hello-sftp" {
		t.Fatalf("body=%q", downloadRR.Body.String())
	}

	if length:=downloadRR.Header().Get("Content-Length");length!=""{
		t.Fatalf("a live SFTP log must not advertise obsolete content length %q",length)
	}
	if disposition := downloadRR.Header().Get("Content-Disposition"); !strings.Contains(disposition, "hello.txt") {
		t.Fatalf("Content-Disposition=%q", disposition)
	}

	progressReq:=httptest.NewRequest(http.MethodGet,"/api/file-transfer/download-progress?ticket="+ticket.Ticket,nil)
	progressRR:=httptest.NewRecorder()
	h.ServeHTTP(progressRR,progressReq)
	if progressRR.Code!=http.StatusOK {t.Fatalf("progress status=%d body=%s",progressRR.Code,progressRR.Body.String())}
	var progress struct {
		Status string `json:"status"`
		BytesDone int64 `json:"bytes_done"`
		BytesTotal int64 `json:"bytes_total"`
	}
	if err:=json.Unmarshal(progressRR.Body.Bytes(),&progress);err!=nil {t.Fatal(err)}
	if progress.Status!="success"||progress.BytesDone!=int64(len("hello-sftp")) {
		t.Fatalf("download completed but progress missing: %+v",progress)
	}

	replayRR := httptest.NewRecorder()
	h.ServeHTTP(replayRR, httptest.NewRequest(http.MethodGet, "/api/file-transfer/download?ticket="+ticket.Ticket, nil))
	if replayRR.Code != http.StatusNotFound {
		t.Fatalf("replay status=%d body=%s", replayRR.Code, replayRR.Body.String())
	}
}

func TestFileTransferSFTPUploadUsesPutCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-upload", Name: "SFTP Upload", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.txt")
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ncat > \"" + capture + "\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	if err := writer.WriteField("profile_id", profile.ID); err != nil { t.Fatal(err) }
	if err := writer.WriteField("path", "/srv/app/new file.txt"); err != nil { t.Fatal(err) }
	part, err := writer.CreateFormFile("file", "new file.txt")
	if err != nil { t.Fatal(err) }
	if _, err := io.WriteString(part, "upload-data"); err != nil { t.Fatal(err) }
	if err := writer.Close(); err != nil { t.Fatal(err) }

	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/upload", &payload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(capture)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "put \"") || !strings.Contains(string(data), "\"/srv/app/new file.txt\"") {
		t.Fatalf("unexpected sftp command %q", data)
	}
}

func TestNativeDownloadProgressOnlyAccessibleWithValidTicket(t *testing.T) {
	s,_,_:=newFileTransferProfileAPITestServer(t)
	rr:=httptest.NewRecorder()
	s.Handler().ServeHTTP(rr,httptest.NewRequest(http.MethodGet,"/api/file-transfer/download-progress?ticket=invalid",nil))
	if rr.Code!=http.StatusNotFound {t.Fatalf("unexpected status %d: %s",rr.Code,rr.Body.String())}
}

func TestStagedBrowserUploadReturnsImmediatelyAndUsesDaemonQueue(t *testing.T) {
	if runtime.GOOS=="windows" {t.Skip("POSIX shell fixture")}
	s,store,_:=newFileTransferProfileAPITestServer(t)
	profile,err:=store.Create(filetransferprofile.Profile{
		ID:"browser-staged",Name:"Browser staged upload",Protocol:filetransferprofile.ProtocolSFTP,SSHProfileID:"ssh-prod",
	})
	if err!=nil{t.Fatal(err)}
	dir:=t.TempDir()
	script:=filepath.Join(dir,"fake-sftp")
	if err:=os.WriteFile(script,[]byte("#!/bin/sh\ncat >/dev/null\n"),0700);err!=nil{t.Fatal(err)}
	s.SFTPExecutable=script
	var payload bytes.Buffer
	writer:=multipart.NewWriter(&payload)
	if err:=writer.WriteField("profile_id",profile.ID);err!=nil{t.Fatal(err)}
	if err:=writer.WriteField("path","/remote/large.bin");err!=nil{t.Fatal(err)}
	if err:=writer.WriteField("background","1");err!=nil{t.Fatal(err)}
	part,err:=writer.CreateFormFile("file","large.bin")
	if err!=nil{t.Fatal(err)}
	if _,err:=io.WriteString(part,"simulated-transfer");err!=nil{t.Fatal(err)}
	if err:=writer.Close();err!=nil{t.Fatal(err)}
	req:=httptest.NewRequest(http.MethodPost,"/api/file-transfer/upload",&payload)
	req.Header.Set("Content-Type",writer.FormDataContentType())
	rr:=httptest.NewRecorder()
	s.Handler().ServeHTTP(rr,req)
	if rr.Code!=http.StatusAccepted{t.Fatalf("staged HTTP %d: %s",rr.Code,rr.Body.String())}
	var resp struct{JobID string `json:"job_id"`}
	if err:=json.Unmarshal(rr.Body.Bytes(),&resp);err!=nil{t.Fatal(err)}
	if resp.JobID==""{t.Fatal("async upload job missing")}
	var source string
	deadline:=time.Now().Add(8*time.Second)
	for time.Now().Before(deadline){
		snap:=s.fileTransferServerQueue(profile.ID).snapshot()
		for _,item:=range snap.Items{
			if item.JobID!=resp.JobID {continue}
			source=item.Source
			if item.Size!=int64(len("simulated-transfer")){t.Fatalf("incorrect staging size %d",item.Size)}
			if item.Status=="failed" {t.Fatalf("background upload failed: %s",item.Error)}
			if item.Status=="success"{
				if _,err:=os.Stat(source);!os.IsNotExist(err){t.Fatalf("staging not cleaned on success: %v",err)}
				return
			}
		}
		time.Sleep(50*time.Millisecond)
	}
	t.Fatalf("background upload never completed: source=%s",source)
}

func TestFileTransferBrowserProgressGrowsBeyondStaleRemoteMetadata(t *testing.T){
	s:=&Server{fileTransferDownloadProgress:map[string]*fileTransferBrowserProgress{
		"browser-ticket":{Status:"running",BytesTotal:100},
	}}
	s.updateBrowserDownloadProgress("browser-ticket","running",150,100,"")
	progress:=s.fileTransferDownloadProgress["browser-ticket"]
	if progress.BytesDone!=150||progress.BytesTotal!=150 {
		t.Fatalf("live browser stream truncated by original source size: %+v",progress)
	}
	s.updateBrowserDownloadProgress("browser-ticket","success",3<<30,150,"")
	if progress.Status!="success"||progress.BytesDone!=3<<30||progress.BytesTotal!=3<<30{
		t.Fatalf("3GiB progress not preserved: %+v",progress)
	}
}
