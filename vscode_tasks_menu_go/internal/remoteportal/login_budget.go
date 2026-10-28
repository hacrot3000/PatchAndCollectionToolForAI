package remoteportal

import (
 "sync"
 "time"
)

// Agent login-only diagnostics are expensive because the real TaskDeck
// password verifier uses scrypt. Tickets may be issued repeatedly for
// different usernames, so account-scoped throttles alone do not cap CPU.
//
// Enforce a single device-wide token bucket before invoking TaskDeck auth.
// Burst: 5 attempts. Refill: 1 every 12 seconds (5/minute sustained).
// Both failed and successful login probes consume a token; no IP address,
// username or secret is retained, and memory use is constant.
const agentLoginProbeBurst = 5
const agentLoginProbeRefill = 12*time.Second

type agentLoginBudget struct {
 mu sync.Mutex
 initialized bool
 tokens int
 last time.Time
}

func (b *agentLoginBudget) allow(now time.Time) bool {
 b.mu.Lock()
 defer b.mu.Unlock()
 if !b.initialized {
  b.initialized=true
  b.tokens=agentLoginProbeBurst
  b.last=now
 }
 elapsed:=now.Sub(b.last)
 if elapsed>=agentLoginProbeRefill {
  refill:=int(elapsed/agentLoginProbeRefill)
  b.tokens+=refill
  if b.tokens>=agentLoginProbeBurst {
   b.tokens=agentLoginProbeBurst
   b.last=now
  } else {
   b.last=b.last.Add(time.Duration(refill)*agentLoginProbeRefill)
  }
 }
 if b.tokens<=0 {return false}
 b.tokens--
 return true
}
