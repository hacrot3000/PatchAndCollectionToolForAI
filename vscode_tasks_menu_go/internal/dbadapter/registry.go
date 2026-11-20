package dbadapter

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrAdapterNotFound = errors.New("database adapter not found")

type Registry struct {
	mu        sync.RWMutex
	manifests map[string]Manifest
}

func NewRegistry() *Registry {
	return &Registry{manifests: make(map[string]Manifest)}
}

func (r *Registry) Register(manifest Manifest) error {
	if r == nil {
		return errors.New("database adapter registry is nil")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Kind = strings.TrimSpace(manifest.Kind)
	manifest.Command = strings.TrimSpace(manifest.Command)
	manifest.Args = append([]string(nil), manifest.Args...)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.manifests[manifest.ID]; exists {
		return fmt.Errorf("database adapter %q is already registered", manifest.ID)
	}
	r.manifests[manifest.ID] = manifest
	return nil
}

func (r *Registry) Get(id string) (Manifest, error) {
	if r == nil {
		return Manifest{}, errors.New("database adapter registry is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Manifest{}, ErrAdapterNotFound
	}
	r.mu.RLock()
	manifest, ok := r.manifests[id]
	r.mu.RUnlock()
	if !ok {
		return Manifest{}, ErrAdapterNotFound
	}
	manifest.Args = append([]string(nil), manifest.Args...)
	return manifest, nil
}

func (r *Registry) List() []Manifest {
	if r == nil {
		return []Manifest{}
	}
	r.mu.RLock()
	out := make([]Manifest, 0, len(r.manifests))
	for _, manifest := range r.manifests {
		copyManifest := manifest
		copyManifest.Args = append([]string(nil), manifest.Args...)
		out = append(out, copyManifest)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}
