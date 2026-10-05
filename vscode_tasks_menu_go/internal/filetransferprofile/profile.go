package filetransferprofile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

const (
	DefaultFTPPort             = 21
	DefaultConnectTimeoutSecond = 10

	maxIDBytes             = 128
	maxNameBytes           = 128
	maxHostBytes           = 512
	maxUsernameBytes       = 256
	maxPathBytes           = 4096
	maxSecretRefBytes      = 256
	maxConnectTimeoutSecond = 300
)

type Protocol string

const (
	ProtocolFTP  Protocol = "ftp"
	ProtocolSFTP Protocol = "sftp"
)

type FTPTLSMode string

const (
	FTPTLSPlain    FTPTLSMode = "plain"
	FTPTLSExplicit FTPTLSMode = "explicit"
	FTPTLSImplicit FTPTLSMode = "implicit"
)

type Profile struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Protocol Protocol `json:"protocol"`

	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`

	SSHProfileID string `json:"ssh_profile_id,omitempty"`
	InitialPath  string `json:"initial_path,omitempty"`

	ConnectTimeoutSeconds int        `json:"connect_timeout_seconds,omitempty"`
	FTPTLSMode            FTPTLSMode `json:"ftp_tls_mode,omitempty"`

	// SecretRef points at the private encrypted secret store. It must never be
	// serialized through public profile APIs.
	SecretRef string `json:"-"`
}

func NewID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate file-transfer profile id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func Normalize(profile Profile) (Profile, error) {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Host = strings.TrimSpace(profile.Host)
	profile.Username = strings.TrimSpace(profile.Username)
	profile.SSHProfileID = strings.TrimSpace(profile.SSHProfileID)
	profile.InitialPath = strings.TrimSpace(profile.InitialPath)
	profile.SecretRef = strings.TrimSpace(profile.SecretRef)

	if err := validateToken("file-transfer profile id", profile.ID, maxIDBytes, true); err != nil {
		return Profile{}, err
	}
	if err := validateText("file-transfer profile name", profile.Name, maxNameBytes, true); err != nil {
		return Profile{}, err
	}
	if profile.InitialPath == "" {
		profile.InitialPath = "."
	}
	if err := validateRemotePath(profile.InitialPath); err != nil {
		return Profile{}, err
	}

	switch profile.Protocol {
	case ProtocolFTP:
		if profile.FTPTLSMode == "" {
			profile.FTPTLSMode = FTPTLSPlain
		}
		switch profile.FTPTLSMode {
		case FTPTLSPlain, FTPTLSExplicit, FTPTLSImplicit:
		default:
			return Profile{}, fmt.Errorf("unsupported ftp tls mode %q", profile.FTPTLSMode)
		}
		if profile.Port == 0 {
			if profile.FTPTLSMode == FTPTLSImplicit {
				profile.Port = 990
			} else {
				profile.Port = DefaultFTPPort
			}
		}
		if profile.ConnectTimeoutSeconds == 0 {
			profile.ConnectTimeoutSeconds = DefaultConnectTimeoutSecond
		}
		if err := validateHost(profile.Host); err != nil {
			return Profile{}, err
		}
		if err := validateText("ftp username", profile.Username, maxUsernameBytes, true); err != nil {
			return Profile{}, err
		}
		if profile.Port < 1 || profile.Port > 65535 {
			return Profile{}, fmt.Errorf("ftp port must be between 1 and 65535")
		}
		if profile.ConnectTimeoutSeconds < 1 || profile.ConnectTimeoutSeconds > maxConnectTimeoutSecond {
			return Profile{}, fmt.Errorf("ftp connect timeout must be between 1 and %d seconds", maxConnectTimeoutSecond)
		}
		if profile.SSHProfileID != "" {
			return Profile{}, fmt.Errorf("ftp profile must not reference an SSH profile")
		}
		if profile.SecretRef != "" {
			if err := validateSecretRef(profile.SecretRef); err != nil {
				return Profile{}, err
			}
		}
	case ProtocolSFTP:
		if err := validateToken("ssh profile id", profile.SSHProfileID, maxIDBytes, true); err != nil {
			return Profile{}, err
		}
		if profile.Host != "" || profile.Port != 0 || profile.Username != "" || profile.SecretRef != "" || profile.ConnectTimeoutSeconds != 0 || profile.FTPTLSMode != "" {
			return Profile{}, fmt.Errorf("sftp profile must inherit host, port, username, authentication and timeout from its SSH profile")
		}
	default:
		return Profile{}, fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}

	return profile, nil
}

func validateHost(value string) error {
	if err := validateText("ftp host", value, maxHostBytes, true); err != nil {
		return err
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("ftp host must not start with -")
	}
	if strings.ContainsAny(value, " \t/@") {
		return fmt.Errorf("ftp host must not contain whitespace, slash, or @")
	}
	return nil
}

func validateRemotePath(value string) error {
	if err := validateText("remote path", value, maxPathBytes, true); err != nil {
		return err
	}
	return nil
}

func validateSecretRef(value string) error {
	if err := validateText("file-transfer secret reference", value, maxSecretRefBytes, true); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' || r == '/' {
			continue
		}
		return fmt.Errorf("file-transfer secret reference contains unsupported character %q", r)
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
