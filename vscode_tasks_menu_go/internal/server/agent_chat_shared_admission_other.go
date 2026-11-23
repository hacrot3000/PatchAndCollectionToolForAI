//go:build !linux && !darwin

package server

import "context"

// On platforms without TaskDeck's flock implementation, keep the original
// single-daemon-only policy; a cross-daemon quota is NOT claimed.
func acquireAgentCrossProcessCapacity(ctx context.Context,owner string)(func(),bool){
 if ctx==nil||ctx.Err()!=nil||owner==""{return nil,false}
 return func(){},true
}
