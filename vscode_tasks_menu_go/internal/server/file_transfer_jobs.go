package server

import (
	"context"
	"crypto/sha256"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

const (
	fileTransferJobHostUpload        = "host_upload"
	fileTransferJobHostDownload      = "host_download"
	fileTransferJobRemoteDelete      = "remote_delete"
	fileTransferJobHostArchiveUpload = "host_archive_upload"

	// Large multi-selection actions can legitimately carry tens of thousands
	// of remote paths/item IDs. Keep the general file-transfer JSON limit small,
	// but give the background job/queue endpoints a bounded bulk-action budget.
	maxFileTransferJobJSONBytes = 16 << 20
)

type fileTransferJobTarget struct {
	Path      string `json:"path"`
	Directory bool   `json:"directory,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Modified  string `json:"modified,omitempty"`
}

type fileTransferJobCreateRequest struct {
	ProfileID       string                  `json:"profile_id"`
	Kind            string                  `json:"kind"`
	HostPaths       []string                `json:"host_paths,omitempty"`
	RemoteTargets   []fileTransferJobTarget `json:"remote_targets,omitempty"`
	RemoteDir       string                  `json:"remote_dir,omitempty"`
	HostDir         string                  `json:"host_dir,omitempty"`
	ConflictPolicy  string                  `json:"conflict_policy,omitempty"`
	MergePolicy     string                  `json:"merge_policy,omitempty"`
	AutoExtract     bool                    `json:"auto_extract,omitempty"`
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
	Recovered      bool      `json:"recovered,omitempty"`
	Request        *fileTransferJobCreateRequest `json:"-"`
	NeedsRescan       bool              `json:"-"`
	Phase             string            `json:"phase,omitempty"`
	Files             int64             `json:"files,omitempty"`
	Bytes             int64             `json:"bytes,omitempty"`
	ArchiveBytes      int64             `json:"archive_bytes,omitempty"`
	RemoteArchive      string            `json:"remote_archive,omitempty"`
	RemoteDestination string            `json:"remote_destination,omitempty"`
	NeedsManualExtract bool             `json:"needs_manual_extract,omitempty"`
	ManualCommands    map[string]string `json:"manual_commands,omitempty"`
}

type fileTransferServerItem struct {
	ID             string                    `json:"id"`
	JobID          string                    `json:"job_id"`
	Kind           string                    `json:"kind"`
	Direction      string                    `json:"direction"`
	Source         string                    `json:"source"`
	Target         string                    `json:"target,omitempty"`
	Size           int64                     `json:"size,omitempty"`
	Attempts       int                       `json:"attempts,omitempty"`
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
	Detail         string                    `json:"detail,omitempty"`
	Files          int64                     `json:"files,omitempty"`
	BytesDone      int64                     `json:"bytes_done,omitempty"`
	BytesTotal     int64                     `json:"bytes_total,omitempty"`
}

type fileTransferPendingScan struct {
	JobID string
	Req   fileTransferJobCreateRequest
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
	MaxConnections         int
	ActiveScans            int
	QueuedScans            int
	ActiveTransfers        int
	ActiveBrowses          int
	PendingScans           []fileTransferPendingScan
	ScanCancels            map[string]context.CancelFunc
	TransferCancels        map[string]context.CancelFunc
	StoppedScanJobs        map[string]bool
	Revision               uint64
	wake                   chan struct{}
	workerOnce             sync.Once
	Recovered              bool
	persist                func()
}

type fileTransferJobsSnapshot struct {
	ProfileID        string                    `json:"profile_id"`
	Paused           bool                      `json:"paused"`
	MaxConnections   int                       `json:"max_connections"`
	ActiveConnections int                      `json:"active_connections"`
	ActiveScans      int                       `json:"active_scans"`
	QueuedScans      int                       `json:"queued_scans"`
	ActiveTransfers  int                       `json:"active_transfers"`
	ActiveBrowses    int                       `json:"active_browses"`
	Revision         uint64                    `json:"revision"`
	Jobs             []*fileTransferServerJob  `json:"jobs"`
	Items            []*fileTransferServerItem `json:"items"`
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
	s.ensureFileTransferQueuesLoadedLocked()
	if s.fileTransferJobQueues == nil {
		s.fileTransferJobQueues = make(map[string]*fileTransferServerQueue)
	}
	if queue := s.fileTransferJobQueues[profileID]; queue != nil {
		return queue
	}
	limit := filetransferprofile.DefaultMaxConnections
	if profile, err := s.resolveFileTransferProfile(profileID); err == nil && profile.MaxConnections > 0 {
		limit = profile.MaxConnections
	}
	queue := &fileTransferServerQueue{
		ProfileID:      profileID,
		MaxConnections: limit,
		Jobs:           make(map[string]*fileTransferServerJob),
		ScanCancels:     make(map[string]context.CancelFunc),
		TransferCancels: make(map[string]context.CancelFunc),
		StoppedScanJobs: make(map[string]bool),
		wake:           make(chan struct{}, 1),
		persist:        s.scheduleFileTransferQueuePersist,
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
	if job.ManualCommands != nil {
		copy.ManualCommands = make(map[string]string, len(job.ManualCommands))
		for key, value := range job.ManualCommands {
			copy.ManualCommands[key] = value
		}
	}
	return &copy
}

func cloneFileTransferItem(item *fileTransferServerItem) *fileTransferServerItem {
	if item == nil {
		return nil
	}
	copy := *item
	if item.Conflict != nil {
		conflict := *item.Conflict
		copy.Conflict = &conflict
	}
	return &copy
}

func (q *fileTransferServerQueue) snapshot() fileTransferJobsSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	limit := q.connectionLimitLocked()
	out := fileTransferJobsSnapshot{
		ProfileID:         q.ProfileID,
		Paused:            q.Paused,
		MaxConnections:    limit,
		ActiveConnections: q.activeConnectionsLocked(),
		ActiveScans:       q.ActiveScans,
		QueuedScans:       q.QueuedScans,
		ActiveTransfers:   q.ActiveTransfers,
		ActiveBrowses:     q.ActiveBrowses,
		Revision:          q.Revision,
		Jobs:              make([]*fileTransferServerJob, 0, len(q.Jobs)),
		Items:             make([]*fileTransferServerItem, 0, len(q.Items)),
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
	if q.persist != nil {
		q.persist()
	}
}

func (q *fileTransferServerQueue) connectionLimitLocked() int {
	if q.MaxConnections < 1 {
		return filetransferprofile.DefaultMaxConnections
	}
	return q.MaxConnections
}

func (q *fileTransferServerQueue) activeConnectionsLocked() int {
	return q.ActiveScans + q.ActiveTransfers + q.ActiveBrowses
}

func (q *fileTransferServerQueue) hasConnectionCapacityLocked() bool {
	return q.activeConnectionsLocked() < q.connectionLimitLocked()
}

func (q *fileTransferServerQueue) enqueueScanLocked(jobID string, req fileTransferJobCreateRequest) {
	q.PendingScans = append(q.PendingScans, fileTransferPendingScan{JobID: jobID, Req: *cloneFileTransferJobRequest(&req)})
	q.QueuedScans++
	if job := q.Jobs[jobID]; job != nil {
		job.Status = "scan_queued"
		job.UpdatedAt = time.Now().UTC()
	}
}

func (q *fileTransferServerQueue) nextPendingScanLocked() *fileTransferPendingScan {
	for len(q.PendingScans) > 0 {
		scan := q.PendingScans[0]
		q.PendingScans = q.PendingScans[1:]
		if q.QueuedScans > 0 {
			q.QueuedScans--
		}
		job := q.Jobs[scan.JobID]
		if job == nil || job.ScanDone {
			continue
		}
		return &scan
	}
	return nil
}

func (q *fileTransferServerQueue) setMaxConnections(limit int) {
	if limit < 1 {
		limit = filetransferprofile.DefaultMaxConnections
	}
	q.mu.Lock()
	q.MaxConnections = limit
	q.touchLocked()
	q.mu.Unlock()
	signalFileTransferQueue(q)
}

func (s *Server) updateFileTransferQueueLimit(profileID string, limit int) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return
	}
	s.fileTransferJobsMu.Lock()
	queue := s.fileTransferJobQueues[profileID]
	s.fileTransferJobsMu.Unlock()
	if queue != nil {
		queue.setMaxConnections(limit)
	}
}

func (s *Server) tryAcquireFileTransferBrowse(profileID string) (func(), bool) {
	queue := s.fileTransferServerQueue(profileID)
	queue.mu.Lock()
	if !queue.hasConnectionCapacityLocked() {
		queue.mu.Unlock()
		return nil, false
	}
	queue.ActiveBrowses++
	queue.touchLocked()
	queue.mu.Unlock()
	released := false
	return func() {
		queue.mu.Lock()
		if !released {
			released = true
			if queue.ActiveBrowses > 0 {
				queue.ActiveBrowses--
			}
			queue.touchLocked()
		}
		queue.mu.Unlock()
		signalFileTransferQueue(queue)
	}, true
}

func (q *fileTransferServerQueue) scanJobStoppedLocked(jobID string) bool {
	return q.StoppedScanJobs != nil && q.StoppedScanJobs[jobID]
}

func (q *fileTransferServerQueue) existingItemLocked(jobID, operation, source, target string, directory bool) *fileTransferServerItem {
	for _, item := range q.Items {
		if item == nil {
			continue
		}
		if item.JobID == jobID && item.Operation == operation && item.Source == source && item.Target == target && item.Directory == directory {
			return item
		}
	}
	return nil
}

func (q *fileTransferServerQueue) addItem(jobID, kind, direction, source, target, operation string, size int64, directory bool) *fileTransferServerItem {
	q.mu.Lock()
	if q.scanJobStoppedLocked(jobID) {
		q.mu.Unlock()
		return nil
	}
	if existing := q.existingItemLocked(jobID, operation, source, target, directory); existing != nil {
		q.mu.Unlock()
		return existing
	}
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

func (q *fileTransferServerQueue) addResolvedItem(jobID, kind, direction, source, target, operation string, size int64, conflict *fileTransferConflictMeta, decision string, overwrite bool) *fileTransferServerItem {
	q.mu.Lock()
	if q.scanJobStoppedLocked(jobID) {
		q.mu.Unlock()
		return nil
	}
	if existing := q.existingItemLocked(jobID, operation, source, target, false); existing != nil {
		q.mu.Unlock()
		return existing
	}
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
		Conflict:  conflict,
		Decision:  decision,
		Overwrite: overwrite,
	}
	q.Items = append(q.Items, item)
	q.touchLocked()
	q.mu.Unlock()
	signalFileTransferQueue(q)
	return item
}

func (q *fileTransferServerQueue) addConflictItem(jobID, kind, direction, source, target, operation string, size int64, conflict fileTransferConflictMeta) *fileTransferServerItem {
	q.mu.Lock()
	if q.scanJobStoppedLocked(jobID) {
		q.mu.Unlock()
		return nil
	}
	if existing := q.existingItemLocked(jobID, operation, source, target, false); existing != nil {
		q.mu.Unlock()
		return existing
	}
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
	if q.scanJobStoppedLocked(jobID) {
		q.mu.Unlock()
		return
	}
	if q.existingItemLocked(jobID, operation, source, target, false) != nil {
		q.mu.Unlock()
		return
	}
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
	if cancel := q.ScanCancels[jobID]; cancel != nil {
		delete(q.ScanCancels, jobID)
		cancel()
	}
	if job := q.Jobs[jobID]; job != nil {
		job.ScanDone = done
		job.UpdatedAt = time.Now().UTC()
		if errors.Is(scanErr, context.Canceled) {
			job.Error = ""
			job.Status = "stopped"
		} else if scanErr != nil {
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
		case "stopped":
			hasFailed = true
		case "conflict":
			hasConflict = true
		}
	}
	switch {
	case hasRunning:
		job.Status = "running"
	case !job.ScanDone && job.Status == "scan_queued":
		job.Status = "scan_queued"
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

func remoteDeletePathIsDescendant(parent, child string) bool {
	parent = normalizeBackgroundRemotePath(parent)
	child = normalizeBackgroundRemotePath(child)
	if parent == child {
		return false
	}
	if parent == "/" {
		return strings.HasPrefix(child, "/") && child != "/"
	}
	if parent == "." {
		return child != "." && !strings.HasPrefix(child, "/")
	}
	return strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}

func (q *fileTransferServerQueue) remoteDeleteDirectoryReadyLocked(item *fileTransferServerItem) bool {
	if item == nil || item.Operation != "remote_delete" || !item.Directory {
		return true
	}
	var failedChild string
	waiting := false
	for _, child := range q.Items {
		if child == nil || child == item || child.Removed || child.JobID != item.JobID || child.Operation != "remote_delete" {
			continue
		}
		if !remoteDeletePathIsDescendant(item.Source, child.Source) {
			continue
		}
		switch child.Status {
		case "success", "skipped":
			continue
		case "failed", "stopped":
			if failedChild == "" {
				failedChild = child.Source
			}
		default:
			waiting = true
		}
	}
	if failedChild != "" {
		item.Status = "failed"
		item.Error = "directory delete blocked because a child did not complete: " + failedChild
		q.recomputeJobLocked(item.JobID)
		q.touchLocked()
		return false
	}
	return !waiting
}

func (q *fileTransferServerQueue) nextRunnableLocked() *fileTransferServerItem {
	for _, item := range q.Items {
		if item.Removed || item.Status != "queued" || !item.Priority {
			continue
		}
		if !q.remoteDeleteDirectoryReadyLocked(item) {
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
		if !q.remoteDeleteDirectoryReadyLocked(item) {
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
			if !queue.hasConnectionCapacityLocked() {
				queue.mu.Unlock()
				break
			}
			if scan := queue.nextPendingScanLocked(); scan != nil {
				queue.ActiveScans++
				ctx, cancel := context.WithCancel(context.Background())
				if queue.ScanCancels == nil {
					queue.ScanCancels = make(map[string]context.CancelFunc)
				}
				queue.ScanCancels[scan.JobID] = cancel
				if job := queue.Jobs[scan.JobID]; job != nil {
					job.Status = "scanning"
					job.Error = ""
					job.UpdatedAt = time.Now().UTC()
				}
				queue.touchLocked()
				queue.mu.Unlock()
				go s.runFileTransferServerScan(ctx, queue, *scan)
				continue
			}
			item := queue.nextRunnableLocked()
			if item == nil {
				queue.mu.Unlock()
				break
			}
			item.Status = "running"
			item.Error = ""
			item.Attempts++
			queue.ActiveTransfers++
			ctx, cancel := context.WithCancel(context.Background())
			if queue.TransferCancels == nil {
				queue.TransferCancels = make(map[string]context.CancelFunc)
			}
			queue.TransferCancels[item.ID] = cancel
			queue.recomputeJobLocked(item.JobID)
			queue.touchLocked()
			queue.mu.Unlock()
			go s.runFileTransferServerItem(ctx, queue, item)
		}
	}
}

func (s *Server) runFileTransferServerScan(ctx context.Context, queue *fileTransferServerQueue, scan fileTransferPendingScan) {
	var scanErr error
	switch scan.Req.Kind {
	case fileTransferJobHostUpload:
		scanErr = s.scanHostUploadJob(ctx, queue, scan.JobID, scan.Req)
	case fileTransferJobHostDownload:
		scanErr = s.scanHostDownloadJob(ctx, queue, scan.JobID, scan.Req)
	case fileTransferJobRemoteDelete:
		scanErr = s.scanRemoteDeleteJob(ctx, queue, scan.JobID, scan.Req)
	default:
		scanErr = fmt.Errorf("unsupported file-transfer job kind %q", scan.Req.Kind)
	}
	queue.setScanState(scan.JobID, true, scanErr)
}

func (s *Server) runFileTransferServerItem(ctx context.Context, queue *fileTransferServerQueue, item *fileTransferServerItem) {
	err := s.executeFileTransferServerItem(ctx, queue, queue.ProfileID, item)
	queue.mu.Lock()
	if cancel := queue.TransferCancels[item.ID]; cancel != nil {
		delete(queue.TransferCancels, item.ID)
		cancel()
	}
	if errors.Is(err, context.Canceled) {
		item.Status = "stopped"
		item.Error = ""
		item.Detail = "Cancelled"
	} else if err != nil {
		item.Status = "failed"
		item.Error = err.Error()
	} else {
		item.Status = "success"
		item.Error = ""
		if item.Detail == "" {
			item.Detail = "Completed"
		}
	}
	if item.RemoveAfterRun {
		item.Removed = true
	}
	if queue.ActiveTransfers > 0 {
		queue.ActiveTransfers--
	}
	queue.recomputeJobLocked(item.JobID)
	queue.touchLocked()
	queue.mu.Unlock()
	signalFileTransferQueue(queue)
}

func (s *Server) executeFileTransferServerItem(ctx context.Context, queue *fileTransferServerQueue, profileID string, item *fileTransferServerItem) error {
	switch item.Operation {
	case "host_archive_upload":
		return s.backgroundHostArchiveUpload(ctx, queue, item)
	case "host_upload":
		return s.backgroundHostToRemote(ctx, profileID, item.Source, item.Target, item.Attempts)
	case "host_download":
		return s.backgroundRemoteToHost(ctx, profileID, item.Source, item.Target, item.Overwrite, item.Size)
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

func (s *Server) backgroundHostToRemote(ctx context.Context, profileID, hostRel, remotePath string, attempts int) error {
	_, hostPath, info, err := s.resolveHostWorkspaceEntry(hostRel)
	if err != nil { return err }
	if !info.Mode().IsRegular() { return errors.New("host upload source is not a regular file") }
	if info.Size() > maxFileTransferBytes { return fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes) }
	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil { return err }

	resumeOffset := int64(0)
	if attempts > 1 {
		if entry, entryErr := s.backgroundRemoteEntry(ctx, profileID, remotePath); entryErr == nil && entry != nil && entry.Type == "file" && entry.Size > 0 && entry.Size < info.Size() {
			resumeOffset = entry.Size
		}
	}
	file, err := os.Open(hostPath)
	if err != nil { return err }
	defer file.Close()
	if resumeOffset > 0 {
		if _, err := file.Seek(resumeOffset, 0); err != nil { return err }
	}

	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		err = s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			if resumeOffset > 0 {
				resumeErr := client.StoreFrom(ctx, remotePath, resumeOffset, file)
				if resumeErr == nil { return nil }
				if !errors.Is(resumeErr, ftpclient.ErrResumeUnsupported) { return resumeErr }
				if _, seekErr := file.Seek(0, 0); seekErr != nil { return seekErr }
			}
			return client.Store(ctx, remotePath, file)
		})
	case filetransferprofile.ProtocolSFTP:
		var command string
		if resumeOffset > 0 { command, err = sftpclient.ReputCommand(hostPath, remotePath) } else { command, err = sftpclient.PutCommand(hostPath, remotePath) }
		if err == nil { _, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10) }
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil { return err }
	if entry, entryErr := s.backgroundRemoteEntry(ctx, profileID, remotePath); entryErr == nil && entry != nil && entry.Type == "file" && entry.Size != info.Size() {
		return fmt.Errorf("uploaded file size mismatch: remote=%d local=%d", entry.Size, info.Size())
	}
	return nil
}

func (s *Server) backgroundHostTarget(rel string, overwrite bool) (string, error) {
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
		if !overwrite {
			return "", errors.New("host destination file already exists")
		}
	} else if !os.IsNotExist(err) {
		return "", errors.New("host destination unavailable")
	}
	return target, nil
}

func fileTransferPartialHostPath(target, remotePath string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(target) + "\x00" + remotePath))
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".taskdeck-part-"+hex.EncodeToString(sum[:6]))
}

func (s *Server) backgroundRemoteToHost(ctx context.Context, profileID, remotePath, hostRel string, overwrite bool, expectedSize int64) error {
	target, err := s.backgroundHostTarget(hostRel, overwrite)
	if err != nil { return err }
	partial := fileTransferPartialHostPath(target, remotePath)
	if info, statErr := os.Lstat(partial); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() { return errors.New("download partial path is not a regular file") }
		if info.Size() > maxFileTransferBytes || (expectedSize > 0 && info.Size() > expectedSize) {
			if err := os.Remove(partial); err != nil { return err }
		}
	} else if !os.IsNotExist(statErr) { return statErr }

	part, err := os.OpenFile(partial, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil { return err }
	info, err := part.Stat()
	if err != nil { _ = part.Close(); return err }
	resumeOffset := info.Size()
	if resumeOffset > 0 {
		if _, err := part.Seek(resumeOffset, 0); err != nil { _ = part.Close(); return err }
	}

	profile, err := s.resolveFileTransferProfile(profileID)
	if err != nil { _ = part.Close(); return err }
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		writer := &transferLimitWriter{dst: part, remaining: maxFileTransferBytes - resumeOffset}
		err = s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			if resumeOffset > 0 {
				resumeErr := client.RetrieveFrom(ctx, remotePath, resumeOffset, writer)
				if resumeErr == nil { return nil }
				if !errors.Is(resumeErr, ftpclient.ErrResumeUnsupported) { return resumeErr }
				if err := part.Truncate(0); err != nil { return err }
				if _, err := part.Seek(0, 0); err != nil { return err }
				writer.remaining = maxFileTransferBytes
			}
			return client.Retrieve(ctx, remotePath, writer)
		})
		if syncErr := part.Sync(); err == nil { err = syncErr }
		if closeErr := part.Close(); err == nil { err = closeErr }
	case filetransferprofile.ProtocolSFTP:
		if closeErr := part.Close(); closeErr != nil { return closeErr }
		var command string
		if resumeOffset > 0 { command, err = sftpclient.RegetCommand(remotePath, partial) } else { command, err = sftpclient.GetCommand(remotePath, partial) }
		if err == nil { _, err = s.runSFTP(ctx, profile, command+"quit\n", 64<<10) }
	default:
		_ = part.Close()
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil { return err }

	finalInfo, err := os.Stat(partial)
	if err != nil || !finalInfo.Mode().IsRegular() { return errors.New("downloaded host partial file unavailable") }
	if finalInfo.Size() > maxFileTransferBytes { return fmt.Errorf("file transfer exceeds %d bytes", maxFileTransferBytes) }
	if expectedSize > 0 && finalInfo.Size() != expectedSize {
		return fmt.Errorf("downloaded file size mismatch: got=%d expected=%d; partial file kept for retry", finalInfo.Size(), expectedSize)
	}
	if err := os.Chmod(partial, 0o644); err != nil { return err }
	if _, err := s.backgroundHostTarget(hostRel, overwrite); err != nil { return err }
	if overwrite {
		if existing, err := os.Lstat(target); err == nil {
			if existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular() { return errors.New("host destination changed to unsafe file type") }
			if err := os.Remove(target); err != nil { return err }
		} else if !os.IsNotExist(err) { return err }
	}
	return os.Rename(partial, target)
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

func (s *Server) scanHostUploadJob(ctx context.Context, queue *fileTransferServerQueue, jobID string, req fileTransferJobCreateRequest) error {
	knownRemote := map[string]bool{normalizeBackgroundRemotePath(req.RemoteDir): true, ".": true, "/": true}
	listings := make(map[string][]fileTransferEntry)
	remoteExisting := func(remotePath string) (*fileTransferEntry, error) {
		parent := normalizeBackgroundRemotePath(pathpkg.Dir(normalizeBackgroundRemotePath(remotePath)))
		name := pathpkg.Base(normalizeBackgroundRemotePath(remotePath))
		entries, ok := listings[parent]
		if !ok {
			var err error
			entries, err = s.backgroundListRemote(ctx, req.ProfileID, parent)
			if err != nil {
				return nil, err
			}
			listings[parent] = entries
		}
		for _, entry := range entries {
			if entry.Name == name {
				copy := entry
				return &copy, nil
			}
		}
		return nil, nil
	}
	var walk func(string, string) error
	walk = func(hostRel, remotePath string) error {
		if err := ctx.Err(); err != nil { return err }
		rel, absolute, info, err := s.resolveHostWorkspaceEntry(hostRel)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := s.ensureBackgroundRemoteDirectory(ctx, req.ProfileID, remotePath, knownRemote); err != nil {
				return err
			}
			delete(listings, normalizeBackgroundRemotePath(pathpkg.Dir(remotePath)))
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
		existing, err := remoteExisting(remotePath)
		if err != nil {
			return err
		}
		if existing == nil {
			queue.addItem(jobID, "Upload", "→", rel, remotePath, "host_upload", info.Size(), false)
			return nil
		}
		if existing.Type != "file" {
			return fmt.Errorf("remote destination exists and is not a file: %s", remotePath)
		}
		conflict := &fileTransferConflictMeta{
			SourceSize: info.Size(), TargetSize: existing.Size,
			SourceModified: info.ModTime().UTC().Format(time.RFC3339Nano),
			TargetModified: existing.Modified,
		}
		return s.enqueueServerTransferConflictAware(ctx, queue, jobID, "Upload", "→", rel, remotePath, "host_upload", info.Size(), conflict)
	}
	for _, requested := range req.HostPaths {
		if err := ctx.Err(); err != nil { return err }
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
			if err := walk(rel, remoteTarget); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) scanHostDownloadJob(ctx context.Context, queue *fileTransferServerQueue, jobID string, req fileTransferJobCreateRequest) error {
	if _, err := s.ensureBackgroundHostDirectory(req.HostDir); err != nil {
		return err
	}
	var walk func(fileTransferJobTarget, string) error
	walk = func(target fileTransferJobTarget, hostParent string) error {
		if err := ctx.Err(); err != nil { return err }
		target.Path = normalizeBackgroundRemotePath(target.Path)
		name := pathpkg.Base(target.Path)
		hostRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(hostParent), name))
		if !target.Directory {
			if target.Size == 0 && target.Modified == "" {
				entry, err := s.backgroundRemoteEntry(ctx, req.ProfileID, target.Path)
				if err != nil {
					return err
				}
				if entry == nil {
					return fmt.Errorf("remote source not found: %s", target.Path)
				}
				if entry.Type != "file" {
					return fmt.Errorf("remote source is not a file: %s", target.Path)
				}
				target.Size = entry.Size
				target.Modified = entry.Modified
			}
			_, existing, exists, err := s.backgroundHostExistingMeta(hostRel)
			if err != nil {
				return err
			}
			if !exists {
				queue.addItem(jobID, "Download", "←", target.Path, hostRel, "host_download", target.Size, false)
				return nil
			}
			conflict := &fileTransferConflictMeta{
				SourceSize: target.Size, TargetSize: existing.Size(),
				SourceModified: target.Modified,
				TargetModified: existing.ModTime().UTC().Format(time.RFC3339Nano),
			}
			return s.enqueueServerTransferConflictAware(ctx, queue, jobID, "Download", "←", target.Path, hostRel, "host_download", target.Size, conflict)
		}
		if _, err := s.ensureBackgroundHostDirectory(hostRel); err != nil {
			return err
		}
		entries, err := s.backgroundListRemote(ctx, req.ProfileID, target.Path)
		if err != nil {
			exists, checkErr := s.backgroundRemoteEntryExists(ctx, req.ProfileID, target.Path)
			if checkErr == nil && !exists {
				return nil
			}
			return err
		}
		for _, entry := range entries {
			child := fileTransferJobTarget{
				Path:      joinBackgroundRemotePath(target.Path, entry.Name),
				Directory: entry.Type == "directory",
				Size:      entry.Size,
				Modified:  entry.Modified,
			}
			if err := walk(child, hostRel); err != nil {
				return err
			}
		}
		return nil
	}
	for _, target := range req.RemoteTargets {
		if err := ctx.Err(); err != nil { return err }
		if err := walk(target, req.HostDir); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) scanRemoteDeleteJob(ctx context.Context, queue *fileTransferServerQueue, jobID string, req fileTransferJobCreateRequest) error {
	var walk func(fileTransferJobTarget) error
	walk = func(target fileTransferJobTarget) error {
		if err := ctx.Err(); err != nil { return err }
		target.Path = normalizeBackgroundRemotePath(target.Path)
		if !target.Directory {
			queue.addItem(jobID, "Delete", "×", target.Path, "", "remote_delete", 0, false)
			return nil
		}
		entries, err := s.backgroundListRemote(ctx, req.ProfileID, target.Path)
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
		if err := ctx.Err(); err != nil { return err }
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
	case fileTransferJobHostUpload, fileTransferJobHostArchiveUpload:
		if len(req.HostPaths) == 0 {
			return nil, errors.New("host_paths are required")
		}
		if req.Kind == fileTransferJobHostArchiveUpload {
			req.MergePolicy, err = normalizeFileTransferArchiveMergePolicy(req.MergePolicy)
			if err != nil {
				return nil, err
			}
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
		Status: "scan_queued", CreatedAt: now, UpdatedAt: now, ConflictPolicy: req.ConflictPolicy,
		Request: cloneFileTransferJobRequest(&req),
	}
	queue := s.fileTransferServerQueue(req.ProfileID)
	queue.mu.Lock()
	queue.Jobs[jobID] = job
	if req.Kind == fileTransferJobHostArchiveUpload {
		job.Status = "queued"
		job.ScanDone = true
		job.Phase = "queued"
		queue.Sequence++
		source := req.HostPaths[0]
		if len(req.HostPaths) > 1 {
			source = fmt.Sprintf("%s (+%d selected)", source, len(req.HostPaths)-1)
		}
		queue.Items = append(queue.Items, &fileTransferServerItem{
			ID: fmt.Sprintf("srv-%s-%d", jobID, queue.Sequence), JobID: jobID,
			Kind: "Compressed upload", Direction: "→", Source: source, Target: req.RemoteDir,
			Status: "queued", Operation: "host_archive_upload", Detail: "Waiting to compress",
		})
	} else {
		queue.enqueueScanLocked(jobID, req)
	}
	queue.touchLocked()
	queue.mu.Unlock()
	signalFileTransferQueue(queue)
	return cloneFileTransferJob(job), nil
}

func decodeFileTransferJobJSON(w http.ResponseWriter, r *http.Request, value any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxFileTransferJobJSONBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return fmt.Errorf("invalid file-transfer job request: %w", err)
	}
	return nil
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
		if err := decodeFileTransferJobJSON(w, r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		permission := identity.PermissionTransferRead
		switch strings.TrimSpace(req.Kind) {
		case fileTransferJobHostUpload, fileTransferJobHostArchiveUpload:
			permission = identity.PermissionTransferUpload
		case fileTransferJobRemoteDelete:
			permission = identity.PermissionTransferDelete
		case fileTransferJobHostDownload:
			permission = identity.PermissionTransferRead
		}
		if !s.requireSharedActionPermission(w, r, permission, req.Kind, "file_transfer:"+req.ProfileID) {
			return
		}
		if strings.TrimSpace(req.Kind) == fileTransferJobHostArchiveUpload && req.AutoExtract {
			if !s.requireSharedActionPermission(w, r, identity.PermissionSSHUse, req.Kind, "file_transfer:"+req.ProfileID) {
				return
			}
		}
		if strings.TrimSpace(req.Kind) == fileTransferJobRemoteDelete {
			directoryPaths := make([]string, 0, len(req.RemoteTargets))
			for _, target := range req.RemoteTargets {
				if target.Directory {
					if path := strings.TrimSpace(target.Path); path != "" {
						directoryPaths = append(directoryPaths, path)
					}
				}
			}
			if len(directoryPaths) > 0 {
				resource := req.ProfileID + ":" + directoryPaths[0]
				if len(directoryPaths) > 1 {
					resource = fmt.Sprintf("%s (+%d directories)", resource, len(directoryPaths)-1)
				}
				if !s.requireDangerousApproval(w, r, "transfer.recursive_delete", resource) {
					return
				}
			}
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

func stopFileTransferScansLocked(queue *fileTransferServerQueue) {
	if queue.StoppedScanJobs == nil {
		queue.StoppedScanJobs = make(map[string]bool)
	}
	for jobID, cancel := range queue.ScanCancels {
		queue.StoppedScanJobs[jobID] = true
		if cancel != nil {
			cancel()
		}
	}
	for _, scan := range queue.PendingScans {
		queue.StoppedScanJobs[scan.JobID] = true
	}
	queue.PendingScans = nil
	queue.QueuedScans = 0
	for _, job := range queue.Jobs {
		if job == nil || job.ScanDone {
			continue
		}
		job.ScanDone = true
		job.NeedsRescan = false
		job.Error = ""
		job.Status = "stopped"
		job.UpdatedAt = time.Now().UTC()
	}
}

func clearFileTransferQueueLocked(queue *fileTransferServerQueue) {
	stopFileTransferScansLocked(queue)
	if queue.StoppedScanJobs == nil {
		queue.StoppedScanJobs = make(map[string]bool)
	}
	for jobID := range queue.Jobs {
		queue.StoppedScanJobs[jobID] = true
	}
	kept := queue.Items[:0]
	activeJobs := make(map[string]bool)
	for _, item := range queue.Items {
		if item == nil {
			continue
		}
		if item.Status == "running" {
			item.RemoveAfterRun = true
			if cancel := queue.TransferCancels[item.ID]; cancel != nil {
				cancel()
			}
			kept = append(kept, item)
			activeJobs[item.JobID] = true
		}
	}
	queue.Items = kept
	for jobID := range queue.Jobs {
		if !activeJobs[jobID] {
			delete(queue.Jobs, jobID)
		}
	}
}

func (s *Server) fileTransferJobsControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferJobControlRequest
	if err := decodeFileTransferJobJSON(w, r, &req); err != nil {
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
	if req.Action == "resolve_conflict" {
		if err := s.resolveServerConflicts(queue, req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, queue.snapshot())
		return
	}
	selected := make(map[string]bool, len(req.ItemIDs))
	for _, id := range req.ItemIDs {
		selected[strings.TrimSpace(id)] = true
	}
	queue.mu.Lock()
	switch req.Action {
	case "pause":
		queue.Paused = true
	case "stop_scans":
		stopFileTransferScansLocked(queue)
	case "clear_queue":
		clearFileTransferQueueLocked(queue)
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
				if cancel := queue.TransferCancels[item.ID]; cancel != nil {
					cancel()
				}
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
			if item.Status == "success" || item.Status == "skipped" {
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
	if req.Action == "resume" {
		s.restartRecoveredFileTransferScans(queue)
	}
	signalFileTransferQueue(queue)
	writeJSON(w, http.StatusOK, queue.snapshot())
}
