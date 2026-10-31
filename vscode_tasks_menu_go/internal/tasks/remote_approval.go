package tasks

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

var ErrInvalidRemoteTaskApproval = errors.New("task definition is not eligible for a remote approval digest")

// RemoteApprovalToken binds the host's independent approval to the complete
// canonical task definition, not the public 31-bit menu selector. A malicious
// tasks.json edit must never preserve authorization by finding an ID collision.
// Only primary-root tasks with no TaskDeck secret mapping can be approved.
func RemoteApprovalToken(task Task) (string, error) {
	if task.ID <= 0 || task.ID > 0x7fffffff || task.Raw == nil || len(task.SecretEnv) != 0 {
		return "", ErrInvalidRemoteTaskApproval
	}
	canonical, err := json.Marshal(task.Raw)
	if err != nil {
		return "", ErrInvalidRemoteTaskApproval
	}
	digest := sha256.Sum256(canonical)
	id := int(binary.BigEndian.Uint32(digest[:4]) & 0x7fffffff)
	if id == 0 {
		id = 1
	}
	if id != task.ID {
		return "", ErrInvalidRemoteTaskApproval
	}
	return strconv.Itoa(task.ID) + "@" + hex.EncodeToString(digest[:]), nil
}
