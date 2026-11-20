package agentchat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Capability values distinguish a provider declaration from a user-triggered
// probe. "unknown" means no probe has run; it is never treated as supported.
const (
	CapabilityUnknown      = "unknown"
	CapabilityVerified     = "verified"
	CapabilityUnsupported  = "unsupported"
	CapabilityInconclusive = "inconclusive"
)

const (
	ProbeText             = "text"
	ProbeStreaming        = "streaming"
	ProbeToolCalling      = "tool_calling"
	ProbePreviousResponse = "previous_response"
	ProbeConversation     = "conversation"
)

// CapabilityProbeVersion must be bumped whenever probe semantics change so
// stale results cannot be treated as current evidence.
const CapabilityProbeVersion = 1

// CapabilityRecord is server-side probe evidence for one provider/model
// endpoint. It intentionally contains no API key, prompt or response text.
type CapabilityRecord struct {
	ProviderID   string            `json:"provider_id"`
	ModelID      string            `json:"model_id"`
	Fingerprint  string            `json:"fingerprint"`
	Version      int               `json:"version"`
	Capabilities map[string]string `json:"capabilities"`
	Detail       map[string]string `json:"detail,omitempty"`
	ProbedAt     time.Time         `json:"probed_at"`
}

// capabilityFingerprint binds probe evidence to the exact endpoint, API type
// and model configuration. A configuration change invalidates prior results.
// CapabilityFingerprint is exported so server request building and probe
// storage derive the identical binding for a provider/model configuration.
func CapabilityFingerprint(p ProviderConfig, model ModelConfig) string {
	h := sha256.New()
	h.Write([]byte("v1\x00"))
	h.Write([]byte(capabilityBaseFields(p)))
	_, _ = h.Write([]byte{0})
	h.Write([]byte(model.ID + "\x00" + model.Name + "\x00"))
	for _, effort := range model.ReasoningEfforts {
		h.Write([]byte(effort + ","))
	}
	h.Write([]byte(model.DefaultReasoningEffort))
	if model.ToolCalling {
		h.Write([]byte("\x00declared-tool"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func capabilityBaseFields(p ProviderConfig) string {
	stream := "off"
	if p.Stream {
		stream = "on"
	}
	base := p.ID + "\x00" + p.APIType + "\x00" + strings.TrimRight(p.BaseURL, "/") + "\x00" + stream
	if len(p.TrustedSharedIPs) > 0 {
		base += "\x00trusted-shared-ips\x00" + strings.Join(p.TrustedSharedIPs, ",")
	}
	return base
}

// CapabilityStore keeps probe evidence in the same private directory as
// provider metadata, keyed by owner. Values are advisory: a missing or
// inconclusive record must not enable an execution tool.
type CapabilityStore struct{ dir string }

func NewCapabilityStore(dir string) (*CapabilityStore, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute capability directory required")
	}
	return &CapabilityStore{dir: filepath.Clean(dir)}, nil
}

func (s *CapabilityStore) file(owner string) (string, error) {
	if owner == "" || len(owner) > 256 {
		return "", errors.New("invalid owner")
	}
	h := sha256.Sum256([]byte(owner))
	return filepath.Join(s.dir, "capabilities-"+hex.EncodeToString(h[:16])+".json"), nil
}

func (s *CapabilityStore) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	fi, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe capability directory")
	}
	return nil
}

type capabilityFile struct {
	Version int                `json:"version"`
	Records []CapabilityRecord `json:"records"`
}

func (s *CapabilityStore) read(owner string) (capabilityFile, error) {
	path, err := s.file(owner)
	if err != nil {
		return capabilityFile{}, err
	}
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return capabilityFile{Version: 1}, nil
	}
	if err != nil {
		return capabilityFile{}, err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 || fi.Size() > 256<<10 {
		return capabilityFile{}, errors.New("unsafe capability file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return capabilityFile{}, err
	}
	var f capabilityFile
	if err = json.Unmarshal(raw, &f); err != nil {
		return capabilityFile{}, err
	}
	if f.Version != 1 || len(f.Records) > 96 {
		return capabilityFile{}, errors.New("invalid capability version/count")
	}
	return f, nil
}

// Get returns only a record whose fingerprint matches the current provider
// and model configuration. Any mismatch is reported as "no evidence".
func (s *CapabilityStore) Get(owner, providerID, modelID, fingerprint string) (CapabilityRecord, bool, error) {
	providerStoreMu.Lock()
	defer providerStoreMu.Unlock()
	if err := s.ensureDir(); err != nil {
		return CapabilityRecord{}, false, err
	}
	f, err := s.read(owner)
	if err != nil {
		return CapabilityRecord{}, false, err
	}
	for _, rec := range f.Records {
		if rec.ProviderID == providerID && rec.ModelID == modelID && rec.Fingerprint == fingerprint && rec.Version == CapabilityProbeVersion {
			return rec, true, nil
		}
	}
	return CapabilityRecord{}, false, nil
}

// Put replaces evidence for the same provider/model at the same fingerprint
// and discards records invalidated by a configuration change.
func (s *CapabilityStore) Put(owner string, rec CapabilityRecord) error {
	if rec.ProviderID == "" || rec.ModelID == "" || rec.Fingerprint == "" || rec.Version != CapabilityProbeVersion {
		return errors.New("invalid capability record")
	}
	for name, value := range rec.Capabilities {
		switch name {
		case ProbeText, ProbeStreaming, ProbeToolCalling, ProbePreviousResponse, ProbeConversation:
		default:
			return errors.New("unknown capability name")
		}
		switch value {
		case CapabilityUnknown, CapabilityVerified, CapabilityUnsupported, CapabilityInconclusive:
		default:
			return errors.New("invalid capability value")
		}
	}
	providerStoreMu.Lock()
	defer providerStoreMu.Unlock()
	if err := s.ensureDir(); err != nil {
		return err
	}
	f, err := s.read(owner)
	if err != nil {
		return err
	}
	kept := make([]CapabilityRecord, 0, len(f.Records))
	for _, existing := range f.Records {
		if existing.ProviderID == rec.ProviderID && existing.ModelID == rec.ModelID && existing.Fingerprint != rec.Fingerprint {
			continue
		}
		if existing.ProviderID == rec.ProviderID && existing.ModelID == rec.ModelID && existing.Fingerprint == rec.Fingerprint {
			continue
		}
		kept = append(kept, existing)
	}
	kept = append(kept, rec)
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].ProviderID != kept[j].ProviderID {
			return kept[i].ProviderID < kept[j].ProviderID
		}
		return kept[i].ModelID < kept[j].ModelID
	})
	f.Records = kept
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	path, err := s.file(owner)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".capability-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
