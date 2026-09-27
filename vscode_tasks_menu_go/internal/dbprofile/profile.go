package dbprofile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

const (
	DefaultTransport = TransportDirect

	maxIDBytes        = 128
	maxNameBytes      = 128
	maxAdapterIDBytes = 128
	maxHostBytes      = 512
	maxUsernameBytes  = 256
	maxDatabaseBytes  = 512
	maxFileBytes      = 4096
	maxSecretRefBytes = 256
	maxOptions        = 32
	maxOptionKeyBytes = 128
	maxOptionValBytes = 4096
)

type Transport string

const (
	TransportDirect    Transport = "direct"
	TransportSSHTunnel Transport = "ssh_tunnel"
)

type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AdapterID string `json:"adapter_id"`
	Transport Transport `json:"transport"`

	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Database string `json:"database,omitempty"`
	File     string `json:"file,omitempty"`

	SSHProfileID string `json:"ssh_profile_id,omitempty"`
	ReadOnly     bool   `json:"read_only,omitempty"`
	Options      map[string]string `json:"options,omitempty"`

	SecretRef string `json:"-"`
}

func NewID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate database profile id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func Normalize(profile Profile) (Profile, error) {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.AdapterID = strings.TrimSpace(profile.AdapterID)
	profile.Host = strings.TrimSpace(profile.Host)
	profile.Username = strings.TrimSpace(profile.Username)
	profile.Database = strings.TrimSpace(profile.Database)
	profile.File = strings.TrimSpace(profile.File)
	profile.SSHProfileID = strings.TrimSpace(profile.SSHProfileID)
	profile.SecretRef = strings.TrimSpace(profile.SecretRef)

	if profile.Transport == "" {
		profile.Transport = DefaultTransport
	}
	if err := validateToken("database profile id", profile.ID, maxIDBytes, true); err != nil {
		return Profile{}, err
	}
	if err := validateText("database profile name", profile.Name, maxNameBytes, true); err != nil {
		return Profile{}, err
	}
	if err := validateToken("database adapter id", profile.AdapterID, maxAdapterIDBytes, true); err != nil {
		return Profile{}, err
	}
	switch profile.Transport {
	case TransportDirect:
		if profile.SSHProfileID != "" {
			return Profile{}, fmt.Errorf("direct database profile must not reference an SSH profile")
		}
	case TransportSSHTunnel:
		if err := validateToken("ssh profile id", profile.SSHProfileID, maxIDBytes, true); err != nil {
			return Profile{}, err
		}
		if profile.Host == "" {
			return Profile{}, fmt.Errorf("ssh-tunneled database profile requires a remote database host")
		}
		if profile.Port == 0 {
			return Profile{}, fmt.Errorf("ssh-tunneled database profile requires a remote database port")
		}
	default:
		return Profile{}, fmt.Errorf("unsupported database transport %q", profile.Transport)
	}

	if profile.Host != "" {
		if err := validateText("database host", profile.Host, maxHostBytes, false); err != nil {
			return Profile{}, err
		}
		if strings.ContainsAny(profile.Host, " \t/") {
			return Profile{}, fmt.Errorf("database host must not contain whitespace or slash")
		}
	}
	if profile.Port < 0 || profile.Port > 65535 {
		return Profile{}, fmt.Errorf("database port must be between 1 and 65535 when set")
	}
	if profile.Username != "" {
		if err := validateText("database username", profile.Username, maxUsernameBytes, false); err != nil {
			return Profile{}, err
		}
	}
	if profile.Database != "" {
		if err := validateText("database name", profile.Database, maxDatabaseBytes, false); err != nil {
			return Profile{}, err
		}
	}
	if profile.File != "" {
		if err := validateText("database file", profile.File, maxFileBytes, false); err != nil {
			return Profile{}, err
		}
	}
	if profile.SecretRef != "" {
		if err := validateSecretRef(profile.SecretRef); err != nil {
			return Profile{}, err
		}
	}

	if len(profile.Options) > maxOptions {
		return Profile{}, fmt.Errorf("database profile exceeds %d options", maxOptions)
	}
	if len(profile.Options) != 0 {
		normalized := make(map[string]string, len(profile.Options))
		for key, value := range profile.Options {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if err := validateToken("database option key", key, maxOptionKeyBytes, true); err != nil {
				return Profile{}, err
			}
			if err := validateText("database option value", value, maxOptionValBytes, false); err != nil {
				return Profile{}, fmt.Errorf("database option %q: %w", key, err)
			}
			normalized[key] = value
		}
		profile.Options = normalized
	}
	return profile, nil
}

func validateSecretRef(value string) error {
	if err := validateText("database secret reference", value, maxSecretRefBytes, true); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '-' || r == '_' || r == '.' || r == ':' || r == '/' {
			continue
		}
		return fmt.Errorf("database secret reference contains unsupported character %q", r)
	}
	return nil
}

func validateToken(label, value string, max int, required bool) error {
	if err := validateText(label, value, max, required); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return fmt.Errorf("%s contains unsupported character %q", label, r)
	}
	return nil
}

func validateText(label, value string, max int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d bytes", label, max)
	}
	for _, r := range value {
		if r == 0 || r == '\r' || r == '\n' || unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", label)
		}
	}
	return nil
}
