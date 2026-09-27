package sshprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	storeVersion      = 1
	maxStoreFileBytes = 1 << 20
	maxProfiles       = 128
)

var ErrProfileNotFound = errors.New("ssh profile not found")

type Store struct {
	path     string
	lockPath string
	mu       sync.Mutex
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
	path = filepath.Clean(path)
	return &Store{path: path, lockPath: path + ".lock"}, nil
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
	var out []Profile
	err := s.withExclusiveLock(func() error {
		var err error
		out, err = s.loadLocked()
		return err
	})
	return out, err
}

func (s *Store) Save(profiles []Profile) error {
	return s.withExclusiveLock(func() error {
		return s.saveLocked(profiles)
	})
}

func (s *Store) Create(profile Profile) (Profile, error) {
	var created Profile
	err := s.withExclusiveLock(func() error {
		profiles, err := s.loadLocked()
		if err != nil {
			return err
		}
		normalized, err := Normalize(profile)
		if err != nil {
			return err
		}
		for _, existing := range profiles {
			if existing.ID == normalized.ID {
				return fmt.Errorf("ssh profile id %q already exists", normalized.ID)
			}
		}
		profiles = append(profiles, normalized)
		if err := s.saveLocked(profiles); err != nil {
			return err
		}
		created = normalized
		return nil
	})
	return created, err
}

func (s *Store) Replace(id string, profile Profile) (Profile, error) {
	id = strings.TrimSpace(id)
	if err := validateID("profile id", id); err != nil {
		return Profile{}, err
	}
	var updated Profile
	err := s.withExclusiveLock(func() error {
		profiles, err := s.loadLocked()
		if err != nil {
			return err
		}
		profile.ID = id
		normalized, err := Normalize(profile)
		if err != nil {
			return err
		}
		for i := range profiles {
			if profiles[i].ID != id {
				continue
			}
			profiles[i] = normalized
			if err := s.saveLocked(profiles); err != nil {
				return err
			}
			updated = normalized
			return nil
		}
		return ErrProfileNotFound
	})
	return updated, err
}

func (s *Store) Delete(id string) (Profile, error) {
	id = strings.TrimSpace(id)
	if err := validateID("profile id", id); err != nil {
		return Profile{}, err
	}
	var deleted Profile
	err := s.withExclusiveLock(func() error {
		profiles, err := s.loadLocked()
		if err != nil {
			return err
		}
		for i := range profiles {
			if profiles[i].ID != id {
				continue
			}
			deleted = profiles[i]
			profiles = append(profiles[:i], profiles[i+1:]...)
			return s.saveLocked(profiles)
		}
		return ErrProfileNotFound
	})
	return deleted, err
}

func (s *Store) Get(id string) (Profile, error) {
	id = strings.TrimSpace(id)
	if err := validateID("profile id", id); err != nil {
		return Profile{}, err
	}
	profiles, err := s.Load()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return Profile{}, ErrProfileNotFound
}

func (s *Store) withExclusiveLock(fn func() error) error {
	if s == nil || s.path == "" || s.lockPath == "" {
		return errors.New("ssh profile store is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create ssh profile store dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect ssh profile store dir: %w", err)
	}
	lock, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open ssh profile store lock: %w", err)
	}
	defer lock.Close()
	if err := lock.Chmod(0o600); err != nil {
		return fmt.Errorf("protect ssh profile store lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock ssh profile store: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}

func (s *Store) loadLocked() ([]Profile, error) {
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

func (s *Store) saveLocked(profiles []Profile) error {
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
	return os.Chmod(s.path, 0o600)
}
