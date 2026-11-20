package sshprofile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate ssh profile id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
