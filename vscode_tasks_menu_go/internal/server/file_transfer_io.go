package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

const (
	maxFileTransferBytes          int64 = 1 << 36 // 64 GiB safety boundary, streamed rather than loaded in memory.
	fileTransferDownloadTicketTTL       = 90 * time.Second
	maxFileTransferDownloadTickets      = 128
)

// Progress of a browser-native download is tracked by its unguessable
// one-use ticket. The byte count includes SFTP staging or FTP streaming;
// success means handed to the HTTP client, not necessarily saved to disk.
type fileTransferBrowserProgress struct {
	Status string `json:"status"`
	BytesDone int64 `json:"bytes_done"`
	BytesTotal int64 `json:"bytes_total,omitempty"`
	Error string `json:"error,omitempty"`
	UpdatedAt time.Time `json:"-"`
}
func (s *Server) updateBrowserDownloadProgress(token, status string, done, total int64, message string){
	s.fileTransferDownloadMu.Lock()
	defer s.fileTransferDownloadMu.Unlock()
	progress:=s.fileTransferDownloadProgress[token]
	if progress==nil {return}
	if status!="" {progress.Status=status}
	if done>=progress.BytesDone {progress.BytesDone=done}
	if total>progress.BytesTotal {progress.BytesTotal=total}
	// A live log may be appended while streaming. Keep the progress total
	// at least as large as bytes actually sent, not the old scanned length.
	if done>progress.BytesTotal {progress.BytesTotal=done}
	if message!="" {progress.Error=message}
	progress.UpdatedAt=time.Now()
}
func (s *Server) fileTransferDownloadProgressAPI(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
	token:=strings.TrimSpace(r.URL.Query().Get("ticket"))
	s.fileTransferDownloadMu.Lock()
	progress:=s.fileTransferDownloadProgress[token]
	var state fileTransferBrowserProgress
	if progress!=nil {state=*progress}
	s.fileTransferDownloadMu.Unlock()
	if progress==nil {http.Error(w,"download tracking ticket not found or expired",http.StatusNotFound);return}
	writeJSON(w,http.StatusOK,state)
}

type fileTransferDownloadTicket struct {
	ProfileID string
	Path      string
	ExpiresAt time.Time
}

func newFileTransferDownloadTicketToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate file-transfer download ticket: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func (s *Server) issueFileTransferDownloadTicket(profileID, remotePath string) (string, error) {
	profileID = strings.TrimSpace(profileID)
	remotePath = strings.TrimSpace(remotePath)
	if profileID == "" || remotePath == "" {
		return "", errors.New("saved file-transfer profile id and remote path are required")
	}
	if _, err := s.resolveFileTransferProfile(profileID); err != nil {
		return "", err
	}
	token, err := newFileTransferDownloadTicketToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.fileTransferDownloadMu.Lock()
	defer s.fileTransferDownloadMu.Unlock()
	if s.fileTransferDownloads == nil {
		s.fileTransferDownloads = make(map[string]fileTransferDownloadTicket)
	}
	for key, ticket := range s.fileTransferDownloads {
		if !ticket.ExpiresAt.After(now) {delete(s.fileTransferDownloads,key)}
	}
	for key, entry:=range s.fileTransferDownloadProgress {
		if now.Sub(entry.UpdatedAt)>24*time.Hour {delete(s.fileTransferDownloadProgress,key)}
	}
	if len(s.fileTransferDownloads) >= maxFileTransferDownloadTickets {
		return "", errors.New("too many pending file-transfer downloads")
	}
	s.fileTransferDownloads[token] = fileTransferDownloadTicket{
		ProfileID: profileID,Path:remotePath,ExpiresAt:now.Add(fileTransferDownloadTicketTTL),
	}
	if s.fileTransferDownloadProgress==nil {s.fileTransferDownloadProgress=make(map[string]*fileTransferBrowserProgress)}
	s.fileTransferDownloadProgress[token]=&fileTransferBrowserProgress{Status:"queued",UpdatedAt:now}
	return token, nil
}

func (s *Server) consumeFileTransferDownloadTicket(token string) (fileTransferDownloadTicket, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return fileTransferDownloadTicket{}, false
	}
	now := time.Now()
	s.fileTransferDownloadMu.Lock()
	defer s.fileTransferDownloadMu.Unlock()
	for key, ticket := range s.fileTransferDownloads {
		if !ticket.ExpiresAt.After(now) {
			delete(s.fileTransferDownloads, key)
		}
	}
	ticket, ok := s.fileTransferDownloads[token]
	if !ok {
		return fileTransferDownloadTicket{}, false
	}
	delete(s.fileTransferDownloads, token)
	return ticket, true
}

type downloadResponseTracker struct {
	http.ResponseWriter
	wrote bool
}

func (w *downloadResponseTracker) Write(p []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(p)
}

func (w *downloadResponseTracker) WriteHeader(statusCode int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(statusCode)
}

type transferLimitWriter struct {
	dst       io.Writer
	remaining int64
}

func (w *transferLimitWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
	}
	if int64(len(p)) > w.remaining {
		allowed := int(w.remaining)
		if allowed > 0 {
			if _, err := w.dst.Write(p[:allowed]); err != nil {
				return 0, err
			}
		}
		w.remaining = 0
		return allowed, fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
	}
	n, err := w.dst.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func remoteDownloadName(remotePath string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(remotePath), "\\", "/")
	name := pathpkg.Base(normalized)
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, name)
	if strings.TrimSpace(name) == "" {
		return "download"
	}
	return name
}

func setRemoteDownloadHeaders(w http.ResponseWriter, name string) {
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disposition == "" {
		disposition = `attachment; filename="download"`
	}
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
}

func (s *Server) fileTransferDownloadTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeFileTransferOperationRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Profile != nil {
		http.Error(w, "saved file-transfer profile id is required", http.StatusBadRequest)
		return
	}
	token, err := s.issueFileTransferDownloadTicket(req.ProfileID, req.Path)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "too many pending") {
			status = http.StatusTooManyRequests
		} else if strings.Contains(err.Error(), "profile not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"ticket": token})
}

func (s *Server) fileTransferDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token:=r.URL.Query().Get("ticket")
	ticket, ok := s.consumeFileTransferDownloadTicket(token)
	if !ok {
		http.Error(w, "download ticket not found or expired", http.StatusNotFound)
		return
	}
	s.updateBrowserDownloadProgress(token,"running",0,0,"")
	req := fileTransferOperationRequest{ProfileID: ticket.ProfileID, Path: ticket.Path}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	release, ok := s.tryAcquireFileTransferBrowse(profile.ID)
	if !ok {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-TaskDeck-Transfer-Pool-Full", "1")
		http.Error(w, "FTP/SFTP connection pool is full", http.StatusTooManyRequests)
		return
	}
	defer release()
	name := remoteDownloadName(req.Path)
	tracked := &downloadResponseTracker{ResponseWriter: w}

	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		err = s.withFTPClient(r.Context(), profile, nil, func(client *ftpclient.Client) error {
			setRemoteDownloadHeaders(w, name)
			progressWriter:=&fileTransferProgressWriter{writer:tracked,callback:func(n int64){s.updateBrowserDownloadProgress(token,"running",n,0,"")}}
			writer := &transferLimitWriter{dst: progressWriter, remaining: maxFileTransferBytes}
			return client.Retrieve(r.Context(), req.Path, writer)
		})
	case filetransferprofile.ProtocolSFTP:
		// OpenSSH SFTP downloads to a private file. Stream its growing
		// contents concurrently instead of waiting for all bytes first:
		// large files must not trigger HTTP proxy read timeouts.
		var dir string
		dir, err = os.MkdirTemp("", "taskdeck-sftp-download-*")
		if err != nil {break}
		defer os.RemoveAll(dir)
		if chmodErr:=os.Chmod(dir,0o700);chmodErr!=nil {err=chmodErr;break}
		localPath:=filepath.Join(dir,"download")
		var command string
		command,err=sftpclient.GetCommand(req.Path,localPath)
		if err!=nil {break}
		// Use remote size for browser progress and incomplete-response
		// detection when the SFTP server provides it.
		var total int64
		if meta,metaErr:=s.backgroundRemoteEntry(r.Context(),req.ProfileID,req.Path);metaErr==nil&&meta!=nil&&meta.Type=="file" {
			total=meta.Size
			if total>maxFileTransferBytes {err=fmt.Errorf("file transfer exceeds %d bytes",maxFileTransferBytes);break}
		}
		// Never advertise a fixed Content-Length for an actively growing
		// remote log. HTTP chunked streaming allows a larger verified body.
		resultCh:=make(chan error,1)
		go func(){
			_,runErr:=s.runSFTP(longFileTransferContext(r.Context()),profile,command+"quit\n",64<<10)
			resultCh<-runErr
		}()
		var file *os.File
		finished:=false
		var transferErr error
		var sent int64
		buf:=make([]byte,256<<10)
		lastReport:=time.Time{}
		for {
			if r.Context().Err()!=nil {err=r.Context().Err();break}
			if !finished {
				select{case transferErr=<-resultCh:finished=true;default:}
			}
			if file==nil {
				file,err=os.Open(localPath)
				if os.IsNotExist(err){
					if finished {if transferErr!=nil {err=transferErr}else{err=errors.New("SFTP download produced no file")};break}
					err=nil
					time.Sleep(150*time.Millisecond)
					continue
				}
				if err!=nil {break}
				defer file.Close()
				if info,statErr:=file.Stat();statErr!=nil||!info.Mode().IsRegular() {
					err=errors.New("SFTP download did not produce a regular file");break
				}
			}
			n,readErr:=file.Read(buf)
			if n>0 {
				if sent+int64(n)>maxFileTransferBytes {err=fmt.Errorf("file transfer exceeds %d bytes",maxFileTransferBytes);break}
				if !tracked.wrote {setRemoteDownloadHeaders(w,name)}
				written,writeErr:=tracked.Write(buf[:n])
				sent+=int64(written)
				if time.Since(lastReport)>=500*time.Millisecond {
					s.updateBrowserDownloadProgress(token,"running",sent,total,"")
					if flush,ok:=w.(http.Flusher);ok {flush.Flush()}
					lastReport=time.Now()
				}
				if writeErr!=nil {err=writeErr;break}
				if written!=n {err=io.ErrShortWrite;break}
			}
			if readErr!=nil&&readErr!=io.EOF {err=readErr;break}
			if readErr==io.EOF {
				if finished {
					if transferErr!=nil {err=transferErr}
					if err==nil&&total>0&&sent!=total {err=fmt.Errorf("incomplete SFTP download: streamed %d of %d bytes",sent,total)}
					break
				}
				select {
				case transferErr=<-resultCh:finished=true
				case <-r.Context().Done():err=r.Context().Err()
				case <-time.After(150*time.Millisecond):
				}
				if err!=nil {break}
			}
		}
		s.updateBrowserDownloadProgress(token,"running",sent,total,"")

	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		s.updateBrowserDownloadProgress(token,"failed",0,0,err.Error())
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "download", ProfileID: profile.ID, Success: false})
		if !tracked.wrote {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	s.updateBrowserDownloadProgress(token,"success",0,0,"")
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "download", ProfileID: profile.ID, Success: true})
}

// A large browser upload must not hold an HTTP response open throughout a
// potentially multi-hour remote copy. Stage on private disk and hand off to
// the existing daemon queue, which exposes remote-byte progress and retries.
func (s *Server) stageBrowserUpload(profileID, remotePath string, src io.Reader) (*fileTransferServerJob,error) {
	root:=s.browserUploadStagingDir()
	if err:=os.MkdirAll(root,0700);err!=nil{return nil,err}
	dirInfo,err:=os.Lstat(root)
	if err!=nil||!dirInfo.IsDir()||dirInfo.Mode()&os.ModeSymlink!=0 {
		return nil,errors.New("private upload staging folder is unsafe")
	}
	file,err:=os.CreateTemp(root,"upload-*.part")
	if err!=nil{return nil,err}
	source:=file.Name()
	defer func(){
		if err!=nil {_=os.Remove(source)}
	}()
	var copied int64
	copied,err=io.Copy(file,io.LimitReader(src,maxFileTransferBytes+1))
	if syncErr:=file.Sync();err==nil{err=syncErr}
	if closeErr:=file.Close();err==nil{err=closeErr}
	if err!=nil{return nil,err}
	if copied>maxFileTransferBytes {
		err=fmt.Errorf("file transfer exceeds %d bytes",maxFileTransferBytes)
		_ =os.Remove(source)
		return nil,err
	}
	var jobID string
	jobID,err=newFileTransferBackgroundID("browser-upload")
	if err!=nil {_=os.Remove(source);return nil,err}
	now:=time.Now().UTC()
	job:=&fileTransferServerJob{
		ID:jobID,ProfileID:profileID,Kind:"browser_upload",Status:"queued",
		CreatedAt:now,UpdatedAt:now,ScanDone:true,
	}
	queue:=s.fileTransferServerQueue(profileID)
	queue.mu.Lock()
	if queue.Jobs==nil {queue.Jobs=make(map[string]*fileTransferServerJob)}
	queue.Jobs[jobID]=job
	queue.Sequence++
	queue.Items=append(queue.Items,&fileTransferServerItem{
		ID:fmt.Sprintf("srv-%s-%d",jobID,queue.Sequence),JobID:jobID,
		Kind:"Browser upload",Direction:"→",Source:source,Target:remotePath,Size:copied,
		Status:"queued",Operation:"browser_staged_upload",Detail:"Waiting for FTP/SFTP upload slot",
	})
	queue.touchLocked()
	queue.mu.Unlock()
	signalFileTransferQueue(queue)
	return cloneFileTransferJob(job),nil
}

func (s *Server) fileTransferUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFileTransferBytes+(8<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		http.Error(w, "invalid or oversized multipart upload", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	profileID := strings.TrimSpace(r.FormValue("profile_id"))
	remotePath := strings.TrimSpace(r.FormValue("path"))
	if profileID == "" || remotePath == "" {
		http.Error(w, "saved file-transfer profile id and remote path are required", http.StatusBadRequest)
		return
	}
	src, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "upload file is required", http.StatusBadRequest)
		return
	}
	defer src.Close()

	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if r.FormValue("background")=="1" {
		job,stageErr:=s.stageBrowserUpload(profile.ID,remotePath,src)
		if stageErr!=nil {http.Error(w,stageErr.Error(),http.StatusConflict);return}
		writeJSON(w,http.StatusAccepted,map[string]string{"job_id":job.ID,"status":"queued"})
		return
	}
	release, ok := s.tryAcquireFileTransferBrowse(profile.ID)
	if !ok {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-TaskDeck-Transfer-Pool-Full", "1")
		http.Error(w, "FTP/SFTP connection pool is full", http.StatusTooManyRequests)
		return
	}
	defer release()

	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		err = s.withFTPClient(r.Context(), profile, nil, func(client *ftpclient.Client) error {
			reader := io.LimitReader(src, maxFileTransferBytes+1)
			counter := &countingReader{src: reader}
			if storeErr := client.Store(r.Context(), remotePath, counter); storeErr != nil {
				return storeErr
			}
			if counter.n > maxFileTransferBytes {
				return fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
			}
			return nil
		})
	case filetransferprofile.ProtocolSFTP:
		var dir string
		dir, err = os.MkdirTemp("", "taskdeck-sftp-upload-*")
		if err != nil {
			break
		}
		defer os.RemoveAll(dir)
		if chmodErr := os.Chmod(dir, 0o700); chmodErr != nil {
			err = chmodErr
			break
		}
		localPath := filepath.Join(dir, "upload")
		var file *os.File
		file, err = os.OpenFile(localPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			break
		}
		reader := io.LimitReader(src, maxFileTransferBytes+1)
		var written int64
		written, err = io.Copy(file, reader)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			break
		}
		if written > maxFileTransferBytes {
			err = fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
			break
		}
		var command string
		command, err = sftpclient.PutCommand(localPath, remotePath)
		if err != nil {
			break
		}
		_, err = s.runSFTP(longFileTransferContext(r.Context()), profile, command+"quit\n", 64<<10)
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "upload", ProfileID: profile.ID, Success: false})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "upload", ProfileID: profile.ID, Success: true})
	w.WriteHeader(http.StatusNoContent)
}

type countingReader struct {
	src io.Reader
	n   int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	r.n += int64(n)
	return n, err
}
