//go:build !linux && !darwin

package agentchat

// POSIX directory-fsync semantics are not available on this platform.
// Native file Sync still protects bytes, but directory-entry crash durability
// is not claimed here. Cross-process / multi-host authority remains separate.
func syncAgentDirectory(_ string)error{return nil}
