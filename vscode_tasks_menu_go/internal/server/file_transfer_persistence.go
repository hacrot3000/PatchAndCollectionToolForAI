package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	fileTransferQueueStateFile = "vscode_tasks_menu.filetransfer.jobs.json"
	fileTransferQueueStateMax  = 16 << 20
)

type fileTransferQueueDiskState struct {
	Version int                         `json:"version"`
	Queues  []fileTransferQueueDiskQueue `json:"queues"`
}

type fileTransferQueueDiskQueue struct {
	ProfileID              string                      `json:"profile_id"`
	Paused                 bool                        `json:"paused"`
	UploadConflictPolicy   string                      `json:"upload_conflict_policy,omitempty"`
	DownloadConflictPolicy string                      `json:"download_conflict_policy,omitempty"`
	Sequence               uint64                      `json:"sequence"`
	Revision               uint64                      `json:"revision"`
	Jobs                   []fileTransferQueueDiskJob  `json:"jobs"`
	Items                  []fileTransferQueueDiskItem `json:"items"`
}

type fileTransferQueueDiskJob struct {
	Job     fileTransferServerJob       `json:"job"`
	Request *fileTransferJobCreateRequest `json:"request,omitempty"`
}

type fileTransferQueueDiskItem struct {
	Item           fileTransferServerItem `json:"item"`
	Operation      string                 `json:"operation"`
	Directory      bool                   `json:"directory,omitempty"`
	Overwrite      bool                   `json:"overwrite,omitempty"`
	Priority       bool                   `json:"priority,omitempty"`
	Removed        bool                   `json:"removed,omitempty"`
	RemoveAfterRun bool                   `json:"remove_after_run,omitempty"`
}

func (s *Server) fileTransferQueueStatePath() (string, error) {
	return projectfiles.Resolve(s.Workspace, fileTransferQueueStateFile)
}

func (s *Server) scheduleFileTransferQueuePersist() {
	s.fileTransferPersistMu.Lock()
	defer s.fileTransferPersistMu.Unlock()
	if s.fileTransferPersistTimer != nil {
		s.fileTransferPersistTimer.Stop()
	}
	s.fileTransferPersistTimer = time.AfterFunc(120*time.Millisecond, func() {
		if err := s.persistFileTransferQueues(); err != nil && s.Log != nil {
			s.Log.Printf("file-transfer queue persist warning: %v", err)
		}
	})
}

func (s *Server) fileTransferDiskSnapshot() fileTransferQueueDiskState {
	s.fileTransferJobsMu.Lock()
	queues := make([]*fileTransferServerQueue, 0, len(s.fileTransferJobQueues))
	for _, queue := range s.fileTransferJobQueues {
		queues = append(queues, queue)
	}
	s.fileTransferJobsMu.Unlock()

	state := fileTransferQueueDiskState{Version: 1, Queues: make([]fileTransferQueueDiskQueue, 0, len(queues))}
	for _, queue := range queues {
		queue.mu.Lock()
		disk := fileTransferQueueDiskQueue{
			ProfileID: queue.ProfileID, Paused: queue.Paused,
			UploadConflictPolicy: queue.UploadConflictPolicy,
			DownloadConflictPolicy: queue.DownloadConflictPolicy,
			Sequence: queue.Sequence, Revision: queue.Revision,
			Jobs: make([]fileTransferQueueDiskJob, 0, len(queue.Jobs)),
			Items: make([]fileTransferQueueDiskItem, 0, len(queue.Items)),
		}
		jobIDs := make([]string, 0, len(queue.Jobs))
		for id := range queue.Jobs {
			jobIDs = append(jobIDs, id)
		}
		sort.Strings(jobIDs)
		for _, id := range jobIDs {
			job := queue.Jobs[id]
			if job == nil {
				continue
			}
			copy := *job
			copy.Request = nil
			disk.Jobs = append(disk.Jobs, fileTransferQueueDiskJob{Job: copy, Request: cloneFileTransferJobRequest(job.Request)})
		}
		for _, item := range queue.Items {
			if item == nil {
				continue
			}
			copy := *item
			if item.Conflict != nil {
				conflict := *item.Conflict
				copy.Conflict = &conflict
			}
			copy.Operation = ""
			copy.Directory = false
			copy.Overwrite = false
			copy.Priority = false
			copy.Removed = false
			copy.RemoveAfterRun = false
			disk.Items = append(disk.Items, fileTransferQueueDiskItem{
				Item: copy, Operation: item.Operation, Directory: item.Directory,
				Overwrite: item.Overwrite, Priority: item.Priority, Removed: item.Removed,
				RemoveAfterRun: item.RemoveAfterRun,
			})
		}
		queue.mu.Unlock()
		state.Queues = append(state.Queues, disk)
	}
	sort.Slice(state.Queues, func(i, j int) bool { return state.Queues[i].ProfileID < state.Queues[j].ProfileID })
	return state
}

func cloneFileTransferJobRequest(req *fileTransferJobCreateRequest) *fileTransferJobCreateRequest {
	if req == nil {
		return nil
	}
	copy := *req
	copy.HostPaths = append([]string(nil), req.HostPaths...)
	copy.RemoteTargets = append([]fileTransferJobTarget(nil), req.RemoteTargets...)
	return &copy
}

func (s *Server) persistFileTransferQueues() error {
	state := s.fileTransferDiskSnapshot()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > fileTransferQueueStateMax {
		return fmt.Errorf("file-transfer queue state exceeds %d bytes", fileTransferQueueStateMax)
	}
	path, err := s.fileTransferQueueStatePath()
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("file-transfer queue state path is not a safe regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".taskdeck-ft-jobs-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *Server) readFileTransferQueueState() (fileTransferQueueDiskState, error) {
	path, err := s.fileTransferQueueStatePath()
	if err != nil {
		return fileTransferQueueDiskState{}, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return fileTransferQueueDiskState{Version: 1}, nil
	}
	if err != nil {
		return fileTransferQueueDiskState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fileTransferQueueDiskState{}, errors.New("file-transfer queue state is not a safe regular file")
	}
	if info.Size() > fileTransferQueueStateMax {
		return fileTransferQueueDiskState{}, fmt.Errorf("file-transfer queue state exceeds %d bytes", fileTransferQueueStateMax)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileTransferQueueDiskState{}, err
	}
	var state fileTransferQueueDiskState
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return fileTransferQueueDiskState{}, fmt.Errorf("decode file-transfer queue state: %w", err)
	}
	if state.Version != 1 {
		return fileTransferQueueDiskState{}, fmt.Errorf("unsupported file-transfer queue state version %d", state.Version)
	}
	return state, nil
}

func (s *Server) ensureFileTransferQueuesLoadedLocked() {
	if s.fileTransferJobsLoaded {
		return
	}
	s.fileTransferJobsLoaded = true
	if s.fileTransferJobQueues == nil {
		s.fileTransferJobQueues = make(map[string]*fileTransferServerQueue)
	}
	state, err := s.readFileTransferQueueState()
	if err != nil {
		if s.Log != nil {
			s.Log.Printf("file-transfer queue restore warning: %v", err)
		}
		return
	}
	for _, disk := range state.Queues {
		profileID := strings.TrimSpace(disk.ProfileID)
		if profileID == "" || len(disk.Items) > 200000 || len(disk.Jobs) > 10000 {
			continue
		}
		limit := filetransferprofile.DefaultMaxConnections
		if profile, profileErr := s.resolveFileTransferProfile(profileID); profileErr == nil && profile.MaxConnections > 0 {
			limit = profile.MaxConnections
		}
		queue := &fileTransferServerQueue{
			ProfileID: profileID, Paused: disk.Paused,
			UploadConflictPolicy: disk.UploadConflictPolicy,
			DownloadConflictPolicy: disk.DownloadConflictPolicy,
			Sequence: disk.Sequence, Revision: disk.Revision,
			MaxConnections: limit,
			Items: make([]*fileTransferServerItem, 0, len(disk.Items)),
			Jobs: make(map[string]*fileTransferServerJob),
			wake: make(chan struct{}, 1),
			Recovered: true,
			persist: s.scheduleFileTransferQueuePersist,
		}
		hasPending := false
		for _, entry := range disk.Jobs {
			job := entry.Job
			if strings.TrimSpace(job.ID) == "" {
				continue
			}
			job.Request = cloneFileTransferJobRequest(entry.Request)
			if !job.ScanDone {
				job.NeedsRescan = job.Request != nil
				job.Status = "queued"
				job.Recovered = true
				hasPending = true
			}
			queue.Jobs[job.ID] = &job
		}
		for _, entry := range disk.Items {
			item := entry.Item
			item.Operation = entry.Operation
			item.Directory = entry.Directory
			item.Overwrite = entry.Overwrite
			item.Priority = false
			item.Removed = entry.Removed
			item.RemoveAfterRun = false
			if item.Status == "running" {
				item.Status = "queued"
				item.Error = "Recovered after TaskDeck daemon restart; resume queue to continue."
			}
			if item.Status == "queued" || item.Status == "conflict" || item.Status == "failed" {
				hasPending = true
			}
			queue.Items = append(queue.Items, &item)
		}
		if hasPending {
			queue.Paused = true
		}
		s.fileTransferJobQueues[profileID] = queue
		queue.workerOnce.Do(func() { go s.runFileTransferServerQueue(queue) })
	}
}

func (s *Server) restartRecoveredFileTransferScans(queue *fileTransferServerQueue) {
	if queue == nil {
		return
	}
	queue.mu.Lock()
	queued := 0
	for id, job := range queue.Jobs {
		if job == nil || job.ScanDone || !job.NeedsRescan || job.Request == nil {
			continue
		}
		req := *cloneFileTransferJobRequest(job.Request)
		job.NeedsRescan = false
		job.Recovered = false
		job.Error = ""
		queue.enqueueScanLocked(id, req)
		queued++
	}
	if queued > 0 {
		queue.touchLocked()
	}
	queue.mu.Unlock()
	if queued > 0 {
		signalFileTransferQueue(queue)
	}
}

