// Package remoterelay validates authorization for the future outbound relay.
// Validation is deliberately independent from the TaskDeck HTTP auth stack:
// a signed connection ticket authorizes rendezvous ONLY, never execution.
package remoterelay

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"regexp"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const ticketPrefix = "taskdeck-relay-ticket-v1."
const maxTicketBytes = 4096
const maxUnexpiredTickets = 20000

var (
	deviceIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	jtiPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	userIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
)

// Claims are emitted and signed by the Portal, not by a browser or Relay.
// A role here is the maximum Portal allowance; TaskDeck may only narrow it.
type Claims struct {
	BrowserBinding string `json:"browser_binding"`
	AuthEpoch int64 `json:"auth_epoch,omitempty"`
	V        int    `json:"v"`
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Subject  string `json:"sub"`
	DeviceID string `json:"device_id"`
	Role     string `json:"role"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	JTI      string `json:"jti"`
}

type Verifier struct {
	key    ed25519.PublicKey
	issuer string
	mu     sync.Mutex
	used   map[string]int64
	ledger *ReplayLedger
}

// ConfigureReplayLedger opts into crash-resistant ticket consumption. The
// production Relay executable must configure this before accepting traffic.
func (v *Verifier) ConfigureReplayLedger(ledger *ReplayLedger) error {
	if v == nil || ledger == nil { return errors.New("persistent ticket ledger is required") }
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.used) != 0 || v.ledger != nil {
		return errors.New("cannot replace replay backend after tickets are consumed")
	}
	v.ledger = ledger
	return nil
}

func NewVerifier(publicKeyBase64, expectedIssuer string) (*Verifier, error) {
	key, err := base64.StdEncoding.Strict().DecodeString(publicKeyBase64)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Portal Ed25519 verification key")
	}
	if !validIssuer(expectedIssuer) {
		return nil, errors.New("invalid expected Portal issuer")
	}
	return &Verifier{key: ed25519.PublicKey(append([]byte(nil), key...)),
		issuer: expectedIssuer, used: make(map[string]int64)}, nil
}

// Consume authorizes a single Portal-granted rendezvous ticket. A repeated
// token/JTI is rejected, including under concurrent browser connects.
// Caller still must verify device presence, agent proof, TaskDeck login,
// per-action scopes and revocation independently.
func validIssuer(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" &&
		u.Hostname() != "" && u.User == nil && u.RawQuery == "" &&
		u.Fragment == "" && u.Path == "" && u.Opaque == "" &&
		raw == u.Scheme + "://" + u.Host
}

func (v *Verifier) Consume(token string, now time.Time) (Claims, error) {
	var c Claims
	if len(token) < 100 || len(token) > maxTicketBytes ||
		strings.Count(token, ".") != 1 {
		return c, errors.New("malformed ticket")
	}
	parts := strings.SplitN(token, ".", 2)
	body, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(body) > 2048 || len(body) == 0 {
		return c, errors.New("invalid ticket payload")
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize ||
		!ed25519.Verify(v.key, []byte(ticketPrefix+parts[0]), sig) {
		return c, errors.New("invalid Portal ticket signature")
	}
	reader := json.NewDecoder(bytes.NewReader(body))
	reader.DisallowUnknownFields()
	if err := reader.Decode(&c); err != nil {
		return Claims{}, errors.New("invalid ticket claims")
	}
	var trailing json.RawMessage
	if err := reader.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Claims{}, errors.New("trailing ticket payload")
	}
	sec := now.Unix()
	if c.V != 2 || c.AuthEpoch<0 || !deviceIDPattern.MatchString(c.BrowserBinding) ||
		c.Issuer != v.issuer || c.Audience != "taskdeck-relay" ||
		!userIDPattern.MatchString(c.Subject) ||
		!deviceIDPattern.MatchString(c.DeviceID) || !jtiPattern.MatchString(c.JTI) ||
		(c.Role != "viewer" && c.Role != "operator" && c.Role != "admin") ||
		c.IssuedAt > sec+5 || c.IssuedAt < sec-40 ||
		c.Expires <= sec || c.Expires > c.IssuedAt+30 || c.Expires <= c.IssuedAt {
		return Claims{}, errors.New("expired or unauthorized ticket claims")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.ledger != nil {
		if err := v.ledger.Consume(c.JTI, c.Expires, now); err != nil {
			return Claims{}, err
		}
		return c, nil
	}
	for jti, exp := range v.used {
		if exp <= sec {
			delete(v.used, jti)
		}
	}
	if _, duplicate := v.used[c.JTI]; duplicate {
		return Claims{}, errors.New("ticket already consumed")
	}
	if len(v.used) >= maxUnexpiredTickets {
		return Claims{}, errors.New("relay replay tracker at capacity")
	}
	v.used[c.JTI] = c.Expires
	return c, nil
}

// VerifyBrowserBinding rejects a Relay-side key/nonce substitution after
// Portal signed the browser's ECDH public key and client challenge.
func VerifyBrowserBinding(c Claims,nonce,encodedPublic string) error {
 public,err:=base64.StdEncoding.Strict().DecodeString(encodedPublic)
 if err!=nil{return errors.New("invalid browser public key")}
 binding,err:=remotetransport.BrowserTicketBinding(nonce,public)
 if err!=nil || c.V!=2 || binding!=c.BrowserBinding {
  return errors.New("Portal ticket does not bind this browser E2EE session")
 }
 return nil
}

func (v *Verifier) ActiveReplayRecords() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.used)
}

// VerifyDeviceProof checks a fresh device proof for a specific Relay origin.
// The public key fingerprint IS the Device ID: no private key or endpoints
// are stored by the Portal. The proof is not a substitute for a paired grant.
type DeviceProof struct {
	DeviceID  string `json:"device_id"`
	PublicKey string `json:"public_key"`
	Audience  string `json:"aud"`
	Challenge string `json:"challenge"`
	DeviceLease string `json:"device_lease"`
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

const DeviceChallengeProtocol = "taskdeck-relay-device-v2"
const DeviceChallengeSize = 32

func VerifyDeviceProof(p DeviceProof, expectedAudience string, now time.Time) error {
	challenge, err := base64.RawURLEncoding.Strict().DecodeString(p.Challenge)
	if err != nil || len(challenge) != DeviceChallengeSize {
		return errors.New("invalid relay challenge")
	}
	if !deviceIDPattern.MatchString(p.DeviceID) || p.Audience != expectedAudience ||
		!jtiPattern.MatchString(p.Nonce) || p.Timestamp < now.Unix()-30 ||
		p.Timestamp > now.Unix()+5 {
		return errors.New("invalid relay device identity envelope")
	}
	pub, err := base64.StdEncoding.Strict().DecodeString(p.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid device public key")
	}
	digest := sha256.Sum256(pub)
	if hex.EncodeToString(digest[:]) != p.DeviceID {
		return errors.New("device fingerprint mismatch")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(p.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid device signature encoding")
	}
	msg := strings.Join([]string{
		DeviceChallengeProtocol, "connect", p.DeviceID, p.Audience,
		strconv.FormatInt(p.Timestamp, 10), p.Nonce, p.Challenge,
	}, string([]byte{10}))
	if !ed25519.Verify(pub, []byte(msg), sig) {
		return errors.New("invalid device connect signature")
	}
	return nil
}
