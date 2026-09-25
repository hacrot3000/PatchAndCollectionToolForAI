package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	passwordScryptLogN       = 17
	passwordScryptR          = 8
	passwordScryptP          = 1
	passwordScryptSaltBytes  = 16
	passwordScryptKeyBytes   = 32
	passwordScryptMaxBytes   = 4096
	passwordScryptMaxMemory  = 256 * 1024 * 1024
	passwordScryptHashPrefix = "$scrypt$"
)

// HashPassword derives a one-way password hash using Python's standard-library
// scrypt implementation. The plaintext password is sent only over the child
// process stdin pipe; it is never placed in argv or the environment.
func HashPassword(ctx context.Context, password string) (string, error) {
	passwordBytes := []byte(password)
	if len(passwordBytes) == 0 {
		return "", fmt.Errorf("password cannot be empty")
	}
	if len(passwordBytes) > passwordScryptMaxBytes {
		return "", fmt.Errorf("password exceeds maximum length of %d bytes", passwordScryptMaxBytes)
	}

	salt := make([]byte, passwordScryptSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := derivePasswordScrypt(ctx, passwordBytes, salt)
	if err != nil {
		return "", err
	}

	params := fmt.Sprintf(
		"v=1,ln=%d,r=%d,p=%d",
		passwordScryptLogN,
		passwordScryptR,
		passwordScryptP,
	)
	return strings.Join([]string{
		"",
		"scrypt",
		params,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

// VerifyPassword validates a password against a TaskDeck scrypt hash. Malformed
// or unsupported hashes fail closed with an error so authentication code can
// distinguish invalid stored state from a normal password mismatch.
func VerifyPassword(ctx context.Context, password, encodedHash string) (bool, error) {
	passwordBytes := []byte(password)
	if len(passwordBytes) == 0 || len(passwordBytes) > passwordScryptMaxBytes {
		return false, nil
	}

	salt, expected, err := parsePasswordScryptHash(encodedHash)
	if err != nil {
		return false, err
	}
	actual, err := derivePasswordScrypt(ctx, passwordBytes, salt)
	if err != nil {
		return false, err
	}
	if len(actual) != len(expected) {
		return false, nil
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func parsePasswordScryptHash(encoded string) ([]byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" || parts[1] != "scrypt" {
		return nil, nil, fmt.Errorf("unsupported password hash format")
	}

	params := make(map[string]int, 4)
	for _, item := range strings.Split(parts[2], ",") {
		keyValue := strings.SplitN(item, "=", 2)
		if len(keyValue) != 2 {
			return nil, nil, fmt.Errorf("invalid scrypt parameter")
		}
		value, err := strconv.Atoi(keyValue[1])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid scrypt parameter %q", keyValue[0])
		}
		if _, exists := params[keyValue[0]]; exists {
			return nil, nil, fmt.Errorf("duplicate scrypt parameter %q", keyValue[0])
		}
		params[keyValue[0]] = value
	}
	if len(params) != 4 ||
		params["v"] != 1 ||
		params["ln"] != passwordScryptLogN ||
		params["r"] != passwordScryptR ||
		params["p"] != passwordScryptP {
		return nil, nil, fmt.Errorf("unsupported scrypt parameters")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != passwordScryptSaltBytes {
		return nil, nil, fmt.Errorf("invalid scrypt salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) != passwordScryptKeyBytes {
		return nil, nil, fmt.Errorf("invalid scrypt key")
	}
	return salt, expected, nil
}

type passwordScryptRequest struct {
	Password string `json:"password"`
	Salt     string `json:"salt"`
	N        int    `json:"n"`
	R        int    `json:"r"`
	P        int    `json:"p"`
	DKLen    int    `json:"dklen"`
	MaxMem   int    `json:"maxmem"`
}

type passwordScryptResponse struct {
	OK    bool   `json:"ok"`
	Key   string `json:"key,omitempty"`
	Error string `json:"error,omitempty"`
}

func derivePasswordScrypt(ctx context.Context, password, salt []byte) ([]byte, error) {
	executable, prefix, err := resolvePythonSQLiteCommand()
	if err != nil {
		return nil, err
	}
	request := passwordScryptRequest{
		Password: base64.RawStdEncoding.EncodeToString(password),
		Salt:     base64.RawStdEncoding.EncodeToString(salt),
		N:        1 << passwordScryptLogN,
		R:        passwordScryptR,
		P:        passwordScryptP,
		DKLen:    passwordScryptKeyBytes,
		MaxMem:   passwordScryptMaxMemory,
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode scrypt request: %w", err)
	}

	args := append(append([]string(nil), prefix...), "-u", "-c", pythonScryptHelper)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = bytes.NewReader(requestBytes)
	var stderr boundedTextBuffer
	stderr.limit = 8 * 1024
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("derive password scrypt hash: %s", detail)
	}

	var response passwordScryptResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("decode scrypt response: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "unknown scrypt helper error"
		}
		return nil, fmt.Errorf("derive password scrypt hash: %s", response.Error)
	}
	key, err := base64.RawStdEncoding.DecodeString(response.Key)
	if err != nil || len(key) != passwordScryptKeyBytes {
		return nil, fmt.Errorf("invalid scrypt helper key")
	}
	return key, nil
}

const pythonScryptHelper = `
import base64
import hashlib
import json
import sys

if sys.version_info < (3, 10):
    raise RuntimeError("TaskDeck shared-server password hashing requires Python 3.10+")
if not hasattr(hashlib, "scrypt"):
    raise RuntimeError("Python hashlib.scrypt is unavailable")

try:
    req = json.load(sys.stdin)
    password = base64.b64decode(req["password"])
    salt = base64.b64decode(req["salt"])
    key = hashlib.scrypt(
        password,
        salt=salt,
        n=int(req["n"]),
        r=int(req["r"]),
        p=int(req["p"]),
        dklen=int(req["dklen"]),
        maxmem=int(req["maxmem"]),
    )
    print(json.dumps({
        "ok": True,
        "key": base64.b64encode(key).decode("ascii").rstrip("="),
    }))
except Exception as exc:
    print(json.dumps({
        "ok": False,
        "error": type(exc).__name__ + ": " + str(exc),
    }))
`
