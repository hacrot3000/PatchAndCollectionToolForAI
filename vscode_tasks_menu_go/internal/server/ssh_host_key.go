package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const maxSSHKnownHostsOutputBytes = 64 << 10

var sshFingerprintPattern = regexp.MustCompile(`SHA256:[A-Za-z0-9+/=]+`)

type sshKnownHostMatch struct {
	File    string `json:"file"`
	Lookup  string `json:"lookup"`
	Entries int    `json:"entries"`
}

type sshHostKeyInfo struct {
	Lookup               string              `json:"lookup"`
	KnownHostsFiles      []string            `json:"known_hosts_files,omitempty"`
	Matches              []sshKnownHostMatch `json:"matches,omitempty"`
	CandidateFingerprint string              `json:"candidate_fingerprint,omitempty"`
	CanRemove            bool                `json:"can_remove"`
}

func classifySSHHostKeyFailure(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "remote host identification has changed"), strings.Contains(text, "offending ") && strings.Contains(text, " key in "):
		return "host_key_changed"
	case strings.Contains(text, "host key verification failed"):
		return "host_key_verification"
	default:
		return ""
	}
}

func sshKnownHostLookup(profile sshprofile.Profile) string {
	if profile.Port == 22 { return profile.Host }
	return "[" + strings.Trim(profile.Host, "[]") + "]:" + strconv.Itoa(profile.Port)
}

func expandSSHConfigPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") || value == "/dev/null" { return "" }
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil { value = filepath.Join(home, value[2:]) }
	}
	return filepath.Clean(value)
}

func sshKnownHostsFiles(ctx context.Context, profile sshprofile.Profile) []string {
	files := []string{}
	seen := map[string]bool{}
	add := func(value string) {
		value = expandSSHConfigPath(value)
		if value == "" || seen[value] { return }
		seen[value] = true; files = append(files, value)
	}
	if executable, err := sshclient.FindOpenSSH(); err == nil {
		args := []string{"-G", "-p", strconv.Itoa(profile.Port)}
		if profile.ProxyJump != "" { args = append(args, "-J", profile.ProxyJump) }
		args = append(args, profile.Username+"@"+profile.Host)
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(probeCtx, executable, args...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		if output, err := cmd.Output(); err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				fields := strings.Fields(strings.TrimSpace(line))
				if len(fields) < 2 || !strings.EqualFold(fields[0], "userknownhostsfile") { continue }
				for _, value := range fields[1:] { add(value) }
			}
		}
		cancel()
	}
	if len(files) == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".ssh", "known_hosts"))
			add(filepath.Join(home, ".ssh", "known_hosts2"))
		}
	}
	return files
}

func inspectSSHHostKey(ctx context.Context, profile sshprofile.Profile, diagnostic string) sshHostKeyInfo {
	info := sshHostKeyInfo{Lookup: sshKnownHostLookup(profile)}
	if match := sshFingerprintPattern.FindString(diagnostic); match != "" { info.CandidateFingerprint = match }
	info.KnownHostsFiles = sshKnownHostsFiles(ctx, profile)
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil { return info }
	for _, path := range info.KnownHostsFiles {
		stat, err := os.Stat(path)
		if err != nil || !stat.Mode().IsRegular() { continue }
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(probeCtx, keygen, "-F", info.Lookup, "-f", path)
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil { continue }
		entries := 0
		for _, line := range strings.Split(string(output), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") { entries++ }
		}
		if entries > 0 { info.Matches = append(info.Matches, sshKnownHostMatch{File: path, Lookup: info.Lookup, Entries: entries}) }
	}
	info.CanRemove = len(info.Matches) > 0
	return info
}

func removeSSHHostKey(ctx context.Context, profile sshprofile.Profile) (sshHostKeyInfo, string, error) {
	info := inspectSSHHostKey(ctx, profile, "")
	if !info.CanRemove { return info, "", errors.New("no matching known_hosts entry was found for this SSH profile") }
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil { return info, "", errors.New("ssh-keygen is not available in PATH") }
	var output []string
	for _, match := range info.Matches {
		removeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		cmd := exec.CommandContext(removeCtx, keygen, "-R", info.Lookup, "-f", match.File)
		data, runErr := cmd.CombinedOutput()
		cancel()
		text := strings.TrimSpace(string(data))
		if len(text) > maxSSHKnownHostsOutputBytes { text = text[:maxSSHKnownHostsOutputBytes] }
		if text != "" { output = append(output, text) }
		if runErr != nil { return info, strings.Join(output, "\n"), fmt.Errorf("remove SSH known_hosts entry from %s: %w", match.File, runErr) }
	}
	updated := inspectSSHHostKey(ctx, profile, "")
	return updated, strings.Join(output, "\n"), nil
}
