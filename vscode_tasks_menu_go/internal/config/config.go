package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Bind          string
	Port          int
	AdvertiseHost string
	OpenBrowser   bool
	TLSCert       string
	TLSKey        string
	AuthEnabled   bool
	Username      string
	Password      string
}

func Default() Config {
	return Config{Bind: "127.0.0.1", Port: 0, OpenBrowser: true, Username: "admin"}
}

func Load(workspace string) (Config, string, error) {
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := writeDefault(path, cfg); err != nil {
			return Config{}, path, err
		}
		return cfg, path, nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return Config{}, path, fmt.Errorf("bảo vệ quyền truy cập %s: %w", path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return Config{}, path, err
	}
	defer f.Close()

	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch section + "." + key {
		case "server.bind":
			cfg.Bind = value
		case "server.port":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 65535 {
				return Config{}, path, fmt.Errorf("port không hợp lệ: %q", value)
			}
			cfg.Port = n
		case "server.advertise_host":
			cfg.AdvertiseHost = value
		case "server.open_browser":
			cfg.OpenBrowser = parseBool(value, true)
		case "server.tls_cert":
			cfg.TLSCert = value
		case "server.tls_key":
			cfg.TLSKey = value
		case "auth.enabled":
			cfg.AuthEnabled = parseBool(value, false)
		case "auth.username":
			cfg.Username = value
		case "auth.password":
			cfg.Password = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, path, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, path, err
	}
	return cfg, path, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Bind) == "" {
		return fmt.Errorf("server.bind không được để trống")
	}
	if c.AuthEnabled && (strings.TrimSpace(c.Username) == "" || c.Password == "" || c.Password == "change-me") {
		return fmt.Errorf("[auth] enabled=true yêu cầu username/password riêng; không được dùng password mặc định change-me")
	}
	if err := c.validateRemoteAuthHost(c.Bind); err != nil {
		return err
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("tls_cert và tls_key phải được cấu hình cùng nhau")
	}
	return nil
}

func (c Config) validateRemoteAuthHost(host string) error {
	if isLoopbackBind(host) {
		return nil
	}
	if !c.AuthEnabled || c.Username == "" || c.Password == "" || c.Password == "change-me" {
		return fmt.Errorf("remote bind %q yêu cầu [auth] enabled=true và username/password riêng", host)
	}
	return nil
}

func (c Config) ValidateListenerAddress(address string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("listener address không hợp lệ %q: %w", address, err)
	}
	return c.validateRemoteAuthHost(strings.Trim(host, "[]"))
}

func (c Config) Address() string { return net.JoinHostPort(c.Bind, strconv.Itoa(c.Port)) }
func (c Config) TLS() bool       { return c.TLSCert != "" && c.TLSKey != "" }

func parseBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func isLoopbackBind(host string) bool {
	h := strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func writeDefault(path string, cfg Config) error {
	content := fmt.Sprintf(`[server]
bind = %s
port = %d
# Set khi bind = 0.0.0.0/:: và muốn URL echo ra dùng IP/hostname truy cập từ xa.
# advertise_host = 192.168.1.20
open_browser = %t
# tls_cert = /absolute/path/to/server.crt
# tls_key = /absolute/path/to/server.key

[auth]
enabled = false
username = %s
password = change-me
`, cfg.Bind, cfg.Port, cfg.OpenBrowser, cfg.Username)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("tạo %s: %w", path, err)
	}
	return nil
}
