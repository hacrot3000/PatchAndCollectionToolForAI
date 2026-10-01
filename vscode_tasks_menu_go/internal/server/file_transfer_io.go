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
	maxFileTransferBytes          int64 = 1 << 30 // 1 GiB initial safety boundary.
	fileTransferDownloadTicketTTL       = 90 * time.Second
	maxFileTransferDownloadTickets      = 128
)

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
		if !ticket.ExpiresAt.After(now) {
			delete(s.fileTransferDownloads, key)
		}
	}
	if len(s.fileTransferDownloads) >= maxFileTransferDownloadTickets {
		return "", errors.New("too many pending file-transfer downloads")
	}
	s.fileTransferDownloads[token] = fileTransferDownloadTicket{
		ProfileID: profileID,
		Path:      remotePath,
		ExpiresAt: now.Add(fileTransferDownloadTicketTTL),
	}
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
	ticket, ok := s.consumeFileTransferDownloadTicket(r.URL.Query().Get("ticket"))
	if !ok {
		http.Error(w, "download ticket not found or expired", http.StatusNotFound)
		return
	}
	req := fileTransferOperationRequest{ProfileID: ticket.ProfileID, Path: ticket.Path}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := remoteDownloadName(req.Path)
	tracked := &downloadResponseTracker{ResponseWriter: w}

	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		err = s.withFTPClient(r.Context(), profile, nil, func(client *ftpclient.Client) error {
			setRemoteDownloadHeaders(w, name)
			writer := &transferLimitWriter{dst: tracked, remaining: maxFileTransferBytes}
			return client.Retrieve(r.Context(), req.Path, writer)
		})
	case filetransferprofile.ProtocolSFTP:
		var dir string
		dir, err = os.MkdirTemp("", "taskdeck-sftp-download-*")
		if err != nil {
			break
		}
		defer os.RemoveAll(dir)
		if chmodErr := os.Chmod(dir, 0o700); chmodErr != nil {
			err = chmodErr
			break
		}
		localPath := filepath.Join(dir, "download")
		var command string
		command, err = sftpclient.GetCommand(req.Path, localPath)
		if err != nil {
			break
		}
		_, err = s.runSFTP(r.Context(), profile, command+"quit\n", 64<<10)
		if err != nil {
			break
		}
		if chmodErr := os.Chmod(localPath, 0o600); chmodErr != nil {
			err = fmt.Errorf("protect sftp download temp file: %w", chmodErr)
			break
		}
		var file *os.File
		file, err = os.Open(localPath)
		if err != nil {
			break
		}
		defer file.Close()
		var info os.FileInfo
		info, err = file.Stat()
		if err != nil {
			break
		}
		if !info.Mode().IsRegular() {
			err = errors.New("sftp download did not produce a regular file")
			break
		}
		if info.Size() > maxFileTransferBytes {
			err = fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
			break
		}
		setRemoteDownloadHeaders(w, name)
		_, err = io.Copy(tracked, file)
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "download", ProfileID: profile.ID, Success: false})
		if !tracked.wrote {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "download", ProfileID: profile.ID, Success: true})
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
		_, err = s.runSFTP(r.Context(), profile, command+"quit\n", 64<<10)
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
