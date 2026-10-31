package tasks

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrInvalidRemoteTaskApproval = errors.New("task definition is not eligible for a remote approval digest")

// RemoteApprovalToken binds the host's independent approval to the complete
// canonical task definition, not the public 31-bit menu selector. A malicious
// tasks.json edit must never preserve authorization by finding an ID collision.
// Only primary-root tasks with no TaskDeck secret mapping can be approved.
func RemoteApprovalToken(task Task) (string, error) {
	if task.ID <= 0 || task.ID > 0x7fffffff || task.Raw == nil || len(task.SecretEnv) != 0 {
		return "", ErrInvalidRemoteTaskApproval
	}
	canonical, err := json.Marshal(task.Raw)
	if err != nil {
		return "", ErrInvalidRemoteTaskApproval
	}
	digest := sha256.Sum256(canonical)
	id := int(binary.BigEndian.Uint32(digest[:4]) & 0x7fffffff)
	if id == 0 {
		id = 1
	}
	if id != task.ID {
		return "", ErrInvalidRemoteTaskApproval
	}
	return strconv.Itoa(task.ID) + "@" + hex.EncodeToString(digest[:]), nil
}

// RemoteExecutableAllowed refuses PATH lookup, workspace-controlled
// executables and binaries writable by other OS users. The hash-based
// task approval covers tasks.json, NOT an executable that might be swapped
// via the project's remote file/editor APIs. Check both the lexical path
// (including a mutable symlink INSIDE the workspace) and the resolved path
// (including a symlink OUTSIDE the workspace pointing back into it).
// This is defense in depth: the host must still audit executable contents
// and any scripts/data files referenced by approved arguments.
func RemoteExecutableAllowed(workspace, command string) bool {
	if workspace == "" || !filepath.IsAbs(command) {
		return false
	}
	realWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return false
	}
	realCommand, err := filepath.EvalSymlinks(command)
	if err != nil {
		return false
	}
	if pathWithinRemoteWorkspace(workspace, command) ||
		pathWithinRemoteWorkspace(realWorkspace, realCommand) {
		return false
	}
	info, err := os.Stat(realCommand)
	if err != nil || !info.Mode().IsRegular() ||
		info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return false
	}
	return true
}

func pathWithinRemoteWorkspace(workspace, path string) bool {
	relative, err := filepath.Rel(workspace, path)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
