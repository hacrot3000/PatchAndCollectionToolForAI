package remoterelay

import "time"

// Once an Agent has passed the signed WebSocket handshake, it must still
// have a bounded CPU budget. Otherwise a single long-lived authenticated
// socket can send unlimited keepalives and force repeated lease signature
// checks without ever using another handshake or connection slot.
//
// This token bucket belongs to ONE connection, needs no global peer map,
// and is sized for sustained 2-second Agent polling plus the bounded
// E2EE diagnostic ACKs. It never grants additional application capabilities.
const (
	maxDeviceFrameBurst    = 12
	deviceFrameRefillEvery = 2 * time.Second
)

type deviceFrameBudget struct {
	tokens int
	last   time.Time
}

func newDeviceFrameBudget(now time.Time) deviceFrameBudget {
	return deviceFrameBudget{tokens: maxDeviceFrameBurst, last: now}
}

func (b *deviceFrameBudget) allow(now time.Time) bool {
	// A backwards clock jump must not refill the bucket. time.Now values
	// normally contain a monotonic component, so NTP corrections cannot
	// move this timestamp backwards in production.
	if now.Before(b.last) {
		return false
	}
	steps := now.Sub(b.last) / deviceFrameRefillEvery
	if steps > 0 {
		// Cap BEFORE converting steps to int or adding, including huge
		// forward jumps. Preserve fractional refill time.
		if steps >= maxDeviceFrameBurst {
			b.tokens = maxDeviceFrameBurst
		} else {
			b.tokens += int(steps)
			if b.tokens > maxDeviceFrameBurst {
				b.tokens = maxDeviceFrameBurst
			}
		}
		b.last = b.last.Add(steps * deviceFrameRefillEvery)
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}
