package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

// RemoteSettings only controls the outbound presence agent. It does not
// authorize remote execution or expose internal TaskDeck handlers.
type RemoteSettings struct {
	Enabled   bool   `json:"enabled"`
	PortalURL string `json:"portal_url"`
	RelayURL string `json:"relay_url"`
	PortalPublicKey string `json:"portal_public_key"`
}

func ReadRemoteSettings(workspace string) (RemoteSettings, error) {
	cfg, _, err := Load(workspace)
	if err != nil {
		return RemoteSettings{}, err
	}
	return RemoteSettings{Enabled: cfg.RemoteAccessEnabled, PortalURL: cfg.RemotePortalURL, RelayURL: cfg.RemoteRelayURL,PortalPublicKey:cfg.RemotePortalPublicKey}, nil
}

func SetRemoteSettings(workspace string, v RemoteSettings) error {
	if strings.TrimSpace(v.PortalURL) != "" {
		if err := ValidateRemotePortalURL(v.PortalURL); err != nil {
			return err
		}
	}
	original, file, err := Load(workspace)
	if err != nil {
		return err
	}
	original.RemoteAccessEnabled = v.Enabled
	original.RemotePortalURL = v.PortalURL
	original.RemoteRelayURL = v.RelayURL
	original.RemotePortalPublicKey = v.PortalPublicKey
	if err := original.Validate(); err != nil {
		return err
	}
	path, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if file != path {
		return fmt.Errorf("config path changed during remote settings update")
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start, end := -1, len(lines)
	section := ""
	for i, line := range lines {
		t := strings.ToLower(strings.TrimSpace(line))
		if !strings.HasPrefix(t, "[") || !strings.HasSuffix(t, "]") {
			continue
		}
		name := strings.TrimSpace(t[1 : len(t)-1])
		if section == "remote_access" && end == len(lines) {
			end = i
		}
		section = name
		if section == "remote_access" && start < 0 {
			start = i
		}
	}
	enabled := "enabled = false"
	if v.Enabled {
		enabled = "enabled = true"
	}
	portal := "portal_url = " + v.PortalURL
	relay := "relay_url = " + v.RelayURL
	portalKey := "portal_public_key = " + v.PortalPublicKey
	if start < 0 {
		lines = append(lines, "", "[remote_access]", enabled, portal, relay,portalKey)
	} else {
		body := make([]string, 0, end-start)
		for _, line := range lines[start+1 : end] {
			key, _, found := strings.Cut(strings.TrimSpace(line), "=")
			if found {
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "enabled", "portal_url", "relay_url", "portal_public_key":
					continue
				}
			}
			body = append(body, line)
		}
		updated := make([]string, 0, len(lines)+2)
		updated = append(updated, lines[:start+1]...)
		updated = append(updated, body...)
		updated = append(updated, enabled, portal, relay,portalKey)
		updated = append(updated, lines[end:]...)
		lines = updated
	}
	result := []byte(strings.Join(lines, "\n") + "\n")
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".remote-config-*")
	if err != nil {
		return err
	}
	tmp := temp.Name()
	defer os.Remove(tmp)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(result); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// Detect edits performed while the settings dialog was open. This is not
	// a cross-process lock; callers still need the existing config writer lock.
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, current) {
		return fmt.Errorf("config changed outside remote settings editor; retry after reload")
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
