//go:build !linux && !darwin

package server

// Unsupported platforms retain in-process turn serialization only, matching
// the Agent journal's non-Unix lock fallback. Do not run multiple TaskDeck
// daemons over a shared Agent state directory on these platforms.
func acquireAgentTurnOSLock(workspace,id string)(func(),error){
 return func(){},nil
}
