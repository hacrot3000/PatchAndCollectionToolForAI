package server

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

const (
	fileTransferJobHostUpload   = "host_upload"
	fileTransferJobHostDownload = "host_download"
	fileTransferJobRemoteDelete = "remote_delete"
)

type fileTransferJobTarget struct {
	Path      string `json:"path"`
	Directory bool   `json:"directory,omitempty"`
}

type fileTransferJobCreateRequest struct {
	ProfileID       string                  `json:"profile_id"`
	Kind            string                  `json:"kind"`
	HostPaths       []string                `json:"host_paths,omitempty"`
	RemoteTargets   []fileTransferJobTarget `json:"remote_targets,omitempty"`
	RemoteDir       string                  `json:"remote_dir,omitempty"`
	HostDir         string                  `json:"host_dir,omitempty"`
	ConflictPolicy  string                  `json:"conflict_policy,omitempty"`
}

type fileTransferJobControlRequest struct {
	ProfileID      string   `json:"profile_id"`
	Action         string   `json:"action"`
	ItemIDs        []string `json:"item_ids,omitempty"`
	ConflictPolicy string   `json:"conflict_policy,omitempty"`
	ConflictScope  string   `json:"conflict_scope,omitempty"`
	JobID          string   `json:"job_id,omitempty"`
}

type fileTransferServerJob struct {
	ID             string    `json:"id"`
	ProfileID      string    `json:"profile_id"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ScanDone       bool      `json:"scan_done"`
	ConflictPolicy string    `json:"conflict_policy,omitempty"`
}

type fileTransferServerItem struct {
	ID             string                    `json:"id"`
	JobID          string                    `json:"job_id"`
	Kind           string                    `json:"kind"`
	Direction      string                    `json:"direction"`
	Source         string                    `json:"source"`
	Target         string                    `json:"target,omitempty"`
	Size           int64                     `json:"size,omitempty"`
	Status         string                    `json:"status"`
	Error          string                    `json:"error,omitempty"`
	Decision       string                    `json:"decision,omitempty"`
	Conflict       *fileTransferConflictMeta `json:"conflict,omitempty"`
	Operation      string                    `json:"-"`
	Directory      bool                      `json:"-"`
	Overwrite      bool                      `json:"-"`
	Priority       bool                      `json:"-"`
	Removed        bool                      `json:"-"`
	RemoveAfterRun bool                      `json:"-"`
}

type fileTransferServerQueue struct {
	mu                     sync.Mutex
	ProfileID              string
	Paused                 bool
	UploadConflictPolicy   string
	DownloadConflictPolicy string
	Items                  []*fileTransferServerItem
	Jobs                   map[string]*fileTransferServerJob
	Sequence               uint64
	ActiveScans            int
	Revision               uint64
	wake                   chan struct{}
	workerOnce             sync.Once
}

type fileTransferJobsSnapshot struct {
	ProfileID   string                   `json:"profile_id"`
	Paused      bool                     `json:"paused"`
	ActiveScans int                      `json:"active_scans"`
	Revision    uint64                   `json:"revision"`
	Jobs        []*fileTransferServerJob `json:"jobs"`
	Items       []*fileTransferServerItem `json:"items"`
}

func newFileTransferBackgroundID(prefix string) (string, error) {
	var raw [12]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(raw[:]), nil
}

func (s *Server) fileTransferServerQueue(profileID string) *fileTransferServerQueue {
	profileID = strings.TrimSpace(profileID)
	s.fileTransferJobsMu.Lock()
	defer s.fileTransferJobsMu.Unlock()
	if s.fileTransferJobQueues == nil {
		s.fileTransferJobQueues = make(map[string]*fileTransferServerQueue)
	}
	if queue := s.fileTransferJobQueues[profileID]; queue != nil {
		return queue
	}
	queue := &fileTransferServerQueue{
		ProfileID: profileID,
		Jobs:      make(map[string]*fileTransferServerJob),
		wake:      make(chan struct{}, 1),
	}
	s.fileTransferJobQueues[profileID] = queue
	queue.workerOnce.Do(func() { go s.runFileTransferServerQueue(queue) })
	return queue
}

func signalFileTransferQueue(queue *fileTransferServerQueue) {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

func cloneFileTransferJob(job *fileTransferServerJob) *fileTransferServerJob {
	if job == nil {
		return nil
	}
	copy := *job
	return &copy
}

func cloneFileTransferItem(item *fileTransferServerItem) *fileTransferServerItem {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func (q *fileTransferServerQueue) snapshot() fileTransferJobsSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := fileTransferJobsSnapshot{
		ProfileID:   q.ProfileID,
		Paused:      q.Paused,
		ActiveScans: q.ActiveScans,
		Revision:    q.Revision,
		Jobs:        make([]*fileTransferServerJob, 0, len(q.Jobs)),
		Items:       make([]*fileTransferServerItem, 0, len(q.Items)),
	}
	for _, job := range q.Jobs {
		out.Jobs = append(out.Jobs, cloneFileTransferJob(job))
	}
	for _, item := range q.Items {
		if !item.Removed {
			out.Items = append(out.Items, cloneFileTransferItem(item))
		}
	}
	return out
}

func (q *fileTransferServerQueue) touchLocked() {
	q.Revision++
}

func (q *fileTransferServerQueue) addItem(jobID, kind, direction, source, target, operation string, size int64, directory bool) *fileTransferServerItem {
	q.mu.Lock()
	q.Sequence++
	item := &fileTransferServerItem{
		ID:        fmt.Sprintf("srv-%s-%d", jobID, q.Sequence),
		JobID:     jobID,
		Kind:      kind,
		Direction: direction,
		Source:    source,
		Target:    target,
		Size:      size,
		Status:    "queued",
		Operation: operation,
		Directory: directory,
	}
	q.Items = append(q.Items, item)
	q.touchLocked()
	q.mu.Unlock()
	signalFileTransferQueue(q)
	return item
}

func (q *fileTransferServerQueue) addConflictItem(jobID, kind, direction, source, target, operation string, size int64, conflict fileTransferConflictMeta) *fileTransferServerItem {
	q.mu.Lock()
	q.Sequence++
	item := &fileTransferServerItem{
		ID:        fmt.Sprintf("srv-%s-%d", jobID, q.Sequence),
		JobID:     jobID,
		Kind:      kind,
		Direction: direction,
		Source:    source,
		Target:    target,
		Size:      size,
		Status:    "conflict",
		Operation: operation,
		Conflict:  &conflict,
	}
	q.Items = append(q.Items, item)
	q.touchLocked()
	q.recomputeJobLocked(jobID)
	q.mu.Unlock()
	return item
}

func (q *fileTransferServerQueue) addSkippedItem(jobID, kind, direction, source, target, operation string, size int64, conflict fileTransferConflictMeta, decision string) {
	q.mu.Lock()
	q.Sequence++
	item := &fileTransferServerItem{
		ID:        fmt.Sprintf("srv-%s-%d", jobID, q.Sequence),
		JobID:     jobID,
		Kind:      kind,
		Direction: direction,
		Source:    source,
		Target:    target,
		Size:      size,
		Status:    "skipped",
		Operation: operation,
		Conflict:  &conflict,
		Decision:  decision,
	}
	q.Items = append(q.Items, item)
	q.touchLocked()
	q.recomputeJobLocked(jobID)
	q.mu.Unlock()
}

func (q *fileTransferServerQueue) effectiveConflictPolicy(jobID, kind string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job := q.Jobs[jobID]; job != nil && job.ConflictPolicy != "" && job.ConflictPolicy != fileTransferConflictAsk {
		return job.ConflictPolicy
	}
	switch kind {
	case "Upload":
		if q.UploadConflictPolicy != "" {
			return q.UploadConflictPolicy
		}
	case "Download":
		if q.DownloadConflictPolicy != "" {
			return q.DownloadConflictPolicy
		}
	}
	return fileTransferConflictAsk
}

func (q *fileTransferServerQueue) setScanState(jobID string, done bool, scanErr error) {
	q.mu.Lock()
	if job := q.Jobs[jobID]; job != nil {
		job.ScanDone = done
		job.UpdatedAt = time.Now().UTC()
		if scanErr != nil {
			job.Error = scanErr.Error()
			job.Status = "failed"
		} else if done {
			job.Status = "queued"
		}
	}
	if done && q.ActiveScans > 0 {
		q.ActiveScans--
	}
	q.recomputeJobLocked(jobID)
	q.touchLocked()
	q.mu.Unlock()
	signalFileTransferQueue(q)
}

func (q *fileTransferServerQueue) recomputeJobLocked(jobID string) {
	job := q.Jobs[jobID]
	if job == nil {
		return
	}
	hasQueued := false
	hasRunning := false
	hasFailed := false
	hasConflict := false
	hasItem := false
	for _, item := range q.Items {
		if item.JobID != jobID || item.Removed {
			continue
		}
		hasItem = true
		switch item.Status {
		case "queued":
			hasQueued = true
		case "running":
			hasRunning = true
		case "failed":
			hasFailed = true
		case "conflict":
			hasConflict = true
		}
	}
	switch {
	case hasRunning:
		job.Status = "running"
	case !job.ScanDone:
		job.Status = "scanning"
	case hasQueued:
		job.Status = "queued"
	case hasConflict:
		job.Status = "conflict"
	case hasFailed:
		job.Status = "failed"
	case hasItem:
		job.Status = "done"
	case job.Error != "":
		job.Status = "failed"
	default:
		job.Status = "done"
	}
	job.UpdatedAt = time.Now().UTC()
}

func (q *fileTransferServerQueue) nextRunnableLocked() *fileTransferServerItem {
	for _, item := range q.Items {
		if item.Removed || item.Status != "queued" || !item.Priority {
			continue
		}
		item.Priority = false
		return item
	}
	if q.Paused {
		return nil
	}
	for _, item := range q.Items {
		if item.Removed || item.Status != "queued" {
			continue
		}
		return item
	}
	return nil
}

func (s *Server) runFileTransferServerQueue(queue *fileTransferServerQueue) {
	for range queue.wake {
		for {
			queue.mu.Lock()
			item := queue.nextRunnableLocked()
			if item == nil {
				queue.mu.Unlock()
				break
			}
			item.Status = "running"
			item.Error = ""
			queue.recomputeJobLocked(item.JobID)
			queue.touchLocked()
			queue.mu.Unlock()

			err := s.executeFileTransferServerItem(context.Background(), queue.ProfileID, item)

			queue.mu.Lock()
			if err != nil {
				item.Status = "failed"
				item.Error = err.Error()
			} else {
				item.Status = "success"
				item.Error = ""
			}
			if item.RemoveAfterRun {
				item.Removed = true
			}
			queue.recomputeJobLocked(item.JobID)
			queue.touchLocked()
			queue.mu.Unlock()
		}
	}
}

func (s *Server) executeFileTransferServerItem(ctx context.Context, profileID string, item *fileTransferServerItem) error {
	switch item.Operation {
	case "host_upload":
		return s.backgroundHostToRemote(ctx, profileID, item.Source, item.Target)
	case "host_download":
		return s.backgroundRemoteToHost(ctx, profileID, item.Source, item.Target, item.Overwrite)
	case "remote_delete":
		err := s.backgroundRemoteMutation(ctx, profileID, "delete", item.Source, "", item.Directory)
		if err == nil {
			return nil
		}
		exists, checkErr := s.backgroundRemoteEntryExists(ctx, profileID, item.Source)
		if checkErr == nil && !exists {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unsupported background file-transfer operation %q", item.Operation)
	}
}

func (s *Server) backgroundListRemote(ctx context.Context, profileID, remotePath string) ([]fileTransferEntry, error) {
	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil {
		return nil, err
	}
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		var entries []fileTransferEntry
		err = s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			items, listErr := client.List(ctx, remotePath)
			if listErr != nil {
				return listErr
			}
			entries = make([]fileTransferEntry, 0, len(items))
			for _, item := range items {
				entries = append(entries, fileTransferEntry(item))
			}
			return nil
		})
		return entries, err
	case filetransferprofile.ProtocolSFTP:
		command, err := sftpclient.ListCommand(remotePath)
		if err != nil {
			return nil, err
		}
		result, err := s.runSFTP(ctx, profile, command+"quit\n", sftpclient.DefaultMaxOutputBytes)
		if err != nil {
			return nil, err
		}
		items, err := sftpclient.ParseLongList(result.Stdout)
		if err != nil {
			return nil, err
		}
		entries := make([]fileTransferEntry, 0, len(items))
		for _, item := range items {
			entries = append(entries, fileTransferEntry(item))
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
}

func (s *Server) backgroundRemoteEntryExists(ctx context.Context, profileID, remotePath string) (bool, error) {
	remotePath = normalizeBackgroundRemotePath(remotePath)
	if remotePath == "." || remotePath == "/" {
		return true, nil
	}
	parent := normalizeBackgroundRemotePath(pathpkg.Dir(remotePath))
	name := pathpkg.Base(remotePath)
	entries, err := s.backgroundListRemote(ctx, profileID, parent)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) backgroundRemoteMutation(ctx context.Context, profileID, action, remotePath, newPath string, directory bool) error {
	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil {
		return err
	}
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		return s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			switch action {
			case "mkdir":
				return client.Mkdir(ctx, remotePath)
			case "delete":
				return client.Delete(ctx, remotePath, directory)
			default:
				return fmt.Errorf("unsupported background mutation %q", action)
			}
		})
	case filetransferprofile.ProtocolSFTP:
		var command string
		switch action {
		case "mkdir":
			command, err = sftpclient.MkdirCommand(remotePath)
		case "delete":
			command, err = sftpclient.RemoveCommand(remotePath, directory)
		default:
			err = fmt.Errorf("unsupported background mutation %q", action)
		}
		if err != nil {
			return err
		}
		_, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10)
		return err
	default:
		return fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
}

func (s *Server) backgroundHostToRemote(ctx context.Context, profileID, hostRel, remotePath string) error {
	_, hostPath, info, err := s.resolveHostWorkspaceEntry(hostRel)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("host upload source is not a regular file")
	}
	if info.Size() > maxFileTransferBytes {
		return fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
	}
	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil {
		return err
	}
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		file, err := os.Open(hostPath)
		if err != nil {
			return err
		}
		defer file.Close()
		return s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			return client.Store(ctx, remotePath, file)
		})
	case filetransferprofile.ProtocolSFTP:
		command, err := sftpclient.PutCommand(hostPath, remotePath)
		if err != nil {
			return err
		}
		_, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10)
		return err
	default:
		return fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
}

func (s *Server) backgroundHostTarget(rel string) (string, error) {
	root, err := s.projectRoot()
	if err != nil {
		return "", err
	}
	clean, err := cleanProjectRelativePath(rel, false)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	parent := filepath.Dir(target)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !pathWithin(root, resolvedParent) {
		return "", errors.New("host destination parent is outside workspace")
	}
	info, err := os.Stat(resolvedParent)
	if err != nil || !info.IsDir() {
		return "", errors.New("host destination parent not found")
	}
	if existing, err := os.Lstat(target); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular() {
			return "", errors.New("host destination is not a regular file")
		}
		return "", errors.New("host destination file already exists")
	} else if !os.IsNotExist(err) {
		return "", errors.New("host destination unavailable")
	}
	return target, nil
}

func (s *Server) backgroundRemoteToHost(ctx context.Context, profileID, remotePath, hostRel string) error {
	target, err := s.backgroundHostTarget(hostRel)
	if err != nil {
		return err
	}
	hostDir := filepath.Dir(target)
	tmp, err := os.CreateTemp(hostDir, ".taskdeck-file-transfer-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil {
		return err
	}
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		writer := &transferLimitWriter{dst: tmp, remaining: maxFileTransferBytes}
		err = s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			return client.Retrieve(ctx, remotePath, writer)
		})
		if syncErr := tmp.Sync(); err == nil {
			err = syncErr
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
	case filetransferprofile.ProtocolSFTP:
		if closeErr := tmp.Close(); closeErr != nil {
			return closeErr
		}
		command, cmdErr := sftpclient.GetCommand(remotePath, tmpPath)
		if cmdErr != nil {
			return cmdErr
		}
		_, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10)
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		return err
	}
	info, err := os.Stat(tmpPath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("downloaded host temp file unavailable")
	}
	if info.Size() > maxFileTransferBytes {
		return fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	if _, err := s.backgroundHostTarget(hostRel); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}

func normalizeBackgroundRemotePath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || value == "." {
		return "."
	}
	absolute := strings.HasPrefix(value, "/")
	clean := pathpkg.Clean(value)
	if clean == "." {
		return "."
	}
	if absolute && !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	return clean
}

func joinBackgroundRemotePath(parent, name string) string {
	parent = normalizeBackgroundRemotePath(parent)
	name = strings.TrimLeft(strings.ReplaceAll(name, "\\", "/"), "/")
	if parent == "." {
		return name
	}
	if parent == "/" {
		return "/" + name
	}
	return strings.TrimSuffix(parent, "/") + "/" + name
}

func (s *Server) ensureBackgroundRemoteDirectory(ctx context.Context, profileID, remoteDir string, known map[string]bool) error {
	remoteDir = normalizeBackgroundRemotePath(remoteDir)
	if remoteDir == "." || remoteDir == "/" || known[remoteDir] {
		return nil
	}
	parent := normalizeBackgroundRemotePath(pathpkg.Dir(remoteDir))
	if parent == "" {
		parent = "."
	}
	if err := s.ensureBackgroundRemoteDirectory(ctx, profileID, parent, known); err != nil {
		return err
	}
	name := pathpkg.Base(remoteDir)
	entries, err := s.backgroundListRemote(ctx, profileID, parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name == name {
			if entry.Type != "directory" {
				return fmt.Errorf("remote path exists and is not a folder: %s", remoteDir)
			}
			known[remoteDir] = true
			return nil
		}
	}
	if err := s.backgroundRemoteMutation(ctx, profileID, "mkdir", remoteDir, "", true); err != nil {
		return err
	}
	known[remoteDir] = true
	return nil
}

func (s *Server) ensureBackgroundHostDirectory(rel string) (string, error) {
	root, err := s.projectRoot()
	if err != nil {
		return "", err
	}
	clean, err := cleanProjectRelativePath(rel, false)
	if err != nil {
		return "", err
	}
	current := root
	if clean == "." || clean == "" {
		return root, nil
	}
	for _, part := range strings.Split(filepath.FromSlash(clean), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if os.IsNotExist(err) {
			if err := os.Mkdir(next, 0o755); err != nil {
				return "", err
			}
			current = next
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("host destination directory is unsafe: %s", rel)
		}
		current = next
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil || !pathWithin(root, resolved) {
		return "", errors.New("host destination directory is outside workspace")
	}
	return current, nil
}

func (s *Server) scanHostUploadJob(queue *fileTransferServerQueue, jobID string, req fileTransferJobCreateRequest) error {
	knownRemote := map[string]bool{normalizeBackgroundRemotePath(req.RemoteDir): true, ".": true, "/": true}
	var walk func(string, string) error
	walk = func(hostRel, remotePath string) error {
		rel, absolute, info, err := s.resolveHostWorkspaceEntry(hostRel)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := s.ensureBackgroundRemoteDirectory(context.Background(), req.ProfileID, remotePath, knownRemote); err != nil {
				return err
			}
			entries, err := os.ReadDir(absolute)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("host upload does not follow symlink %s", filepath.ToSlash(filepath.Join(rel, entry.Name())))
				}
				if err := walk(filepath.ToSlash(filepath.Join(rel, entry.Name())), joinBackgroundRemotePath(remotePath, entry.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported host upload item %s", rel)
		}
		queue.addItem(jobID, "Upload", "→", rel, remotePath, "host_upload", info.Size(), false)
		return nil
	}
	for _, requested := range req.HostPaths {
		rel, _, info, err := s.resolveHostWorkspaceEntry(requested)
		if err != nil {
			return err
		}
		base := filepath.Base(filepath.FromSlash(rel))
		remoteTarget := joinBackgroundRemotePath(req.RemoteDir, base)
		if info.IsDir() {
			if err := walk(rel, remoteTarget); err != nil {
				return err
			}
		} else {
			queue.addItem(jobID, "Upload", "→", rel, remoteTarget, "host_upload", info.Size(), false)
		}
	}
	return nil
}

func (s *Server) scanHostDownloadJob(queue *fileTransferServerQueue, jobID string, req fileTransferJobCreateRequest) error {
	if _, err := s.ensureBackgroundHostDirectory(req.HostDir); err != nil {
		return err
	}
	var walk func(fileTransferJobTarget, string) error
	walk = func(target fileTransferJobTarget, hostParent string) error {
		name := pathpkg.Base(normalizeBackgroundRemotePath(target.Path))
		hostRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(hostParent), name))
		if !target.Directory {
			queue.addItem(jobID, "Download", "←", target.Path, hostRel, "host_download", 0, false)
			return nil
		}
		if _, err := s.ensureBackgroundHostDirectory(hostRel); err != nil {
			return err
		}
		entries, err := s.backgroundListRemote(context.Background(), req.ProfileID, target.Path)
		if err != nil {
			exists, checkErr := s.backgroundRemoteEntryExists(context.Background(), req.ProfileID, target.Path)
			if checkErr == nil && !exists {
				return nil
			}
			return err
		}
		for _, entry := range entries {
			child := fileTransferJobTarget{
				Path:      joinBackgroundRemotePath(target.Path, entry.Name),
				Directory: entry.Type == "directory",
			}
			if child.Directory {
				if err := walk(child, hostRel); err != nil {
					return err
				}
			} else {
				childHost := filepath.ToSlash(filepath.Join(filepath.FromSlash(hostRel), entry.Name))
				queue.addItem(jobID, "Download", "←", child.Path, childHost, "host_download", entry.Size, false)
			}
		}
		return nil
	}
	for _, target := range req.RemoteTargets {
		if err := walk(target, req.HostDir); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) scanRemoteDeleteJob(queue *fileTransferServerQueue, jobID string, req fileTransferJobCreateRequest) error {
	var walk func(fileTransferJobTarget) error
	walk = func(target fileTransferJobTarget) error {
		target.Path = normalizeBackgroundRemotePath(target.Path)
		if !target.Directory {
			queue.addItem(jobID, "Delete", "×", target.Path, "", "remote_delete", 0, false)
			return nil
		}
		entries, err := s.backgroundListRemote(context.Background(), req.ProfileID, target.Path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := fileTransferJobTarget{
				Path:      joinBackgroundRemotePath(target.Path, entry.Name),
				Directory: entry.Type == "directory",
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		queue.addItem(jobID, "Delete", "×", target.Path, "", "remote_delete", 0, true)
		return nil
	}
	for _, target := range req.RemoteTargets {
		if err := walk(target); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) createFileTransferServerJob(req fileTransferJobCreateRequest) (*fileTransferServerJob, error) {
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.Kind = strings.TrimSpace(req.Kind)
	req.RemoteDir = normalizeBackgroundRemotePath(req.RemoteDir)
	req.HostDir = strings.TrimSpace(req.HostDir)
	policy, err := normalizeFileTransferConflictPolicy(req.ConflictPolicy)
	if err != nil {
		return nil, err
	}
	req.ConflictPolicy = policy
	if req.HostDir == "" {
		req.HostDir = "."
	}
	if req.ProfileID == "" {
		return nil, errors.New("profile_id is required")
	}
	if _, err := s.resolveFileTransferProfile(req.ProfileID); err != nil {
		return nil, err
	}
	switch req.Kind {
	case fileTransferJobHostUpload:
		if len(req.HostPaths) == 0 {
			return nil, errors.New("host_paths are required")
		}
	case fileTransferJobHostDownload, fileTransferJobRemoteDelete:
		if len(req.RemoteTargets) == 0 {
			return nil, errors.New("remote_targets are required")
		}
	default:
		return nil, errors.New("unsupported file-transfer job kind")
	}
	jobID, err := newFileTransferBackgroundID("ftjob")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	job := &fileTransferServerJob{
		ID: jobID, ProfileID: req.ProfileID, Kind: req.Kind,
		Status: "scanning", CreatedAt: now, UpdatedAt: now, ConflictPolicy: req.ConflictPolicy,
	}
	queue := s.fileTransferServerQueue(req.ProfileID)
	queue.mu.Lock()
	queue.Jobs[jobID] = job
	queue.ActiveScans++
	queue.touchLocked()
	queue.mu.Unlock()

	go func() {
		var scanErr error
		switch req.Kind {
		case fileTransferJobHostUpload:
			scanErr = s.scanHostUploadJob(queue, jobID, req)
		case fileTransferJobHostDownload:
			scanErr = s.scanHostDownloadJob(queue, jobID, req)
		case fileTransferJobRemoteDelete:
			scanErr = s.scanRemoteDeleteJob(queue, jobID, req)
		}
		queue.setScanState(jobID, true, scanErr)
	}()
	return cloneFileTransferJob(job), nil
}

func (s *Server) fileTransferJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profileID := strings.TrimSpace(r.URL.Query().Get("profile_id"))
		if profileID == "" {
			http.Error(w, "profile_id is required", http.StatusBadRequest)
			return
		}
		if _, err := s.resolveFileTransferProfile(profileID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, s.fileTransferServerQueue(profileID).snapshot())
	case http.MethodPost:
		var req fileTransferJobCreateRequest
		if err := decodeFileTransferJSON(w, r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		job, err := s.createFileTransferServerJob(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) fileTransferJobsControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferJobControlRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.Action = strings.TrimSpace(req.Action)
	if req.ProfileID == "" || req.Action == "" {
		http.Error(w, "profile_id and action are required", http.StatusBadRequest)
		return
	}
	queue := s.fileTransferServerQueue(req.ProfileID)
	selected := make(map[string]bool, len(req.ItemIDs))
	for _, id := range req.ItemIDs {
		selected[strings.TrimSpace(id)] = true
	}
	queue.mu.Lock()
	switch req.Action {
	case "pause":
		queue.Paused = true
	case "resume":
		queue.Paused = false
	case "resume_selected":
		for _, item := range queue.Items {
			if !selected[item.ID] || item.Removed {
				continue
			}
			if item.Status == "failed" {
				item.Status = "queued"
				item.Error = ""
			}
			if item.Status == "queued" {
				item.Priority = true
			}
		}
	case "remove_selected":
		for _, item := range queue.Items {
			if !selected[item.ID] || item.Removed {
				continue
			}
			if item.Status == "running" {
				item.RemoveAfterRun = true
			} else {
				item.Removed = true
			}
			queue.recomputeJobLocked(item.JobID)
		}
	case "retry_failed":
		for _, item := range queue.Items {
			if !item.Removed && item.Status == "failed" {
				item.Status = "queued"
				item.Error = ""
			}
		}
	case "clear_done":
		for _, item := range queue.Items {
			if item.Status == "success" {
				item.Removed = true
			}
		}
	default:
		queue.mu.Unlock()
		http.Error(w, "unsupported file-transfer queue action", http.StatusBadRequest)
		return
	}
	queue.touchLocked()
	queue.mu.Unlock()
	signalFileTransferQueue(queue)
	writeJSON(w, http.StatusOK, queue.snapshot())
}
