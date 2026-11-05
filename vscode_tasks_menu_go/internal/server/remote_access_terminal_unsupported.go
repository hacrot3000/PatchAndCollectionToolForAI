//go:build !linux && !darwin

package server

import (
 "context"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Windows and other platforms intentionally have no Remote Terminal host
// implementation at this stage. Never fall back to a generic shell proxy.
func (s *Server) startRemoteTerminal(context.Context,*remoteTaskDeckSession)error{
 return remotetransport.ErrRemoteActionDenied
}
func (session *remoteTaskDeckSession) pollRemoteTerminal(context.Context)(remotetransport.RemoteTerminalProgress,error){
 return remotetransport.RemoteTerminalProgress{},remotetransport.ErrRemoteActionDenied
}
func (s *Server) writeRemoteTerminal(context.Context,*remoteTaskDeckSession,string)(remotetransport.RemoteTerminalProgress,error){
 return remotetransport.RemoteTerminalProgress{},remotetransport.ErrRemoteActionDenied
}
func (s *Server) resizeRemoteTerminal(context.Context,*remoteTaskDeckSession,int,int)(remotetransport.RemoteTerminalProgress,error){
 return remotetransport.RemoteTerminalProgress{},remotetransport.ErrRemoteActionDenied
}
func (s *Server) closeRemoteTerminal(context.Context,*remoteTaskDeckSession)(remotetransport.RemoteTerminalProgress,error){
 return remotetransport.RemoteTerminalProgress{},remotetransport.ErrRemoteActionDenied
}
func (session *remoteTaskDeckSession) stopRemoteTerminal(){}
