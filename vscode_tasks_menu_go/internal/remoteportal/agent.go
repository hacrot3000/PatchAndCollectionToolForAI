// Package remoteportal implements the OUTBOUND identity/presence protocol.
// It intentionally does not expose the TaskDeck HTTP/WS handlers or grant remote
// access. Remote execution must wait for a separately audited auth/RBAC adapter.
package remoteportal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const protocol = "taskdeck-remote-v1"

type Identity struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

// LoadOrCreateIdentity stores only the 32-byte seed, with OS-owner file
// permissions. keyPath must be private, absolute and outside a shared project.
// Future Settings integration is responsible for selecting the config path.
func LoadOrCreateIdentity(keyPath string) (*Identity, error) {
	if !filepath.IsAbs(keyPath) {
		return nil, errors.New("identity path must be absolute")
	}
	parent := filepath.Dir(keyPath)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	// Refuse a symlinked, world-readable or group-writable identity
	// directory. The private key may never live in a shared project tree.
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("remote identity directory must be private, non-symlink, and owner-only")
	}
	seed, err := readSeed(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		seed = make([]byte, ed25519.SeedSize)
		if _, err := io.ReadFull(rand.Reader, seed); err != nil {
			return nil, err
		}
		f, createErr := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(createErr, os.ErrExist) {
			seed, err = readSeed(keyPath)
			if err != nil {
				return nil, err
			}
		} else if createErr != nil {
			return nil, createErr
		} else {
			_, err = f.Write(seed)
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(keyPath)
				return nil, err
			}
		}
	} else if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New("invalid Ed25519 identity seed length")
	}
	private := ed25519.NewKeyFromSeed(seed)
	return &Identity{private: private, public: private.Public().(ed25519.PublicKey)}, nil
}

func readSeed(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("identity file must be a regular owner-only file")
	}
	return os.ReadFile(path)
}

func (i *Identity) DeviceID() string {
	sum := sha256.Sum256(i.public)
	return hex.EncodeToString(sum[:])
}

type signedMessage struct {
	DeviceID    string `json:"device_id"`
	PublicKey   string `json:"public_key"`
	DisplayName string `json:"display_name,omitempty"`
	PairingHash string `json:"pairing_hash,omitempty"`
	Timestamp   int64  `json:"timestamp"`
	Nonce       string `json:"nonce"`
	Signature   string `json:"signature"`
}

func (i *Identity) sign(action string, fields ...string) (signedMessage, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return signedMessage{}, err
	}
	now := time.Now().Unix()
	nonce := hex.EncodeToString(b)
	parts := append([]string{protocol, action}, fields...)
	parts = append(parts, fmt.Sprint(now), nonce)
	wire := strings.Join(parts, "\n")
	sig := ed25519.Sign(i.private, []byte(wire))
	return signedMessage{
		DeviceID: i.DeviceID(), PublicKey: base64.StdEncoding.EncodeToString(i.public),
		Timestamp: now, Nonce: nonce, Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

type Agent struct {
	identity *Identity
	base     string
	client   http.Client
	leaseMu sync.RWMutex
	deviceLease string
	grantMu sync.RWMutex
	grantVerifier *AgentGrantVerifier
	loginMu sync.RWMutex
	loginVerifier func(context.Context,*remotetransport.Channel,*AgentGrant,[]byte)([]byte,error)
	loginBudget agentLoginBudget
	// Internal-only test seam. Normal Agent operation MUST verify fresh
	// signed device proof with Portal over its configured HTTPS origin.
	liveGrantCheck func(context.Context,*AgentGrant)error
}

// NewAgent deliberately rejects non-HTTPS and user-supplied proxy paths.
// All machines share the same Portal URL; no public address for TaskDeck.
func NewAgent(portalURL, keyPath string, sourceClient *http.Client) (*Agent, error) {
	u, err := url.Parse(portalURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("portal must be an HTTPS origin (without path/query/credentials)")
	}
	identity, err := LoadOrCreateIdentity(keyPath)
	if err != nil {
		return nil, err
	}
	c := http.Client{Timeout: 10 * time.Second}
	if sourceClient != nil {
		c = *sourceClient
	}
	// Never follow redirects carrying signed device identity material.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	return &Agent{identity: identity, base: strings.TrimSuffix(u.String(), "/"), client: c,
		loginBudget:agentLoginBudget{
			// All workspace identities share this parent directory: the
			// old fixed filename caused unrelated projects to reject one
			// another's valid logins due to a mismatched Device ID.
			path:filepath.Join(filepath.Dir(keyPath),"remote-login-rate-"+identity.DeviceID()+".json"),
			legacyPath:filepath.Join(filepath.Dir(keyPath),"remote-login-rate.json"),
			deviceID:identity.DeviceID(),
		},
	}, nil
}

func (a *Agent) DeviceID() string { return a.identity.DeviceID() }

// PortalOrigin is the immutable HTTPS origin this Agent uses for signed
// grant introspection, not the Relay or a browser-provided endpoint.
func (a *Agent) PortalOrigin() string {
	if a==nil{return ""}
	return a.base
}

// ConfigurePortalGrantKey is a LOCAL-ONLY trust pin operation. It never
// accepts a key delivered through Relay or from a browser message. The
// calling installer/settings layer must obtain this public key from an
// owner-verified out-of-band channel. The key cannot be replaced while this
// Agent is running; rotation requires explicit new Agent initialization.
func (a *Agent) ConfigurePortalGrantKey(publicKeyBase64 string) error {
	if a==nil || a.identity==nil {return errors.New("Agent is unavailable")}
	v,err:=NewAgentGrantVerifier(publicKeyBase64,a.base,a.DeviceID())
	if err!=nil{return err}
	a.grantMu.Lock()
	defer a.grantMu.Unlock()
	if a.grantVerifier!=nil {
		return errors.New("Agent Portal trust key is already pinned")
	}
	a.grantVerifier=v
	return nil
}

// VerifyPortalGrant fails closed until the owner has pinned Portal's public
// signing key. Only a signed, unexpired, single-use grant for this Agent and
// an established E2EE channel is accepted. This method grants NO command
// execution, HTTP access or TaskDeck login on its own.
func (a *Agent) VerifyPortalGrant(ticket string,channel *remotetransport.Channel,now time.Time) (*AgentGrant,error) {
	if a==nil {return nil,remotetransport.ErrRemoteActionDenied}
	a.grantMu.RLock()
	v:=a.grantVerifier
	a.grantMu.RUnlock()
	if v==nil {return nil,remotetransport.ErrRemoteActionDenied}
	return v.Consume(ticket,channel,now)
}


// ConfigureEncryptedLoginVerifier registers an in-process TaskDeck-only
// callback. Neither Relay nor Portal supplies this callback or receives a
// password. It is intentionally one-time, owner-configured at daemon startup.
func (a *Agent) ConfigureEncryptedLoginVerifier(
 verify func(context.Context,*remotetransport.Channel,*AgentGrant,[]byte)([]byte,error),
) error {
 if a==nil || verify==nil {return errors.New("missing TaskDeck login verifier")}
 a.loginMu.Lock()
 defer a.loginMu.Unlock()
 if a.loginVerifier!=nil {return errors.New("TaskDeck login verifier already configured")}
 a.loginVerifier=verify
 return nil
}

func (a *Agent) verifyEncryptedLogin(ctx context.Context,channel *remotetransport.Channel,
 grant *AgentGrant,ciphertext []byte) ([]byte,error) {
 a.loginMu.RLock()
 verify:=a.loginVerifier
 a.loginMu.RUnlock()
 // The Agent (not a caller-supplied TaskDeck callback) is responsible
 // for admitting EXACTLY one login attempt per locally pinned Portal ticket.
 // This single-use admission happens before Portal work. The encrypted
 // login frame itself is normally the FIRST authenticated browser record,
 // so peer possession is verified after the TaskDeck callback decrypts it.
 if verify==nil || !a.OwnsPortalGrant(grant) || channel==nil ||
  channel!=grant.channel || !channel.Established() {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // A grant that an unexpected caller consumed before the Agent's
 // admission cannot be laundered through a later callback.
 if grant.loginUsed.Load() {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 if _,err:=grant.RoleFor(channel,time.Now());err!=nil ||
  !grant.loginStarted.CompareAndSwap(false,true) {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // This one-use admission is intentionally never released on a failed
 // network call, invalid credential or a callback error.
 if err:=a.confirmCurrentPortalGrant(ctx,grant);err!=nil {
  grant.Close()
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Device-wide bounded scrypt work, independent of the username and
 // of repeated Portal ticket issuance. Disabled by default at startup.
 allowed,quotaErr:=a.loginBudget.allowPersistent(time.Now())
 if quotaErr!=nil || !allowed {
  grant.Close()
  return nil,remotetransport.ErrRemoteActionDenied
 }
 sealed,err:=verify(ctx,channel,grant,ciphertext)
 // An injected callback must not be able to return success without the
 // TaskDeck credential verifier consuming this exact one-use grant.
 if err!=nil || !grant.loginUsed.Load() ||
  !channel.AuthenticatedPeerTraffic() ||
  remotetransport.VerifyOutgoingRemoteLoginResult(channel,sealed)!=nil {
  // Never pass a misconfigured callback's plaintext, stale ciphertext,
  // malformed JSON or additional TaskDeck data to Relay.
  grant.Close()
  channel.Close()
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Local password verification may take seconds. A Portal grant can be
 // revoked during that work, or while the second HTTPS introspection is
 // pending. Explicit local grant closure MUST veto a late positive result.
 // The TaskDeck one-shot callback now retires only its local principal;
 // this Agent alone closes the Portal grant after both checks succeed.
 if ctx.Err()!=nil || grant.closed.Load() || !channel.Established() ||
  a.confirmCurrentPortalGrant(ctx,grant)!=nil ||
  ctx.Err()!=nil || grant.closed.Load() || !channel.Established() ||
  !grant.loginUsed.Load() {
  grant.Close()
  channel.Close()
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Atomic finalization linearizes explicit grant revocation against
 // the last one-shot login decision. If another goroutine revoked this
 // grant after the HTTPS check, its Close() wins the CAS and no success
 // bytes may leave Agent. If finalization wins, the grant was consumed
 // BEFORE the result was released; it is never an application capability.
 if !grant.closed.CompareAndSwap(false,true) {
  channel.Close()
  return nil,remotetransport.ErrRemoteActionDenied
 }
 return sealed,nil
}

// Enroll generates one local code for explicit admin confirmation. Only a hash
// of the raw random code is sent to the Portal. The code is not persisted.
func (a *Agent) Enroll(ctx context.Context, displayName string) (string, error) {
	if len(displayName) < 1 || len(displayName) > 80 || !utf8.ValidString(displayName) || strings.IndexFunc(displayName, func(r rune) bool {
		return r < 32 || r == 127
	}) >= 0 {
		return "", errors.New("invalid device display name")
	}
	rawCode := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, rawCode); err != nil {
		return "", err
	}
	pairingCode := hex.EncodeToString(rawCode)
	pairingHash := sha256.Sum256(rawCode)
	hash := hex.EncodeToString(pairingHash[:])
	signed, err := a.identity.sign("enroll", a.identity.DeviceID(), displayName, hash)
	if err != nil {
		return "", err
	}
	signed.DisplayName = displayName
	signed.PairingHash = hash
	if err := a.post(ctx, "/api/agent/enroll", signed); err != nil {
		return "", err
	}
	return pairingCode, nil
}

func (a *Agent) Heartbeat(ctx context.Context) error {
	signed, err := a.identity.sign("heartbeat", a.identity.DeviceID())
	if err != nil {
		return err
	}
	reply, err := a.postReply(ctx, "/api/agent/heartbeat", signed)
	if err != nil {
		a.clearLease()
		return err
	}
	var data struct {
		OK bool `json:"ok"`
		DeviceLease string `json:"device_lease"`
	}
	if err := json.Unmarshal(reply, &data); err != nil || !data.OK {
		a.clearLease()
		return errors.New("Portal heartbeat response invalid")
	}
	a.leaseMu.Lock()
	a.deviceLease = data.DeviceLease
	a.leaseMu.Unlock()
	return nil
}

// Portal failure or unpairing must not leave a stale signed lease cached
// for a subsequent Relay reconnect. The Relay separately rechecks expiry.
func (a *Agent) clearLease() {
	a.leaseMu.Lock()
	a.deviceLease = ""
	a.leaseMu.Unlock()
}

func (a *Agent) currentLease() string {
	a.leaseMu.RLock()
	defer a.leaseMu.RUnlock()
	return a.deviceLease
}

func (a *Agent) post(ctx context.Context, endpoint string, payload signedMessage) error {
	_, err := a.postReply(ctx, endpoint, payload)
	return err
}
func (a *Agent) postReply(ctx context.Context, endpoint string, payload signedMessage) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return a.postRaw(ctx,endpoint,data)
}

func (a *Agent) postRaw(ctx context.Context, endpoint string, data []byte) ([]byte,error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4097))
	if readErr != nil { return nil, readErr }
	if len(raw) > 4096 { return nil, errors.New("Portal response too large") }
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote portal rejected %s: HTTP %d", endpoint, response.StatusCode)
	}
	return raw, nil
}
