package server

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	maxFileTransferCompressedArchiveBytes int64 = 64 << 30
	fileTransferArchiveProgressInterval         = 500 * time.Millisecond
)

type fileTransferArchiveProgress struct {
	Files        int64
	Dirs         int64
	SourceBytes  int64
	ArchiveBytes int64
}

type fileTransferArchiveCountingWriter struct {
	dst io.Writer
	n   int64
}

func (w *fileTransferArchiveCountingWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > maxFileTransferCompressedArchiveBytes-w.n {
		return 0, fmt.Errorf("compressed upload archive exceeds %d bytes", maxFileTransferCompressedArchiveBytes)
	}
	n, err := w.dst.Write(p)
	w.n += int64(n)
	return n, err
}

type fileTransferContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r fileTransferContextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}

func writeLargeFileTransferTarGz(ctx context.Context, writer io.Writer, sources []projectArchiveSource, progress func(fileTransferArchiveProgress)) error {
	counting := &fileTransferArchiveCountingWriter{dst: writer}
	gz := gzip.NewWriter(counting)
	tw := tar.NewWriter(gz)
	state := fileTransferArchiveProgress{}
	lastReport := time.Now()
	report := func(force bool) {
		state.ArchiveBytes = counting.n
		if progress != nil && (force || time.Since(lastReport) >= fileTransferArchiveProgressInterval) {
			progress(state)
			lastReport = time.Now()
		}
	}
	for _, source := range sources {
		err := filepath.Walk(source.Path, func(current string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink compressed-upload input is not supported: %s", current)
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return fmt.Errorf("special compressed-upload input is not supported: %s", current)
			}
			rel, err := filepath.Rel(source.Path, current)
			if err != nil {
				return err
			}
			name := source.Base
			if rel != "." {
				name = filepath.ToSlash(filepath.Join(source.Base, rel))
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(name)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				state.Dirs++
				report(false)
				return nil
			}
			state.Files++
			if info.Size() > 0 && state.SourceBytes <= (1<<63-1)-info.Size() {
				state.SourceBytes += info.Size()
			}
			file, err := os.Open(current)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyBuffer(tw, fileTransferContextReader{ctx: ctx, r: file}, make([]byte, 256<<10))
			closeErr := file.Close()
			report(false)
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	report(true)
	return nil
}

func updateCompressedUploadProgress(queue *fileTransferServerQueue, item *fileTransferServerItem, phase, detail string, state fileTransferArchiveProgress, archiveBytes int64) {
	queue.mu.Lock()
	item.Detail = detail
	item.Files = state.Files
	item.BytesDone = state.SourceBytes
	if archiveBytes > 0 {
		item.Size = archiveBytes
	}
	if job := queue.Jobs[item.JobID]; job != nil {
		job.Phase = phase
		job.Files = state.Files
		job.Bytes = state.SourceBytes
		if archiveBytes > 0 {
			job.ArchiveBytes = archiveBytes
		} else if state.ArchiveBytes > 0 {
			job.ArchiveBytes = state.ArchiveBytes
		}
		job.UpdatedAt = time.Now().UTC()
	}
	queue.touchLocked()
	queue.mu.Unlock()
}

func bestEffortRemoveCompressedRemoteArchive(s *Server, profileID, remoteArchive string) {
	if remoteArchive == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.backgroundRemoteMutation(ctx, profileID, "delete", remoteArchive, "", false)
}

func (s *Server) backgroundHostArchiveUpload(ctx context.Context, queue *fileTransferServerQueue, item *fileTransferServerItem) error {
	queue.mu.Lock()
	job := queue.Jobs[item.JobID]
	var req *fileTransferJobCreateRequest
	if job != nil {
		req = cloneFileTransferJobRequest(job.Request)
	}
	queue.mu.Unlock()
	if req == nil {
		return errors.New("compressed upload job request is unavailable")
	}
	mergePolicy, err := normalizeFileTransferArchiveMergePolicy(req.MergePolicy)
	if err != nil {
		return err
	}
	sources, err := s.collectProjectArchiveSources(req.HostPaths, "")
	if err != nil {
		return err
	}
	roots := fileTransferArchiveRoots(sources)
	if _, err := validateFileTransferArchiveRoots(roots); err != nil {
		return err
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "taskdeck-folder-upload-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create compressed-upload temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}

	updateCompressedUploadProgress(queue, item, "compressing", "Compressing selected folders", fileTransferArchiveProgress{}, 0)
	var last fileTransferArchiveProgress
	err = writeLargeFileTransferTarGz(ctx, tmp, sources, func(state fileTransferArchiveProgress) {
		last = state
		updateCompressedUploadProgress(queue, item, "compressing",
			fmt.Sprintf("Compressing · %d files · %d MiB source", state.Files, state.SourceBytes>>20), state, 0)
	})
	if err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	info, err := os.Stat(tmpPath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("compressed-upload archive temp file unavailable")
	}
	last.ArchiveBytes = info.Size()

	id, err := newFileTransferBackgroundID("folder")
	if err != nil {
		return err
	}
	remoteArchive := joinBackgroundRemotePath(req.RemoteDir, ".taskdeck-"+id+".tar.gz")
	commands := fileTransferManualExtractCommandsWithPolicy("tar.gz", remoteArchive, req.RemoteDir, roots, mergePolicy)
	queue.mu.Lock()
	if live := queue.Jobs[item.JobID]; live != nil {
		live.RemoteArchive = remoteArchive
		live.RemoteDestination = req.RemoteDir
		live.ArchiveBytes = info.Size()
		live.ManualCommands = commands
	}
	queue.touchLocked()
	queue.mu.Unlock()

	updateCompressedUploadProgress(queue, item, "uploading", fmt.Sprintf("Uploading archive · %d MiB", info.Size()>>20), last, info.Size())
	if err := s.uploadTemporaryArchive(ctx, profile, tmpPath, remoteArchive); err != nil {
		bestEffortRemoveCompressedRemoteArchive(s, req.ProfileID, remoteArchive)
		return err
	}
	select {
	case <-ctx.Done():
		bestEffortRemoveCompressedRemoteArchive(s, req.ProfileID, remoteArchive)
		return ctx.Err()
	default:
	}

	if !req.AutoExtract || profile.Protocol != "sftp" {
		queue.mu.Lock()
		if live := queue.Jobs[item.JobID]; live != nil {
			live.Phase = "manual_extract"
			live.NeedsManualExtract = true
			live.UpdatedAt = time.Now().UTC()
		}
		item.Detail = "Archive uploaded · manual extraction required"
		queue.touchLocked()
		queue.mu.Unlock()
		return nil
	}

	updateCompressedUploadProgress(queue, item, "extracting", "Extracting archive through SSH", last, info.Size())
	command, err := remoteArchiveExtractCommandWithPolicy("tar.gz", remoteArchive, req.RemoteDir, roots, mergePolicy)
	if err != nil {
		return err
	}
	if _, err := s.runSFTPLinkedSSHCommand(ctx, profile, command); err != nil {
		queue.mu.Lock()
		if live := queue.Jobs[item.JobID]; live != nil {
			live.Phase = "manual_extract"
			live.NeedsManualExtract = true
			live.UpdatedAt = time.Now().UTC()
		}
		item.Detail = "Auto-extract failed · archive kept"
		queue.touchLocked()
		queue.mu.Unlock()
		return fmt.Errorf("automatic extraction failed; archive kept at %s: %w", remoteArchive, err)
	}

	queue.mu.Lock()
	if live := queue.Jobs[item.JobID]; live != nil {
		live.Phase = "done"
		live.NeedsManualExtract = false
		live.ManualCommands = nil
		live.RemoteArchive = ""
		live.UpdatedAt = time.Now().UTC()
	}
	item.Detail = fmt.Sprintf("Completed · %d files", last.Files)
	queue.touchLocked()
	queue.mu.Unlock()
	return nil
}
