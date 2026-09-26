package secretstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	storeVersion       = 1
	masterKeyBytes     = 32
	maxSecretIDBytes   = 256
	maxSecretBytes     = 64 << 10
	maxSecretRecords   = 512
	maxSecretFileBytes = 2 << 20
)

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Put(id string, secret []byte) error
	Get(id string) ([]byte, error)
	Delete(id string) error
}

type FileStore struct {
	dir      string
	keyPath  string
	dataPath string
	lockPath string
	mu       sync.Mutex
}

type encryptedRecord struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type secretFile struct {
	Version int                        `json:"version"`
	Records map[string]encryptedRecord `json:"records"`
}

func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	if strings.TrimSpace(base) == "" {
		return "", errors.New("user config dir is empty")
	}
	return filepath.Join(base, "taskdeck"), nil
}

func NewDefaultFileStore() (*FileStore, error) {
	dir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return NewFileStore(dir)
}

func NewFileStore(dir string) (*FileStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("secret store directory is required")
	}
	if !filepath.IsAbs(dir) {
		return nil, errors.New("secret store directory must be absolute")
	}
	dir = filepath.Clean(dir)
	return &FileStore{
		dir:      dir,
		keyPath:  filepath.Join(dir, "master.key"),
		dataPath: filepath.Join(dir, "secrets.json"),
		lockPath: filepath.Join(dir, "secrets.lock"),
	}, nil
}

func (s *FileStore) Put(id string, secret []byte) error {
	if err := validateSecretID(id); err != nil {
		return err
	}
	if len(secret) == 0 {
		return errors.New("secret value is required")
	}
	if len(secret) > maxSecretBytes {
		return fmt.Errorf("secret exceeds %d bytes", maxSecretBytes)
	}

	return s.withExclusiveLock(func() error {
		key, err := s.loadOrCreateKeyLocked()
		if err != nil {
			return err
		}
		data, err := s.loadFileLocked()
		if err != nil {
			return err
		}
		if _, exists := data.Records[id]; !exists && len(data.Records) >= maxSecretRecords {
			return fmt.Errorf("secret store exceeds %d records", maxSecretRecords)
		}
		record, err := encrypt(key, id, secret)
		if err != nil {
			return err
		}
		data.Records[id] = record
		return s.writeFileLocked(data)
	})
}

func (s *FileStore) Get(id string) ([]byte, error) {
	if err := validateSecretID(id); err != nil {
		return nil, err
	}
	var out []byte
	err := s.withExclusiveLock(func() error {
		key, err := s.loadOrCreateKeyLocked()
		if err != nil {
			return err
		}
		data, err := s.loadFileLocked()
		if err != nil {
			return err
		}
		record, ok := data.Records[id]
		if !ok {
			return ErrNotFound
		}
		out, err = decrypt(key, id, record)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *FileStore) Delete(id string) error {
	if err := validateSecretID(id); err != nil {
		return err
	}
	return s.withExclusiveLock(func() error {
		data, err := s.loadFileLocked()
		if err != nil {
			return err
		}
		if _, ok := data.Records[id]; !ok {
			return nil
		}
		delete(data.Records, id)
		return s.writeFileLocked(data)
	})
}

func (s *FileStore) withExclusiveLock(fn func() error) error {
	if s == nil || s.dir == "" {
		return errors.New("secret store is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create secret store directory: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("protect secret store directory: %w", err)
	}
	lock, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open secret store lock: %w", err)
	}
	defer lock.Close()
	if err := lock.Chmod(0o600); err != nil {
		return fmt.Errorf("protect secret store lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock secret store: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}

func (s *FileStore) loadOrCreateKeyLocked() ([]byte, error) {
	key, err := os.ReadFile(s.keyPath)
	if err == nil {
		if len(key) != masterKeyBytes {
			return nil, errors.New("secret store master key has invalid length")
		}
		if err := os.Chmod(s.keyPath, 0o600); err != nil {
			return nil, fmt.Errorf("protect secret store master key: %w", err)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read secret store master key: %w", err)
	}

	key = make([]byte, masterKeyBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate secret store master key: %w", err)
	}
	f, err := os.OpenFile(s.keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create secret store master key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		_ = os.Remove(s.keyPath)
		return nil, fmt.Errorf("write secret store master key: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(s.keyPath)
		return nil, fmt.Errorf("sync secret store master key: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(s.keyPath)
		return nil, fmt.Errorf("close secret store master key: %w", err)
	}
	return key, nil
}

func (s *FileStore) loadFileLocked() (secretFile, error) {
	out := secretFile{Version: storeVersion, Records: map[string]encryptedRecord{}}
	info, err := os.Stat(s.dataPath)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return secretFile{}, fmt.Errorf("stat secret store: %w", err)
	}
	if info.Size() > maxSecretFileBytes {
		return secretFile{}, fmt.Errorf("secret store exceeds %d bytes", maxSecretFileBytes)
	}
	data, err := os.ReadFile(s.dataPath)
	if err != nil {
		return secretFile{}, fmt.Errorf("read secret store: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return out, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return secretFile{}, fmt.Errorf("parse secret store: %w", err)
	}
	if out.Version != storeVersion {
		return secretFile{}, fmt.Errorf("unsupported secret store version %d", out.Version)
	}
	if out.Records == nil {
		out.Records = map[string]encryptedRecord{}
	}
	if len(out.Records) > maxSecretRecords {
		return secretFile{}, fmt.Errorf("secret store exceeds %d records", maxSecretRecords)
	}
	return out, nil
}

func (s *FileStore) writeFileLocked(data secretFile) error {
	data.Version = storeVersion
	if data.Records == nil {
		data.Records = map[string]encryptedRecord{}
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode secret store: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxSecretFileBytes {
		return fmt.Errorf("secret store exceeds %d bytes", maxSecretFileBytes)
	}
	tmp, err := os.CreateTemp(s.dir, ".secrets.*.tmp")
	if err != nil {
		return fmt.Errorf("create secret store temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect secret store temp file: %w", err)
	}
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write secret store temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync secret store temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close secret store temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.dataPath); err != nil {
		return fmt.Errorf("replace secret store: %w", err)
	}
	return os.Chmod(s.dataPath, 0o600)
}

func encrypt(key []byte, id string, plaintext []byte) (encryptedRecord, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedRecord{}, fmt.Errorf("initialize secret cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedRecord{}, fmt.Errorf("initialize secret AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedRecord{}, fmt.Errorf("generate secret nonce: %w", err)
	}
	aad := []byte("taskdeck-secret-v1:" + id)
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	return encryptedRecord{
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decrypt(key []byte, id string, record encryptedRecord) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize secret cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize secret AEAD: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(record.Nonce)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errors.New("stored secret nonce is invalid")
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return nil, errors.New("stored secret ciphertext is invalid")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte("taskdeck-secret-v1:"+id))
	if err != nil {
		return nil, errors.New("stored secret authentication failed")
	}
	return plaintext, nil
}

func validateSecretID(id string) error {
	if id == "" {
		return errors.New("secret id is required")
	}
	if len(id) > maxSecretIDBytes {
		return fmt.Errorf("secret id exceeds %d bytes", maxSecretIDBytes)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == ':' || r == '/' {
			continue
		}
		return fmt.Errorf("secret id contains unsupported character %q", r)
	}
	return nil
}
