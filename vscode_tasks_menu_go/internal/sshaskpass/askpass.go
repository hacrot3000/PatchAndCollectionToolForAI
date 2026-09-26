package sshaskpass

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
)

const (
	defaultTTL       = 45 * time.Second
	maxRequestBytes  = 8 << 10
	maxResponseBytes = 128 << 10
	maxPromptBytes   = 2048
	maxSocketPathLen = 90
)

type Ticket struct {
	SocketPath string
	Token      string

	listener *net.UnixListener
	once     sync.Once
}

type request struct {
	Token  string `json:"token"`
	Prompt string `json:"prompt,omitempty"`
}

type response struct {
	OK     bool   `json:"ok"`
	Secret string `json:"secret,omitempty"`
}

func Prepare(store secretstore.Store, secretRef, runtimeDir string) (*Ticket, error) {
	if store == nil {
		return nil, errors.New("ssh askpass secret store is unavailable")
	}
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return nil, errors.New("ssh askpass secret reference is required")
	}
	runtimeDir = strings.TrimSpace(runtimeDir)
	if runtimeDir == "" || !filepath.IsAbs(runtimeDir) {
		return nil, errors.New("ssh askpass runtime directory must be absolute")
	}
	secret, err := store.Get(secretRef)
	if err != nil {
		return nil, fmt.Errorf("load ssh authentication secret: %w", err)
	}
	if len(secret) == 0 {
		zero(secret)
		return nil, errors.New("ssh authentication secret is empty")
	}

	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		zero(secret)
		return nil, fmt.Errorf("create ssh askpass runtime directory: %w", err)
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		zero(secret)
		return nil, fmt.Errorf("protect ssh askpass runtime directory: %w", err)
	}

	var tokenRaw [32]byte
	if _, err := io.ReadFull(rand.Reader, tokenRaw[:]); err != nil {
		zero(secret)
		return nil, fmt.Errorf("generate ssh askpass token: %w", err)
	}
	var nameRaw [8]byte
	if _, err := io.ReadFull(rand.Reader, nameRaw[:]); err != nil {
		zero(secret)
		return nil, fmt.Errorf("generate ssh askpass socket name: %w", err)
	}
	socketPath := filepath.Join(runtimeDir, "askpass-"+hex.EncodeToString(nameRaw[:])+".sock")
	if len(socketPath) > maxSocketPathLen {
		zero(secret)
		return nil, errors.New("ssh askpass runtime path is too long")
	}
	_ = os.Remove(socketPath)
	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		zero(secret)
		return nil, fmt.Errorf("listen ssh askpass socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		zero(secret)
		return nil, fmt.Errorf("protect ssh askpass socket: %w", err)
	}

	ticket := &Ticket{
		SocketPath: socketPath,
		Token:      hex.EncodeToString(tokenRaw[:]),
		listener:   listener,
	}
	go ticket.serve(secret, defaultTTL)
	return ticket, nil
}

func (t *Ticket) Close() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		if t.listener != nil {
			_ = t.listener.Close()
		}
		if t.SocketPath != "" {
			_ = os.Remove(t.SocketPath)
		}
	})
}

func (t *Ticket) serve(secret []byte, ttl time.Duration) {
	defer zero(secret)
	defer t.Close()
	if ttl <= 0 {
		ttl = defaultTTL
	}
	_ = t.listener.SetDeadline(time.Now().Add(ttl))
	conn, err := t.listener.AcceptUnix()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req request
	dec := json.NewDecoder(io.LimitReader(conn, maxRequestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return
	}
	if len(req.Token) != len(t.Token) || subtle.ConstantTimeCompare([]byte(req.Token), []byte(t.Token)) != 1 {
		return
	}
	if len(req.Prompt) > maxPromptBytes {
		return
	}
	encoded := base64.RawStdEncoding.EncodeToString(secret)
	_ = json.NewEncoder(conn).Encode(response{OK: true, Secret: encoded})
}

func RunHelper(socketPath, token, prompt string, out io.Writer) error {
	socketPath = strings.TrimSpace(socketPath)
	token = strings.TrimSpace(token)
	if socketPath == "" || token == "" {
		return errors.New("ssh askpass ticket is incomplete")
	}
	if !filepath.IsAbs(socketPath) || len(socketPath) > maxSocketPathLen {
		return errors.New("ssh askpass socket path is invalid")
	}
	if len(token) > 256 || strings.ContainsAny(token, "\r\n\x00") {
		return errors.New("ssh askpass token is invalid")
	}
	if len(prompt) > maxPromptBytes {
		prompt = prompt[:maxPromptBytes]
	}

	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return errors.New("ssh askpass broker is unavailable")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(conn).Encode(request{Token: token, Prompt: prompt}); err != nil {
		return errors.New("ssh askpass request failed")
	}

	var resp response
	dec := json.NewDecoder(io.LimitReader(conn, maxResponseBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&resp); err != nil || !resp.OK || resp.Secret == "" {
		return errors.New("ssh askpass response failed")
	}
	secret, err := base64.RawStdEncoding.DecodeString(resp.Secret)
	if err != nil {
		return errors.New("ssh askpass response is invalid")
	}
	defer zero(secret)
	if len(secret) == 0 {
		return errors.New("ssh askpass returned an empty secret")
	}
	if _, err := out.Write(secret); err != nil {
		return errors.New("ssh askpass output failed")
	}
	if _, err := out.Write([]byte("\n")); err != nil {
		return errors.New("ssh askpass output failed")
	}
	return nil
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
