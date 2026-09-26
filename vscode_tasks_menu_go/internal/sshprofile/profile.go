package sshprofile

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	DefaultPort                 = 22
	DefaultConnectTimeoutSecond = 10
	DefaultServerAliveSecond    = 15
	DefaultServerAliveCountMax  = 3

	maxIDLen             = 128
	maxNameLen           = 128
	maxHostLen           = 512
	maxUsernameLen       = 128
	maxPathLen           = 4096
	maxSecretRefLen      = 256
	maxProxyJumpLen      = 512
	maxPresetCommands    = 32
	maxPresetNameLen     = 128
	maxPresetCommandLen  = 8192
	maxConnectTimeoutSec = 300
	maxServerAliveSec    = 3600
	maxServerAliveCount  = 20
)

type AuthMethod string

const (
	AuthAgent      AuthMethod = "agent"
	AuthPrivateKey AuthMethod = "private_key"
	AuthPassword   AuthMethod = "password"
)

type PresetCommand struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
}

type Profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`

	AuthMethod   AuthMethod `json:"auth_method"`
	IdentityFile string     `json:"identity_file,omitempty"`

	// SecretRef identifies a secret in the private SecretStore. It is deliberately
	// omitted from JSON so profile values cannot leak through an API accidentally.
	SecretRef string `json:"-"`

	CustomHomeDir string          `json:"custom_home_dir,omitempty"`
	PresetCommands []PresetCommand `json:"preset_commands,omitempty"`

	ConnectTimeoutSeconds    int    `json:"connect_timeout_seconds,omitempty"`
	ServerAliveIntervalSeconds int  `json:"server_alive_interval_seconds,omitempty"`
	ServerAliveCountMax      int    `json:"server_alive_count_max,omitempty"`
	ProxyJump                string `json:"proxy_jump,omitempty"`
}

func Normalize(p Profile) (Profile, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.IdentityFile = strings.TrimSpace(p.IdentityFile)
	p.SecretRef = strings.TrimSpace(p.SecretRef)
	p.CustomHomeDir = strings.TrimSpace(p.CustomHomeDir)
	p.ProxyJump = strings.TrimSpace(p.ProxyJump)

	if p.Port == 0 {
		p.Port = DefaultPort
	}
	if p.ConnectTimeoutSeconds == 0 {
		p.ConnectTimeoutSeconds = DefaultConnectTimeoutSecond
	}
	if p.ServerAliveIntervalSeconds == 0 {
		p.ServerAliveIntervalSeconds = DefaultServerAliveSecond
	}
	if p.ServerAliveCountMax == 0 {
		p.ServerAliveCountMax = DefaultServerAliveCountMax
	}

	if err := validateID("profile id", p.ID); err != nil {
		return Profile{}, err
	}
	if err := validateText("profile name", p.Name, maxNameLen, true); err != nil {
		return Profile{}, err
	}
	if err := validateHost(p.Host); err != nil {
		return Profile{}, err
	}
	if err := validateUsername(p.Username); err != nil {
		return Profile{}, err
	}
	if p.Port < 1 || p.Port > 65535 {
		return Profile{}, fmt.Errorf("ssh port must be between 1 and 65535")
	}

	switch p.AuthMethod {
	case AuthAgent:
		if p.SecretRef != "" {
			return Profile{}, fmt.Errorf("agent authentication must not reference a secret")
		}
	case AuthPrivateKey:
		if err := validateText("identity file", p.IdentityFile, maxPathLen, true); err != nil {
			return Profile{}, err
		}
	case AuthPassword:
		if err := validateText("secret reference", p.SecretRef, maxSecretRefLen, true); err != nil {
			return Profile{}, err
		}
	default:
		return Profile{}, fmt.Errorf("unsupported ssh authentication method %q", p.AuthMethod)
	}

	if p.IdentityFile != "" {
		if err := validateText("identity file", p.IdentityFile, maxPathLen, true); err != nil {
			return Profile{}, err
		}
	}
	if p.CustomHomeDir != "" {
		if err := validateText("custom home dir", p.CustomHomeDir, maxPathLen, true); err != nil {
			return Profile{}, err
		}
	}
	if p.ProxyJump != "" {
		if err := validateText("proxy jump", p.ProxyJump, maxProxyJumpLen, true); err != nil {
			return Profile{}, err
		}
		if strings.ContainsAny(p.ProxyJump, " 	") {
			return Profile{}, fmt.Errorf("proxy jump must be a single OpenSSH destination")
		}
	}
	if p.ConnectTimeoutSeconds < 1 || p.ConnectTimeoutSeconds > maxConnectTimeoutSec {
		return Profile{}, fmt.Errorf("ssh connect timeout must be between 1 and %d seconds", maxConnectTimeoutSec)
	}
	if p.ServerAliveIntervalSeconds < 1 || p.ServerAliveIntervalSeconds > maxServerAliveSec {
		return Profile{}, fmt.Errorf("ssh server alive interval must be between 1 and %d seconds", maxServerAliveSec)
	}
	if p.ServerAliveCountMax < 1 || p.ServerAliveCountMax > maxServerAliveCount {
		return Profile{}, fmt.Errorf("ssh server alive count must be between 1 and %d", maxServerAliveCount)
	}

	if len(p.PresetCommands) > maxPresetCommands {
		return Profile{}, fmt.Errorf("ssh preset commands exceed %d entries", maxPresetCommands)
	}
	seenPresetIDs := make(map[string]bool, len(p.PresetCommands))
	for i := range p.PresetCommands {
		item := p.PresetCommands[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Command = strings.TrimSpace(item.Command)
		item.Cwd = strings.TrimSpace(item.Cwd)
		if item.ID != "" {
			if err := validateID("preset id", item.ID); err != nil {
				return Profile{}, fmt.Errorf("preset %d: %w", i+1, err)
			}
			if seenPresetIDs[item.ID] {
				return Profile{}, fmt.Errorf("preset %d: duplicate id %q", i+1, item.ID)
			}
			seenPresetIDs[item.ID] = true
		}
		if err := validateText("preset name", item.Name, maxPresetNameLen, true); err != nil {
			return Profile{}, fmt.Errorf("preset %d: %w", i+1, err)
		}
		if err := validateText("preset command", item.Command, maxPresetCommandLen, true); err != nil {
			return Profile{}, fmt.Errorf("preset %d: %w", i+1, err)
		}
		if item.Cwd != "" {
			if err := validateText("preset cwd", item.Cwd, maxPathLen, true); err != nil {
				return Profile{}, fmt.Errorf("preset %d: %w", i+1, err)
			}
		}
		p.PresetCommands[i] = item
	}

	return p, nil
}

func validateID(label, value string) error {
	if err := validateText(label, value, maxIDLen, true); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("%s contains unsupported character %q", label, r)
	}
	return nil
}

func validateHost(value string) error {
	if err := validateText("ssh host", value, maxHostLen, true); err != nil {
		return err
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("ssh host must not start with -")
	}\n	if strings.ContainsAny(value, " 	/@") {
		return fmt.Errorf("ssh host must not contain whitespace, slash, or @")
	}
	return nil
}

func validateUsername(value string) error {
	if err := validateText("ssh username", value, maxUsernameLen, true); err != nil {
		return err
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("ssh username must not start with -")
	}\n	if strings.ContainsAny(value, " 	@/") {
		return fmt.Errorf("ssh username must not contain whitespace, slash, or @")
	}
	return nil
}

func validateText(label, value string, maxLen int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds %d bytes", label, maxLen)
	}
	for _, r := range value {
		if r == 0 || r == '\r' || r == '\n' || unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", label)
		}
	}
	return nil
}
