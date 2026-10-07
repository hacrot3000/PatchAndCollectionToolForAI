package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

type fileTransferTextWriteRequest struct {
	ProfileID      string `json:"profile_id"`
	Path           string `json:"path"`
	Content        string `json:"content"`
	ExpectedSHA256 string `json:"expected_sha256"`
}

type boundedCompareBuffer struct {
	buf       bytes.Buffer
	remaining int64
	exceeded  bool
}

func (w *boundedCompareBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if int64(len(p)) > w.remaining {
		allowed := int(w.remaining)
		if allowed > 0 {
			_, _ = w.buf.Write(p[:allowed])
		}
		w.remaining = 0
		w.exceeded = true
		return allowed, fmt.Errorf("remote text exceeds compare limit")
	}
	n, err := w.buf.Write(p)
	w.remaining -= int64(n)
	return original, err
}

func (s *Server) fileTransferText(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.fileTransferTextRead(w, r)
	case http.MethodPut:
		s.fileTransferTextWrite(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) fileTransferTextRead(w http.ResponseWriter, r *http.Request) {
	profileID := strings.TrimSpace(r.URL.Query().Get("profile_id"))
	remotePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if profileID == "" || remotePath == "" {
		http.Error(w, "profile_id and path are required", http.StatusBadRequest)
		return
	}
	profile, err := s.resolveFileTransferProfile(profileID)
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
	content, err := s.readFileTransferText(r.Context(), profile, remotePath)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errFileTransferTextTooLarge) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, errFileTransferTextUnsupported) {
			status = http.StatusUnsupportedMediaType
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "compare_text_read", ProfileID: profile.ID, Success: false})
		http.Error(w, err.Error(), status)
		return
	}
	sum := sha256.Sum256(content)
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "compare_text_read", ProfileID: profile.ID, Success: true})
	writeJSON(w, http.StatusOK, map[string]any{
		"profile_id": profile.ID, "path": remotePath, "content": string(content), "sha256": hex.EncodeToString(sum[:]),
	})
}

var (
	errFileTransferTextTooLarge   = errors.New("remote text exceeds compare limit")
	errFileTransferTextUnsupported = errors.New("remote file is binary or unsupported text")
)

func (s *Server) readFileTransferText(ctx context.Context, profile filetransferprofile.Profile, remotePath string) ([]byte, error) {
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		buffer := &boundedCompareBuffer{remaining: projectEditableLimit + 1}
		err := s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			return client.Retrieve(ctx, remotePath, buffer)
		})
		if buffer.exceeded || int64(buffer.buf.Len()) > projectEditableLimit {
			return nil, errFileTransferTextTooLarge
		}
		if err != nil {
			return nil, err
		}
		data := append([]byte(nil), buffer.buf.Bytes()...)
		if !projectTextBytesValid(data) {
			return nil, errFileTransferTextUnsupported
		}
		return data, nil
	case filetransferprofile.ProtocolSFTP:
		dir, err := os.MkdirTemp("", "taskdeck-sftp-compare-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, err
		}
		localPath := filepath.Join(dir, "compare")
		command, err := sftpclient.GetCommand(remotePath, localPath)
		if err != nil {
			return nil, err
		}
		if _, err := s.runSFTP(ctx, profile, command+"quit\n", 64<<10); err != nil {
			return nil, err
		}
		info, err := os.Stat(localPath)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("remote compare download is not a regular file")
		}
		if info.Size() > projectEditableLimit {
			return nil, errFileTransferTextTooLarge
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, err
		}
		if !projectTextBytesValid(data) {
			return nil, errFileTransferTextUnsupported
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
}

func validCompareSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size
}

func (s *Server) fileTransferTextWrite(w http.ResponseWriter, r *http.Request) {
	var req fileTransferTextWriteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectEditableLimit+(64<<10)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid remote text write request", http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.Path = strings.TrimSpace(req.Path)
	req.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(req.ExpectedSHA256))
	content := []byte(req.Content)
	if req.ProfileID == "" || req.Path == "" || !validCompareSHA256(req.ExpectedSHA256) {
		http.Error(w, "profile_id, path and expected_sha256 are required", http.StatusBadRequest)
		return
	}
	if int64(len(content)) > projectEditableLimit {
		http.Error(w, errFileTransferTextTooLarge.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	if !projectTextBytesValid(content) {
		http.Error(w, errFileTransferTextUnsupported.Error(), http.StatusUnsupportedMediaType)
		return
	}
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
	currentSHA, err := s.backgroundRemoteSHA256(r.Context(), req.ProfileID, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !strings.EqualFold(currentSHA, req.ExpectedSHA256) {
		http.Error(w, "remote file changed after compare load", http.StatusConflict)
		return
	}
	if err := s.writeFileTransferText(r.Context(), profile, req.Path, content); err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "compare_text_write", ProfileID: profile.ID, Success: false})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	sum := sha256.Sum256(content)
	writtenSHA := hex.EncodeToString(sum[:])
	remoteSHA, err := s.backgroundRemoteSHA256(r.Context(), req.ProfileID, req.Path)
	if err != nil || !strings.EqualFold(remoteSHA, writtenSHA) {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "compare_text_write", ProfileID: profile.ID, Success: false})
		http.Error(w, "remote compare write verification failed", http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "compare_text_write", ProfileID: profile.ID, Success: true})
	writeJSON(w, http.StatusOK, map[string]any{
		"profile_id": profile.ID, "path": req.Path, "sha256": writtenSHA, "content": req.Content,
	})
}

func (s *Server) writeFileTransferText(ctx context.Context, profile filetransferprofile.Profile, remotePath string, content []byte) error {
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		return s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			return client.Store(ctx, remotePath, bytes.NewReader(content))
		})
	case filetransferprofile.ProtocolSFTP:
		dir, err := os.MkdirTemp("", "taskdeck-sftp-compare-upload-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
		localPath := filepath.Join(dir, "compare")
		if err := os.WriteFile(localPath, content, 0o600); err != nil {
			return err
		}
		command, err := sftpclient.PutCommand(localPath, remotePath)
		if err != nil {
			return err
		}
		_, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10)
		return err
	default:
		return fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
}

var _ io.Writer = (*boundedCompareBuffer)(nil)
