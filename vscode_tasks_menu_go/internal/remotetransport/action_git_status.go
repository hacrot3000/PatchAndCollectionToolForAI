package remotetransport

import (
    "encoding/json"
    "unicode"
    "unicode/utf8"
)

const RemoteGitStatusResultProtocol = "taskdeck-e2ee-git-status-result-v1"

// Fit the whole response inside the existing 512-byte cleartext and
// 537-byte ciphertext application frame cap. Refuse overflow, never
// silently truncate or create an unbounded streaming route.
const MaxRemoteGitStatusText = 256

type remoteGitStatusResultWire struct {
    Type string `json:"type"`
    RequestID string `json:"request_id"`
    Status string `json:"status"`
}

func validRemoteGitStatusText(s string) bool {
    if len(s) == 0 || len(s) > MaxRemoteGitStatusText || !utf8.ValidString(s) {
        return false
    }
    for _, r := range s {
        if r == '\n' || r == '\t' {
            continue
        }
        if unicode.IsControl(r) {
            return false
        }
    }
    return true
}

// SealRemoteActionGitStatusResult is only called after the Agent's local
// TaskDeck principal was independently authenticated and git.status RBAC
// was checked before and after executing the read-only git subprocess.
func SealRemoteActionGitStatusResult(ch *Channel, id, status string) ([]byte, error) {
    if ch == nil || !ch.Established() || !validActionID(id) ||
        !validRemoteGitStatusText(status) {
        return nil, ErrInvalidRemoteAction
    }
    clear, err := json.Marshal(remoteGitStatusResultWire{
        Type: RemoteGitStatusResultProtocol, RequestID: id, Status: status,
    })
    if err != nil || len(clear) > maxRemoteActionPlaintext {
        return nil, ErrInvalidRemoteAction
    }
    return ch.Seal(clear)
}

// OpenRemoteActionGitStatusResult rejects forged capabilities, duplicate or
// unknown fields, wrong IDs and malformed Git output. Any invalid AEAD
// record permanently destroys the browser channel, as with session.ping.
func OpenRemoteActionGitStatusResult(ch *Channel, frame []byte, wantID string) (string, error) {
    if ch == nil || !ch.Established() || !validActionID(wantID) {
        return "", ErrInvalidRemoteAction
    }
    clear, err := ch.Open(frame)
    if err != nil { return "", ErrInvalidRemoteAction }
    defer wipeLoginBytes(clear)
    fields, err := decodeActionObject(clear, "type", "request_id", "status")
    if err == nil {
        var typ, id, status string
        if json.Unmarshal(fields["type"], &typ) != nil ||
            json.Unmarshal(fields["request_id"], &id) != nil ||
            json.Unmarshal(fields["status"], &status) != nil ||
            typ != RemoteGitStatusResultProtocol || id != wantID ||
            !validRemoteGitStatusText(status) {
            err = ErrInvalidRemoteAction
        } else {
            return status, nil
        }
    }
    ch.Close()
    return "", ErrInvalidRemoteAction
}
