package sshprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	storeVersion      = 1
	maxStoreFileBytes = 1 << 20
	maxProfiles       = 128
)

type Store struct {
	path string
}

type storedProfile struct {
	Profile
	SecretRef string `json:"secret_ref,omitempty"`
}

type storeFile struct {
	Version  int             `json:"version"`
	Profiles []storedProfile `json:"profiles"`
}

func DefaultStorePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	if strings.TrimSpace(base) == "" {
		return "", errors.New("user config dir is empty")
	}
	return filepath.Join(base, "taskdeck", "ssh_profiles.json"), nil
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("ssh profile store path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("ssh profile store path must be absolute")
	}
	return &Store{path: filepath.Clean(path)}, nil
}

func DefaultStore() (*Store, error) {
	path, err := DefaultStorePath()
	if err != nil {
		return nil, err
	}
	return NewStore(path)
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Load() ([]Profile, error) {
	if s == nil || s.path == "" {
		return nil, errors.New("ssh profile store is not initialized")
	}
	info, err := os.Stat(s.path)
	if os.IsNotExist(err) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat ssh profile store: %w", err)
	}
	if info.Size() > maxStoreFileBytes {
		return nil, fmt.Errorf("ssh profile store exceeds %d bytes", maxStoreFileBytes)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read ssh profile store: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []Profile{}, nil
	}

	var disk storeFile
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&disk); err != nil {
		return nil, fmt.Errorf("parse ssh profile store: %w", err)
	}
	if disk.Version != storeVersion {
		return nil, fmt.Errorf("unsupported ssh profile store version %d", disk.Version)
	}
	if len(disk.Profiles) > maxProfiles {
		return nil, fmt.Errorf("ssh profile store exceeds %d profiles", maxProfiles)
	}

	out := make([]Profile, 0, len(disk.Profiles))
	seen := make(map[string]bool, len(disk.Profiles))
	for i, stored := range disk.Profiles {
		p := stored.Profile
		p.SecretRef = stored.SecretRef
		normalized, err := Normalize(p)
		if err != nil {
			return nil, fmt.Errorf("ssh profile %d: %w", i+1, err)
		}
		if seen[normalized.ID] {
			return nil, fmt.Errorf("duplicate ssh profile id %q", normalized.ID)
		}
		seen[normalized.ID] = true
		out = append(out, normalized)
	}
	return out, nil
}

func (s *Store) Save(profiles []Profile) error {
	if s == nil || s.path == "" {
		return errors.New("ssh profile store is not initialized")
	}
	if len(profiles) > maxProfiles {
		return fmt.Errorf("ssh profiles exceed %d entries", maxProfiles)
	}

	disk := storeFile{
		Version:  storeVersion,
		Profiles: make([]storedProfile, 0, len(profiles)),
	}
	seen := make(map[string]bool, len(profiles))
	for i, profile := range profiles {
		normalized, err := Normalize(profile)
		if err != nil {
			return fmt.Errorf("ssh profile %d: %w", i+1, err)
		}
		if seen[normalized.ID] {
			return fmt.Errorf("duplicate ssh profile id %q", normalized.ID)
		}
		seen[normalized.ID] = true
		disk.Profiles = append(disk.Profiles, storedProfile{
			Profile:   normalized,
			SecretRef: normalized.SecretRef,
		})
	}

	data, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ssh profile store: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxStoreFileBytes {
		return fmt.Errorf("ssh profile store exceeds %d bytes", maxStoreFileBytes)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create ssh profile store dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect ssh profile store dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".ssh_profiles.*.tmp")
	if err != nil {
		return fmt.Errorf("create ssh profile temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect ssh profile temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write ssh profile temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync ssh profile temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close ssh profile temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace ssh profile store: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("protect ssh profile store: %w", err)
	}
	return nil
}
