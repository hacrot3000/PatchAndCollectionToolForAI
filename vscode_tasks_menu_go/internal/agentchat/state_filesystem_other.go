//go:build !linux && !darwin

package agentchat

// No portable statfs classifier is available on this platform. The existing
// lockfile_other.go also does not provide cross-process flock. Neither
// multi-daemon nor shared network-filesystem safety is promised here.
func validateAgentStateFilesystem(_ string)error{return nil}
