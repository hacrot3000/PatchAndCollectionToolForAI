package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	fileTransferConflictAsk           = "ask"
	fileTransferConflictOverwrite     = "overwrite"
	fileTransferConflictSkip          = "skip"
	fileTransferConflictSizeDiff      = "size_diff"
	fileTransferConflictSourceNewer   = "source_newer"
	fileTransferConflictChecksumDiff  = "checksum_diff"

	fileTransferConflictScopeItem             = "item"
	fileTransferConflictScopeJob              = "job"
	fileTransferConflictScopeDirectionSession = "direction_session"
	fileTransferConflictScopeDirectionAlways  = "direction_always"
)

type fileTransferConflictMeta struct {
	SourceSize       int64  `json:"source_size"`
	TargetSize       int64  `json:"target_size"`
	SourceModified   string `json:"source_modified,omitempty"`
	TargetModified   string `json:"target_modified,omitempty"`
	SourceSHA256     string `json:"source_sha256,omitempty"`
	TargetSHA256     string `json:"target_sha256,omitempty"`
}

func normalizeFileTransferConflictPolicy(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fileTransferConflictAsk, nil
	}
	switch value {
	case fileTransferConflictAsk,
		fileTransferConflictOverwrite,
		fileTransferConflictSkip,
		fileTransferConflictSizeDiff,
		fileTransferConflictSourceNewer,
		fileTransferConflictChecksumDiff:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported file-transfer conflict policy %q", value)
	}
}

func normalizeFileTransferConflictScope(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fileTransferConflictScopeItem, nil
	}
	switch value {
	case fileTransferConflictScopeItem,
		fileTransferConflictScopeJob,
		fileTransferConflictScopeDirectionSession,
		fileTransferConflictScopeDirectionAlways:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported file-transfer conflict scope %q", value)
	}
}

func parseFileTransferModified(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"01-02-06 03:04PM",
		"01-02-06 15:04",
		"Jan 2 2006",
		"Jan 02 2006",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{"Jan 2 15:04", "Jan 02 15:04"} {
		if partial, err := time.Parse(layout, value); err == nil {
			parsed := time.Date(now.Year(), partial.Month(), partial.Day(), partial.Hour(), partial.Minute(), 0, 0, time.UTC)
			if parsed.After(now.UTC().Add(24 * time.Hour)) {
				parsed = parsed.AddDate(-1, 0, 0)
			}
			return parsed, true
		}
	}
	return time.Time{}, false
}

func (s *Server) backgroundRemoteEntry(ctx context.Context, profileID, remotePath string) (*fileTransferEntry, error) {
	remotePath = normalizeBackgroundRemotePath(remotePath)
	if remotePath == "." || remotePath == "/" {
		return &fileTransferEntry{Name: pathpkg.Base(remotePath), Type: "directory"}, nil
	}
	parent := normalizeBackgroundRemotePath(pathpkg.Dir(remotePath))
	name := pathpkg.Base(remotePath)
	entries, err := s.backgroundListRemote(ctx, profileID, parent)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			copy := entry
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *Server) backgroundHostExistingMeta(rel string) (string, os.FileInfo, bool, error) {
	root, err := s.projectRoot()
	if err != nil {
		return "", nil, false, err
	}
	clean, err := cleanProjectRelativePath(rel, false)
	if err != nil {
		return "", nil, false, err
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	parent := filepath.Dir(target)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !pathWithin(root, resolvedParent) {
		return "", nil, false, errors.New("host destination parent is outside workspace")
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return target, nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, false, errors.New("host destination is a symlink")
	}
	if !info.Mode().IsRegular() {
		return "", nil, false, errors.New("host destination is not a regular file")
	}
	return target, info, true, nil
}

func hashFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Server) backgroundHostSHA256(rel string) (string, error) {
	_, absolute, info, err := s.resolveHostWorkspaceEntry(rel)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("host checksum source is not a regular file")
	}
	if info.Size() > maxFileTransferBytes {
		return "", fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
	}
	return hashFileSHA256(absolute)
}

func (s *Server) backgroundRemoteSHA256(ctx context.Context, profileID, remotePath string) (string, error) {
	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		writer := &transferLimitWriter{dst: hash, remaining: maxFileTransferBytes}
		err = s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			return client.Retrieve(ctx, remotePath, writer)
		})
	case filetransferprofile.ProtocolSFTP:
		dir, mkErr := os.MkdirTemp("", "taskdeck-sftp-hash-*")
		if mkErr != nil {
			return "", mkErr
		}
		defer os.RemoveAll(dir)
		if chmodErr := os.Chmod(dir, 0o700); chmodErr != nil {
			return "", chmodErr
		}
		localPath := filepath.Join(dir, "remote")
		command, cmdErr := sftpclient.GetCommand(remotePath, localPath)
		if cmdErr != nil {
			return "", cmdErr
		}
		if _, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10); err == nil {
			info, statErr := os.Stat(localPath)
			if statErr != nil {
				err = statErr
			} else if !info.Mode().IsRegular() {
				err = errors.New("sftp checksum download did not produce a regular file")
			} else if info.Size() > maxFileTransferBytes {
				err = fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
			} else {
				var file *os.File
				file, err = os.Open(localPath)
				if err == nil {
					_, err = io.Copy(hash, file)
					_ = file.Close()
				}
			}
		}
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}


func (s *Server) enqueueServerTransferConflictAware(ctx context.Context, queue *fileTransferServerQueue, jobID, kind, direction, source, target, operation string, size int64, conflict *fileTransferConflictMeta) error {
	if conflict == nil {
		queue.addItem(jobID, kind, direction, source, target, operation, size, false)
		return nil
	}
	policy := queue.effectiveConflictPolicy(jobID, kind)
	if policy == fileTransferConflictAsk {
		queue.addConflictItem(jobID, kind, direction, source, target, operation, size, *conflict)
		return nil
	}
	item := &fileTransferServerItem{
		JobID: jobID, Kind: kind, Direction: direction, Source: source, Target: target,
		Size: size, Operation: operation, Conflict: conflict,
	}
	overwrite, err := s.evaluateServerConflict(ctx, queue.ProfileID, item, policy)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		conflictItem := queue.addConflictItem(jobID, kind, direction, source, target, operation, size, *conflict)
		if conflictItem == nil {
			return nil
		}
		queue.mu.Lock()
		conflictItem.Error = err.Error()
		queue.touchLocked()
		queue.mu.Unlock()
		return nil
	}
	if !overwrite {
		queue.addSkippedItem(jobID, kind, direction, source, target, operation, size, *conflict, policy)
		return nil
	}
	queue.addResolvedItem(jobID, kind, direction, source, target, operation, size, conflict, policy, true)
	return nil
}

func (s *Server) resolveServerConflictItem(queue *fileTransferServerQueue, item *fileTransferServerItem, policy string) error {
	if item == nil {
		return nil
	}
	overwrite, err := s.evaluateServerConflict(context.Background(), queue.ProfileID, item, policy)
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if item.Status != "conflict" || item.Removed {
		return nil
	}
	if err != nil {
		item.Error = err.Error()
		queue.recomputeJobLocked(item.JobID)
		queue.touchLocked()
		return err
	}
	item.Error = ""
	item.Decision = policy
	if overwrite {
		item.Status = "queued"
		if item.Kind == "Download" {
			item.Overwrite = true
		}
	} else {
		item.Status = "skipped"
		item.Overwrite = false
	}
	queue.recomputeJobLocked(item.JobID)
	queue.touchLocked()
	return nil
}

func (s *Server) resolveServerConflicts(queue *fileTransferServerQueue, req fileTransferJobControlRequest) error {
	policy, err := normalizeFileTransferConflictPolicy(req.ConflictPolicy)
	if err != nil {
		return err
	}
	if policy == fileTransferConflictAsk {
		return errors.New("resolve_conflict requires a concrete conflict policy")
	}
	scope, err := normalizeFileTransferConflictScope(req.ConflictScope)
	if err != nil {
		return err
	}
	selected := make(map[string]bool, len(req.ItemIDs))
	for _, id := range req.ItemIDs {
		if id = strings.TrimSpace(id); id != "" {
			selected[id] = true
		}
	}

	queue.mu.Lock()
	jobID := strings.TrimSpace(req.JobID)
	kind := ""
	if jobID == "" || scope == fileTransferConflictScopeDirectionSession || scope == fileTransferConflictScopeDirectionAlways {
		for _, item := range queue.Items {
			if item.Removed || item.Status != "conflict" {
				continue
			}
			if selected[item.ID] || (jobID != "" && item.JobID == jobID) {
				if jobID == "" {
					jobID = item.JobID
				}
				if kind == "" {
					kind = item.Kind
				}
				break
			}
		}
	}
	if scope == fileTransferConflictScopeJob {
		if jobID == "" {
			queue.mu.Unlock()
			return errors.New("conflict job is required")
		}
		if job := queue.Jobs[jobID]; job != nil {
			job.ConflictPolicy = policy
			job.UpdatedAt = time.Now().UTC()
		} else {
			queue.mu.Unlock()
			return errors.New("conflict job not found")
		}
	}
	if scope == fileTransferConflictScopeDirectionSession || scope == fileTransferConflictScopeDirectionAlways {
		if kind == "" {
			queue.mu.Unlock()
			return errors.New("conflict direction is required")
		}
		switch kind {
		case "Upload":
			queue.UploadConflictPolicy = policy
		case "Download":
			queue.DownloadConflictPolicy = policy
		default:
			queue.mu.Unlock()
			return fmt.Errorf("conflict scope is unsupported for %s", kind)
		}
	}
	items := make([]*fileTransferServerItem, 0)
	for _, item := range queue.Items {
		if item.Removed || item.Status != "conflict" {
			continue
		}
		match := false
		switch scope {
		case fileTransferConflictScopeItem:
			match = selected[item.ID]
		case fileTransferConflictScopeJob:
			match = item.JobID == jobID
		case fileTransferConflictScopeDirectionSession, fileTransferConflictScopeDirectionAlways:
			match = item.Kind == kind
		}
		if match {
			items = append(items, item)
		}
	}
	queue.touchLocked()
	queue.mu.Unlock()

	for _, item := range items {
		_ = s.resolveServerConflictItem(queue, item, policy)
	}
	signalFileTransferQueue(queue)
	return nil
}

func (s *Server) evaluateServerConflict(ctx context.Context, profileID string, item *fileTransferServerItem, policy string) (bool, error) {
	policy, err := normalizeFileTransferConflictPolicy(policy)
	if err != nil {
		return false, err
	}
	if item == nil || item.Conflict == nil {
		return true, nil
	}
	switch policy {
	case fileTransferConflictOverwrite:
		return true, nil
	case fileTransferConflictSkip:
		return false, nil
	case fileTransferConflictSizeDiff:
		return item.Conflict.SourceSize != item.Conflict.TargetSize, nil
	case fileTransferConflictSourceNewer:
		source, okSource := parseFileTransferModified(item.Conflict.SourceModified, time.Now())
		target, okTarget := parseFileTransferModified(item.Conflict.TargetModified, time.Now())
		if !okSource || !okTarget {
			return false, errors.New("modified time is unavailable for source-newer comparison")
		}
		return source.After(target), nil
	case fileTransferConflictChecksumDiff:
		var sourceHash, targetHash string
		switch item.Kind {
		case "Upload":
			sourceHash, err = s.backgroundHostSHA256(item.Source)
			if err == nil {
				targetHash, err = s.backgroundRemoteSHA256(ctx, profileID, item.Target)
			}
		case "Download":
			sourceHash, err = s.backgroundRemoteSHA256(ctx, profileID, item.Source)
			if err == nil {
				targetHash, err = s.backgroundHostSHA256(item.Target)
			}
		default:
			err = fmt.Errorf("checksum conflict comparison is unsupported for %s", item.Kind)
		}
		if err != nil {
			return false, err
		}
		return sourceHash != targetHash, nil
	case fileTransferConflictAsk:
		return false, errors.New("conflict requires user decision")
	default:
		return false, fmt.Errorf("unsupported conflict policy %q", policy)
	}
}

type fileTransferHostHashRequest struct {
	Path string `json:"path"`
}

func (s *Server) fileTransferHostHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferHostHashRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	hash, err := s.backgroundHostSHA256(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sha256": hash})
}

type fileTransferRemoteHashRequest struct {
	ProfileID string `json:"profile_id"`
	Path      string `json:"path"`
}

func (s *Server) fileTransferRemoteHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferRemoteHashRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.Path = strings.TrimSpace(req.Path)
	if req.ProfileID == "" || req.Path == "" {
		http.Error(w, "profile_id and path are required", http.StatusBadRequest)
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
	hash, err := s.backgroundRemoteSHA256(r.Context(), req.ProfileID, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sha256": hash})
}

func (s *Server) fileTransferUploadedHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFileTransferBytes+(8<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		http.Error(w, "invalid or oversized multipart hash input", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	hash := sha256.New()
	reader := io.LimitReader(file, maxFileTransferBytes+1)
	counter := &countingReader{src: reader}
	if _, err := io.Copy(hash, counter); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if counter.n > maxFileTransferBytes {
		http.Error(w, fmt.Sprintf("file transfer exceeds %d bytes", maxFileTransferBytes), http.StatusRequestEntityTooLarge)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sha256": hex.EncodeToString(hash.Sum(nil))})
}
