package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

const (
	fileTransferUploadQuickScanFiles    = 200
	fileTransferUploadQuickScanSoftFiles = 100
	fileTransferUploadQuickScanDuration = 2500 * time.Millisecond
)

type fileTransferUploadQuickScanRequest struct {
	Paths []string `json:"paths"`
}

type fileTransferUploadQuickScanResult struct {
	Files     int   `json:"files"`
	Dirs      int   `json:"dirs"`
	Bytes     int64 `json:"bytes"`
	Complete  bool  `json:"complete"`
	TimedOut  bool  `json:"timed_out"`
	ManyFiles bool  `json:"many_files"`
	ElapsedMS int64 `json:"elapsed_ms"`
}

var errFileTransferQuickScanStop = errors.New("file-transfer quick scan stop")

func quickScanProjectArchiveSources(sources []projectArchiveSource) (fileTransferUploadQuickScanResult, error) {
	started := time.Now()
	deadline := started.Add(fileTransferUploadQuickScanDuration)
	result := fileTransferUploadQuickScanResult{Complete: true}
	for _, source := range sources {
		err := filepath.Walk(source.Path, func(current string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if time.Now().After(deadline) {
				result.Complete = false
				result.TimedOut = true
				return errFileTransferQuickScanStop
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return nil
			}
			if info.IsDir() {
				result.Dirs++
			} else {
				result.Files++
				if info.Size() > 0 && result.Bytes <= maxProjectArchiveTotalBytes-info.Size() {
					result.Bytes += info.Size()
				}
				if result.Files >= fileTransferUploadQuickScanFiles {
					result.Complete = false
					return errFileTransferQuickScanStop
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, errFileTransferQuickScanStop) {
			return fileTransferUploadQuickScanResult{}, err
		}
		if errors.Is(err, errFileTransferQuickScanStop) {
			break
		}
	}
	result.ManyFiles = result.Files >= fileTransferUploadQuickScanFiles ||
		(result.TimedOut && result.Files >= fileTransferUploadQuickScanSoftFiles)
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result, nil
}

func (s *Server) fileTransferUploadQuickScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferUploadQuickScanRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sources, err := s.collectProjectArchiveSources(req.Paths, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := quickScanProjectArchiveSources(sources)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type fileTransferArchiveUploadRequest struct {
	ProfileID string   `json:"profile_id"`
	HostPaths []string `json:"host_paths"`
	RemoteDir string   `json:"remote_dir"`
}

type fileTransferArchiveExtractRequest struct {
	ProfileID        string   `json:"profile_id"`
	RemoteArchive    string   `json:"remote_archive"`
	RemoteDestination string  `json:"remote_destination"`
	Roots            []string `json:"roots"`
	Format           string   `json:"format,omitempty"`
	MergePolicy      string   `json:"merge_policy,omitempty"`
}

func fileTransferArchiveRoots(sources []projectArchiveSource) []string {
	seen := make(map[string]bool, len(sources))
	roots := make([]string, 0, len(sources))
	for _, source := range sources {
		root := strings.TrimSpace(source.Base)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

func validateFileTransferArchiveRoots(roots []string) ([]string, error) {
	if len(roots) == 0 || len(roots) > 512 {
		return nil, errors.New("archive roots are required")
	}
	out := make([]string, 0, len(roots))
	seen := make(map[string]bool, len(roots))
	for _, value := range roots {
		value = strings.TrimSpace(value)
		if value == "" || value == "." || value == ".." || filepath.Base(value) != value ||
			strings.ContainsAny(value, "/\\\x00\r\n") {
			return nil, errors.New("archive root names must be safe top-level names")
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out, nil
}

func fileTransferPowerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func fileTransferManualExtractCommands(format, archivePath, destination string, roots []string) map[string]string {
	archiveQ := remoteArchiveShellQuote(archivePath)
	destQ := remoteArchiveShellQuote(destination)
	posixChecks := ""
	for _, root := range roots {
		target := strings.TrimSuffix(destination, "/") + "/" + root
		posixChecks += "test ! -e " + remoteArchiveShellQuote(target) + "; "
	}
	posixExtract := "tar -xzf " + archiveQ + " -C " + destQ
	if format == "zip" {
		posixExtract = "unzip -q -- " + archiveQ + " -d " + destQ
	}
	posix := "set -eu; test -d " + destQ + "; " + posixChecks + posixExtract + " && rm -f -- " + archiveQ

	psArchive := fileTransferPowerShellQuote(archivePath)
	psDest := fileTransferPowerShellQuote(destination)
	psChecks := ""
	for _, root := range roots {
		psChecks += "if (Test-Path -LiteralPath (Join-Path $dest " + fileTransferPowerShellQuote(root) + ")) { throw 'Destination entry already exists: " + strings.ReplaceAll(root, "'", "''") + "' }; "
	}
	psExtract := "tar -xzf $archive -C $dest; if ($LASTEXITCODE -ne 0) { throw 'tar extraction failed' }"
	if format == "zip" {
		psExtract = "Expand-Archive -LiteralPath $archive -DestinationPath $dest -ErrorAction Stop"
	}
	powershell := "$archive=" + psArchive + "; $dest=" + psDest + "; if (-not (Test-Path -LiteralPath $dest -PathType Container)) { throw 'Destination directory not found' }; " + psChecks + psExtract + "; Remove-Item -LiteralPath $archive"
	return map[string]string{"posix": posix, "powershell": powershell}
}

func remoteArchiveExtractIntoExistingCommand(format, archivePath, destination string, roots []string) (string, error) {
	if format != "tar.gz" && format != "zip" {
		return "", errors.New("unsupported remote archive format")
	}
	commands := fileTransferManualExtractCommands(format, archivePath, destination, roots)
	command := commands["posix"]
	if format == "tar.gz" {
		archive := remoteArchiveShellQuote(archivePath)
		dest := remoteArchiveShellQuote(destination)
		checks := ""
		for _, root := range roots {
			target := strings.TrimSuffix(destination, "/") + "/" + root
			checks += "test ! -e " + remoteArchiveShellQuote(target) + "; "
		}
		command = "set -eu; test -d " + dest + "; " + checks +
			"command -v tar >/dev/null 2>&1; tar -xzf " + archive + " -C " + dest +
			" --no-same-owner --no-same-permissions; rm -f -- " + archive
	}
	return command, nil
}

func (s *Server) uploadTemporaryArchive(ctx context.Context, profile filetransferprofile.Profile, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		return s.withFTPClient(ctx, profile, nil, func(client *ftpclient.Client) error {
			return client.Store(ctx, remotePath, file)
		})
	case filetransferprofile.ProtocolSFTP:
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

func (s *Server) fileTransferArchiveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferArchiveUploadRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	rawRemoteDir := strings.TrimSpace(req.RemoteDir)
	if strings.ContainsAny(rawRemoteDir, "\x00\r\n") {
		http.Error(w, "remote directory must not contain NUL or line breaks", http.StatusBadRequest)
		return
	}
	req.RemoteDir = normalizeBackgroundRemotePath(rawRemoteDir)
	if req.ProfileID == "" || len(req.HostPaths) == 0 {
		http.Error(w, "profile_id and host_paths are required", http.StatusBadRequest)
		return
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	sources, err := s.collectProjectArchiveSources(req.HostPaths, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	roots := fileTransferArchiveRoots(sources)
	if _, err := validateFileTransferArchiveRoots(roots); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tmp, err := os.CreateTemp("", "taskdeck-folder-upload-*.tar.gz")
	if err != nil {
		http.Error(w, "cannot create temporary archive", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		http.Error(w, "cannot protect temporary archive", http.StatusInternalServerError)
		return
	}
	if err := writeLargeFileTransferTarGz(r.Context(), tmp, sources, nil); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) {
			status = http.StatusRequestTimeout
		}
		http.Error(w, "cannot create temporary compressed-upload archive: "+err.Error(), status)
		return
	}
	if err := tmp.Sync(); err != nil {
		http.Error(w, "cannot sync temporary archive", http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		http.Error(w, "cannot close temporary archive", http.StatusInternalServerError)
		return
	}
	info, err := os.Stat(tmpPath)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "temporary archive unavailable", http.StatusInternalServerError)
		return
	}
	if info.Size() > maxFileTransferBytes {
		http.Error(w, fmt.Sprintf("compressed archive exceeds %d bytes", maxFileTransferBytes), http.StatusRequestEntityTooLarge)
		return
	}
	id, err := newFileTransferBackgroundID("folder")
	if err != nil {
		http.Error(w, "cannot generate archive name", http.StatusInternalServerError)
		return
	}
	archiveName := ".taskdeck-" + id + ".tar.gz"
	remoteArchive := joinBackgroundRemotePath(req.RemoteDir, archiveName)

	release, ok := s.tryAcquireFileTransferBrowse(profile.ID)
	if !ok {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-TaskDeck-Transfer-Pool-Full", "1")
		http.Error(w, "FTP/SFTP connection pool is full", http.StatusTooManyRequests)
		return
	}
	defer release()
	if err := s.uploadTemporaryArchive(r.Context(), profile, tmpPath, remoteArchive); err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "folder_archive_upload", ProfileID: profile.ID, Success: false})
		http.Error(w, "archive upload failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	commands := fileTransferManualExtractCommands("tar.gz", remoteArchive, req.RemoteDir, roots)
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "folder_archive_upload", ProfileID: profile.ID, Success: true})
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "format": "tar.gz", "remote_archive": remoteArchive,
		"remote_destination": req.RemoteDir, "roots": roots, "archive_size": info.Size(),
		"manual_commands": commands,
	})
}

func (s *Server) fileTransferArchiveExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferArchiveExtractRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.RemoteArchive = strings.TrimSpace(req.RemoteArchive)
	req.RemoteDestination = normalizeBackgroundRemotePath(req.RemoteDestination)
	req.Format = strings.ToLower(strings.TrimSpace(req.Format))
	if req.Format == "" {
		req.Format = "tar.gz"
	}
	mergePolicy, err := normalizeFileTransferArchiveMergePolicy(req.MergePolicy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.MergePolicy = mergePolicy
	if req.ProfileID == "" || req.RemoteArchive == "" || req.RemoteDestination == "" {
		http.Error(w, "profile_id, remote_archive and remote_destination are required", http.StatusBadRequest)
		return
	}
	if strings.ContainsAny(req.RemoteArchive+req.RemoteDestination, "\x00\r\n") {
		http.Error(w, "remote paths must not contain NUL or line breaks", http.StatusBadRequest)
		return
	}
	roots, err := validateFileTransferArchiveRoots(req.Roots)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if profile.Protocol != filetransferprofile.ProtocolSFTP {
		http.Error(w, "automatic archive extraction requires SFTP linked to an SSH profile", http.StatusBadRequest)
		return
	}
	command, err := remoteArchiveExtractCommandWithPolicy(req.Format, req.RemoteArchive, req.RemoteDestination, roots, req.MergePolicy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	output, err := s.runSFTPLinkedSSHCommand(r.Context(), profile, command)
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "folder_archive_extract", ProfileID: profile.ID, Success: false})
		http.Error(w, "remote archive extraction failed; uploaded archive was kept: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "folder_archive_extract", ProfileID: profile.ID, Success: true})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "remote_archive": req.RemoteArchive,
		"remote_destination": req.RemoteDestination, "roots": roots, "output": output,
	})
}
