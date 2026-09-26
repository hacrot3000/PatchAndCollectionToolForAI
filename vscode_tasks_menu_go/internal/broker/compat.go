package broker

import (
	"time"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

// CompatibilityService preserves sessions owned by an older long-lived broker
// while routing only executions that need newer Patch protocol capabilities to
// a daemon-local Manager. This avoids killing user terminals during an update.
type CompatibilityService struct {
	primary  *Client
	fallback *session.Manager
}

var _ session.Service = (*CompatibilityService)(nil)
var _ session.ProtocolStateProvider = (*CompatibilityService)(nil)
var _ session.ProtocolCommandWriter = (*CompatibilityService)(nil)

func NewCompatibilityService(primary *Client) *CompatibilityService {
	return &CompatibilityService{
		primary:  primary,
		fallback: session.NewManager(4 << 20),
	}
}

func (s *CompatibilityService) NeedsPatchProtocolFallback() bool {
	if s == nil || s.primary == nil {
		return true
	}
	return !s.primary.SupportsPatchProtocolEvents() || !s.primary.SupportsPatchProtocolCommands()
}

func (s *CompatibilityService) SupportsSessionOwnership() bool {
	return s != nil && s.primary != nil && s.primary.SupportsSessionOwnership()
}

func (s *CompatibilityService) useFallback(spec tasks.Execution) bool {
	if s == nil || s.primary == nil || !spec.ProtocolEvents {
		return s == nil || s.primary == nil
	}
	if !s.primary.SupportsPatchProtocolEvents() {
		return true
	}
	return spec.ProtocolCommands && !s.primary.SupportsPatchProtocolCommands()
}

func (s *CompatibilityService) local(id string) bool {
	if s == nil || s.fallback == nil {
		return false
	}
	_, ok := s.fallback.Metadata(id)
	return ok
}

func (s *CompatibilityService) Close() {
	if s != nil && s.fallback != nil {
		s.fallback.Shutdown(2 * time.Second)
	}
}

func (s *CompatibilityService) List() []session.Metadata {
	out := append([]session.Metadata(nil), s.primary.List()...)
	return append(out, s.fallback.List()...)
}

func (s *CompatibilityService) Start(spec tasks.Execution) (session.Metadata, error) {
	if s.useFallback(spec) {
		return s.fallback.Start(spec)
	}
	return s.primary.Start(spec)
}

func (s *CompatibilityService) Metadata(id string) (session.Metadata, bool) {
	if s.local(id) {
		return s.fallback.Metadata(id)
	}
	return s.primary.Metadata(id)
}

func (s *CompatibilityService) SetTitle(id, title string) (session.Metadata, error) {
	if s.local(id) {
		return s.fallback.SetTitle(id, title)
	}
	return s.primary.SetTitle(id, title)
}

func (s *CompatibilityService) Input(id string, data []byte) error {
	if s.local(id) {
		return s.fallback.Input(id, data)
	}
	return s.primary.Input(id, data)
}

func (s *CompatibilityService) Resize(id string, rows, cols uint16) error {
	if s.local(id) {
		return s.fallback.Resize(id, rows, cols)
	}
	return s.primary.Resize(id, rows, cols)
}

func (s *CompatibilityService) Stop(id string) error {
	if s.local(id) {
		return s.fallback.Stop(id)
	}
	return s.primary.Stop(id)
}

func (s *CompatibilityService) Kill(id string) error {
	if s.local(id) {
		return s.fallback.Kill(id)
	}
	return s.primary.Kill(id)
}

func (s *CompatibilityService) Clear(id string) error {
	if s.local(id) {
		return s.fallback.Clear(id)
	}
	return s.primary.Clear(id)
}

func (s *CompatibilityService) Remove(id string) error {
	if s.local(id) {
		return s.fallback.Remove(id)
	}
	return s.primary.Remove(id)
}

func (s *CompatibilityService) Subscribe(id string) ([]byte, <-chan []byte, func(), error) {
	if s.local(id) {
		return s.fallback.Subscribe(id)
	}
	return s.primary.Subscribe(id)
}

func (s *CompatibilityService) CurrentCwd(id string) (string, error) {
	if s.local(id) {
		return s.fallback.CurrentCwd(id)
	}
	return s.primary.CurrentCwd(id)
}

func (s *CompatibilityService) ProtocolState(id string) (session.ProtocolState, error) {
	if s.local(id) {
		return s.fallback.ProtocolState(id)
	}
	return s.primary.ProtocolState(id)
}

func (s *CompatibilityService) ProtocolCommand(id string, data []byte) error {
	if s.local(id) {
		return s.fallback.ProtocolCommand(id, data)
	}
	return s.primary.ProtocolCommand(id, data)
}
