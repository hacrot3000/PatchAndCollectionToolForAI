//go:build darwin && go1.25

package server

import (
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "syscall"
)

// On macOS, os.Root (Go 1.25+) provides descriptor-relative access and
// renames. This prevents parent-directory symlink-swap escapes during edits.
var errPinnedProjectFileTooLarge = errors.New("project file exceeds pinned read limit")

type projectPinnedFile struct {
    parent *os.Root
    name string
    temp *os.File
    tempName string
}

func openProjectPinnedFile(root, canonicalPath string) (*projectPinnedFile, error) {
    rel, err := filepath.Rel(root, canonicalPath)
    if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
        return nil, fmt.Errorf("project file is outside workspace")
    }
    parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
    if len(parts) == 0 || parts[len(parts)-1] == "" || parts[len(parts)-1] == "." || parts[len(parts)-1] == ".." {
        return nil, fmt.Errorf("invalid project file")
    }
    current, err := os.OpenRoot(root)
    if err != nil { return nil, err }
    for _, part := range parts[:len(parts)-1] {
        if part == "" || part == "." || part == ".." {
            _ = current.Close()
            return nil, fmt.Errorf("invalid project path component")
        }
        next, openErr := current.OpenRoot(part)
        _ = current.Close()
        if openErr != nil { return nil, openErr }
        current = next
    }
    return &projectPinnedFile{parent:current, name:parts[len(parts)-1]}, nil
}

func (p *projectPinnedFile) openCurrent() (*os.File, error) {
    if p == nil || p.parent == nil { return nil, fmt.Errorf("project file handle is closed") }
    // Parent directories are confined by os.Root. Never follow a leaf symlink.
    return p.parent.OpenFile(p.name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

func (p *projectPinnedFile) readCurrent(maxBytes int64) ([]byte, os.FileInfo, error) {
    file, err := p.openCurrent()
    if err != nil { return nil, nil, err }
    defer file.Close()
    info, err := file.Stat()
    if err != nil { return nil, nil, err }
    if !info.Mode().IsRegular() { return nil, info, fmt.Errorf("project path is not a regular file") }
    if maxBytes > 0 && info.Size() > maxBytes { return nil, info, errPinnedProjectFileTooLarge }
    var reader io.Reader = file
    if maxBytes > 0 { reader = io.LimitReader(file, maxBytes+1) }
    data, err := io.ReadAll(reader)
    if err != nil { return nil, info, err }
    if maxBytes > 0 && int64(len(data)) > maxBytes { return nil, info, errPinnedProjectFileTooLarge }
    return data, info, nil
}

func (p *projectPinnedFile) readChunk(offset int64, limit int) ([]byte, os.FileInfo, error) {
    if offset < 0 || limit < 1 { return nil, nil, fmt.Errorf("invalid project file chunk range") }
    file, err := p.openCurrent()
    if err != nil { return nil, nil, err }
    defer file.Close()
    info, err := file.Stat()
    if err != nil { return nil, nil, err }
    if !info.Mode().IsRegular() { return nil, info, fmt.Errorf("project path is not a regular file") }
    if offset >= info.Size() { return []byte{}, info, nil }
    if int64(limit) > info.Size()-offset { limit = int(info.Size()-offset) }
    data := make([]byte, limit)
    n, err := file.ReadAt(data, offset)
    if err != nil && !errors.Is(err, io.EOF) { return nil, info, err }
    return data[:n], info, nil
}

func (p *projectPinnedFile) writeTemp(data []byte, original os.FileInfo) error {
    if p == nil || p.parent == nil || original == nil { return fmt.Errorf("invalid pinned project file") }
    if p.temp != nil || p.tempName != "" { return fmt.Errorf("project temp file already exists") }
    // Reject a replaced symlink before writing.
    source, err := p.openCurrent()
    if err != nil { return err }
    sourceInfo, statErr := source.Stat()
    _ = source.Close()
    if statErr != nil || !sourceInfo.Mode().IsRegular() { return fmt.Errorf("project source changed") }
    var tmp *os.File
    var name string
    for i := 0; i < 16; i++ {
        var token [10]byte
        if _, err = rand.Read(token[:]); err != nil { return err }
        name = ".task-menu-editor-" + hex.EncodeToString(token[:])
        tmp, err = p.parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, original.Mode().Perm())
        if errors.Is(err, os.ErrExist) { continue }
        if err != nil { return err }
        break
    }
    if tmp == nil { return fmt.Errorf("cannot allocate project temporary file") }
    p.temp, p.tempName = tmp, name
    cleanup := func() {
        _ = tmp.Close()
        _ = p.parent.Remove(name)
        p.temp, p.tempName = nil, ""
    }
    if owner, ok := original.Sys().(*syscall.Stat_t); ok {
        info, err := tmp.Stat()
        if err != nil { cleanup(); return err }
        if st, ok := info.Sys().(*syscall.Stat_t); ok && (st.Uid != owner.Uid || st.Gid != owner.Gid) {
            if err := tmp.Chown(int(owner.Uid), int(owner.Gid)); err != nil {
                cleanup()
                return fmt.Errorf("cannot preserve file ownership: %w", err)
            }
        }
    }
    if _, err := tmp.Write(data); err != nil { cleanup(); return err }
    if err := tmp.Chmod(original.Mode().Perm()); err != nil { cleanup(); return err }
    if err := tmp.Sync(); err != nil { cleanup(); return err }
    if err := tmp.Close(); err != nil {
        _ = p.parent.Remove(name)
        p.temp, p.tempName = nil, ""
        return err
    }
    p.temp = nil
    return nil
}

func (p *projectPinnedFile) commitTemp() error {
    if p == nil || p.parent == nil || p.tempName == "" { return fmt.Errorf("project temp file is unavailable") }
    if err := p.parent.Rename(p.tempName, p.name); err != nil { return err }
    p.tempName = ""
    dir, err := p.parent.Open(".")
    if err != nil { return err }
    defer dir.Close()
    return dir.Sync()
}

func (p *projectPinnedFile) close() {
    if p == nil { return }
    if p.temp != nil { _ = p.temp.Close(); p.temp = nil }
    if p.parent != nil {
        if p.tempName != "" { _ = p.parent.Remove(p.tempName); p.tempName = "" }
        _ = p.parent.Close()
        p.parent = nil
    }
}
