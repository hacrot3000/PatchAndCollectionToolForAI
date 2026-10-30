package server

import (
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// remoteWorkspaceAgentMatches prevents cross-workspace confused-deputy
// authorization. The Portal and Relay can be completely malicious: neither
// can provide a replacement for the owner-only Agent identity on this host.
//
// Fail closed if the identity has been removed. Never silently generate a
// fresh private key during login authorization, and never use a Device ID
// from PHP, a ticket or an untrusted browser message as the trust anchor.
func (s *Server) remoteWorkspaceAgentMatches(
 agent *remoteportal.Agent, channel *remotetransport.Channel,
) bool {
 if s==nil || agent==nil || channel==nil || !channel.Established() ||
  s.Workspace=="" || s.Config.RemotePortalURL=="" {
  return false
 }
 if agent.PortalOrigin()!=strings.TrimSuffix(s.Config.RemotePortalURL,"/") {
  return false
 }
 path,err:=remoteportal.DefaultIdentityPath(s.Workspace)
 if err!=nil{return false}
 trusted,err:=remoteportal.LoadExistingIdentity(path)
 if err!=nil{return false}
 deviceID:=trusted.DeviceID()
 return agent.DeviceID()==deviceID && channel.BoundDeviceID()==deviceID
}
