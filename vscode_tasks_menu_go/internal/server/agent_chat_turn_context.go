package server

import "context"

// agentServerContext returns the daemon lifecycle context. Provider turns are
// deliberately detached from HTTP request cancellation, but remain bounded by
// their own timeout and are cancelled when the daemon shuts down.
func (s *Server) agentServerContext() context.Context {
	if s == nil {
		return context.Background()
	}
	s.agentRunMu.Lock()
	ctx := s.agentRunCtx
	s.agentRunMu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
