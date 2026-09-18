//go:build linux

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

var errPinnedProjectFileTooLarge = errors.New("project file exceeds pinned read limit")

type projectPinnedFile struct {
	parent   *os.File
	name     string
	temp     *os.File
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
	current, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*projectPinnedFile, error) {
		_ = current.Close()
		return nil, err
	}
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return fail(fmt.Errorf("invalid project path component"))
		}
		fd, openErr := syscall.Openat(
			int(current.Fd()),
			part,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
			0,
		)
		if openErr != nil {
			return fail(openErr)
		}
		next := os.NewFile(uintptr(fd), part)
		if next == nil {
			_ = syscall.Close(fd)
			return fail(fmt.Errorf("cannot pin project directory"))
		}
		info, statErr := next.Stat()
		if statErr != nil || !info.IsDir() {
			_ = next.Close()
			if statErr != nil {
				return fail(statErr)
			}
			return fail(fmt.Errorf("project path component is not a directory"))
		}
		_ = current.Close()
		current = next
	}
	return &projectPinnedFile{parent: current, name: parts[len(parts)-1]}, nil
}

func (p *projectPinnedFile) readCurrent(maxBytes int64) ([]byte, os.FileInfo, error) {
	if p == nil || p.parent == nil {
		return nil, nil, fmt.Errorf("project file handle is closed")
	}
	fd, err := syscall.Openat(
		int(p.parent.Fd()),
		p.name,
		syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), p.name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, nil, fmt.Errorf("cannot open project file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, info, fmt.Errorf("project path is not a regular file")
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, info, errPinnedProjectFileTooLarge
	}
	reader := io.Reader(file)
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, info, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, info, errPinnedProjectFileTooLarge
	}
	return data, info, nil
}

func (p *projectPinnedFile) writeTemp(data []byte, original os.FileInfo) error {
	if p == nil || p.parent == nil || original == nil {
		return fmt.Errorf("invalid pinned project file")
	}
	if p.temp != nil || p.tempName != "" {
		return fmt.Errorf("project temp file already exists")
	}
	var temp *os.File
	var tempName string
	for attempt := 0; attempt < 16; attempt++ {
		var token [10]byte
		if _, err := rand.Read(token[:]); err != nil {
			return err
		}
		tempName = ".task-menu-editor-" + hex.EncodeToString(token[:])
		fd, err := syscall.Openat(
			int(p.parent.Fd()),
			tempName,
			syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
			uint32(original.Mode().Perm()),
		)
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return err
		}
		temp = os.NewFile(uintptr(fd), tempName)
		if temp == nil {
			_ = syscall.Close(fd)
			_ = syscall.Unlinkat(int(p.parent.Fd()), tempName)
			return fmt.Errorf("cannot create project temp file")
		}
		break
	}
	if temp == nil {
		return fmt.Errorf("cannot allocate unique project temp file")
	}
	p.temp, p.tempName = temp, tempName
	cleanup := func() {
		_ = temp.Close()
		_ = syscall.Unlinkat(int(p.parent.Fd()), tempName)
		p.temp = nil
		p.tempName = ""
	}
	if stat, ok := original.Sys().(*syscall.Stat_t); ok {
		if err := temp.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			cleanup()
			return fmt.Errorf("cannot preserve file ownership: %w", err)
		}
	}
	if err := temp.Chmod(original.Mode().Perm()); err != nil {
		cleanup()
		return fmt.Errorf("cannot preserve file permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = syscall.Unlinkat(int(p.parent.Fd()), tempName)
		p.temp = nil
		p.tempName = ""
		return err
	}
	p.temp = nil
	return nil
}

func (p *projectPinnedFile) commitTemp() error {
	if p == nil || p.parent == nil || p.tempName == "" {
		return fmt.Errorf("project temp file is unavailable")
	}
	if err := syscall.Renameat(int(p.parent.Fd()), p.tempName, int(p.parent.Fd()), p.name); err != nil {
		return err
	}
	p.tempName = ""
	return p.parent.Sync()
}

func (p *projectPinnedFile) close() {
	if p == nil {
		return
	}
	if p.temp != nil {
		_ = p.temp.Close()
		p.temp = nil
	}
	if p.tempName != "" && p.parent != nil {
		_ = syscall.Unlinkat(int(p.parent.Fd()), p.tempName)
		p.tempName = ""
	}
	if p.parent != nil {
		_ = p.parent.Close()
		p.parent = nil
	}
}
