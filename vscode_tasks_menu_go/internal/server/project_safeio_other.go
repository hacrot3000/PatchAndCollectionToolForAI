//go:build !linux

package server

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errPinnedProjectFileTooLarge = errors.New("project file exceeds pinned read limit")

type projectPinnedFile struct {
	path     string
	temp     *os.File
	tempName string
}

func openProjectPinnedFile(root, canonicalPath string) (*projectPinnedFile, error) {
	resolved, err := filepath.EvalSymlinks(canonicalPath)
	if err != nil || !pathWithin(root, resolved) {
		return nil, fmt.Errorf("project file is outside workspace")
	}
	return &projectPinnedFile{path: resolved}, nil
}

func (p *projectPinnedFile) readCurrent(maxBytes int64) ([]byte, os.FileInfo, error) {
	file, err := os.Open(p.path)
	if err != nil {
		return nil, nil, err
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
	tmp, err := os.CreateTemp(filepath.Dir(p.path), ".task-menu-editor-*")
	if err != nil {
		return err
	}
	p.temp, p.tempName = tmp, tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		p.temp = nil
		p.tempName = ""
	}
	if err := tmp.Chmod(original.Mode().Perm()); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		p.temp = nil
		p.tempName = ""
		return err
	}
	p.temp = nil
	return nil
}

func (p *projectPinnedFile) commitTemp() error {
	if p.tempName == "" {
		return fmt.Errorf("project temp file is unavailable")
	}
	if err := os.Rename(p.tempName, p.path); err != nil {
		return err
	}
	p.tempName = ""
	if dir, err := os.Open(filepath.Dir(p.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (p *projectPinnedFile) close() {
	if p == nil {
		return
	}
	if p.temp != nil {
		_ = p.temp.Close()
		p.temp = nil
	}
	if p.tempName != "" {
		_ = os.Remove(p.tempName)
		p.tempName = ""
	}
}
