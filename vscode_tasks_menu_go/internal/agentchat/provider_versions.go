package agentchat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const maxArchivedProviderVersions = 4096

type providerVersionRecord struct {
	Version  int            `json:"version"`
	Provider ProviderConfig `json:"provider"`
	ModelID  string         `json:"model_id"`
	ConfigID string         `json:"config_version"`
}

func (s *ProviderStore) versionDir(owner string) (string, error) {
	path, err := s.file(owner)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, filepath.Base(path[:len(path)-len(filepath.Ext(path))])+"-versions"), nil
}

// archiveProviderVersionsLocked stores public configuration snapshots only. It
// is called while providerStoreMu is held and never contains API credentials.
func (s *ProviderStore) archiveProviderVersionsLocked(owner string, provider ProviderConfig) error {
	dir, err := s.versionDir(owner)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe provider-version directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries)+len(provider.Models) > maxArchivedProviderVersions {
		return errors.New("provider-version history capacity reached")
	}
	for _, model := range provider.Models {
		version := RunProviderConfigVersion(provider, model)
		path := filepath.Join(dir, provider.ID+"-"+version+".json")
		if existing, err := os.Lstat(path); err == nil {
			if !existing.Mode().IsRegular() || existing.Mode()&os.ModeSymlink != 0 || existing.Mode().Perm()&0077 != 0 || existing.Size() > 128<<10 {
				return errors.New("unsafe existing provider-version snapshot")
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		record := providerVersionRecord{Version: 1, Provider: provider, ModelID: model.ID, ConfigID: version}
		body, err := json.Marshal(record)
		if err != nil || len(body) > 128<<10 {
			return errors.New("provider-version snapshot exceeds size limit")
		}
		tmp, err := os.CreateTemp(dir, ".provider-version-*.tmp")
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		if err = tmp.Chmod(0600); err == nil {
			_, err = tmp.Write(append(body, '\n'))
		}
		if err == nil {
			err = tmp.Sync()
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmpPath, path)
		}
		_ = os.Remove(tmpPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetVersion resolves a provider/model configuration as it existed when a run
// was submitted. Current provider removal still revokes access; edits to an
// existing provider leave the old public configuration available for retries.
func (s *ProviderStore) GetVersion(owner, providerID, modelID, version string) (ProviderConfig, ModelConfig, error) {
	providerStoreMu.Lock()
	defer providerStoreMu.Unlock()
	if err := s.ensureDir(); err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	current, err := s.read(owner)
	if err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	var stillConfigured bool
	for _, provider := range current.Providers {
		if provider.ID != providerID {
			continue
		}
		stillConfigured = true
		if model, ok := provider.FindModel(modelID); ok && RunProviderConfigVersion(provider, model) == version {
			return provider, model, nil
		}
	}
	if !stillConfigured || len(version) != 64 {
		return ProviderConfig{}, ModelConfig{}, ErrNotFound
	}
	dir, err := s.versionDir(owner)
	if err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode().Perm()&0077 != 0 {
		return ProviderConfig{}, ModelConfig{}, errors.New("unsafe provider-version directory")
	}
	path := filepath.Join(dir, providerID+"-"+version+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 128<<10 {
		return ProviderConfig{}, ModelConfig{}, errors.New("unsafe provider-version snapshot")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	var record providerVersionRecord
	if err := json.Unmarshal(body, &record); err != nil || record.Version != 1 || record.ConfigID != version || record.Provider.ID != providerID {
		return ProviderConfig{}, ModelConfig{}, errors.New("invalid provider-version snapshot")
	}
	if err := ValidateProvider(record.Provider); err != nil {
		return ProviderConfig{}, ModelConfig{}, err
	}
	model, ok := record.Provider.FindModel(modelID)
	if !ok || record.ModelID != modelID || RunProviderConfigVersion(record.Provider, model) != version {
		return ProviderConfig{}, ModelConfig{}, ErrNotFound
	}
	return record.Provider, model, nil
}
