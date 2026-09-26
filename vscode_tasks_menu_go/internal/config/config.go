package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	ProtocolHTTP  = "http"
	ProtocolHTTPS = "https"
)

var sharedProjectIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type Config struct {
	Protocol            string
	Bind                string
	Port                int
	AdvertiseHost       string
	OpenBrowser         bool
	TLSCert             string
	TLSKey              string
	AuthEnabled         bool
	Username            string
	Password            string
	SharedServerEnabled bool
	SharedProjectID     string
	SharedIdentityDB    string
}

func Default() Config {
	return Config{Protocol: ProtocolHTTPS, Bind: "127.0.0.1", Port: 0, OpenBrowser: true, Username: "admin"}
}

func Load(workspace string) (Config, string, error) {
	path, err := projectfiles.Resolve(workspace, "vscode_tasks_menu.ini")
	if err != nil {
		return Config{}, projectfiles.Path(workspace, "vscode_tasks_menu.ini"), err
	}
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
		case "server.protocol":
			cfg.Protocol = strings.ToLower(value)
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
		case "shared_server.enabled":
			cfg.SharedServerEnabled = parseBool(value, false)
		case "shared_server.project_id":
			cfg.SharedProjectID = value
		case "shared_server.identity_db":
			cfg.SharedIdentityDB = value
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
	protocol := strings.ToLower(strings.TrimSpace(c.Protocol))
	switch protocol {
	case ProtocolHTTP, ProtocolHTTPS:
	default:
		return fmt.Errorf("server.protocol phải là http hoặc https")
	}
	if strings.TrimSpace(c.Bind) == "" {
		return fmt.Errorf("server.bind không được để trống")
	}
	if !c.SharedServerEnabled && c.AuthEnabled && (strings.TrimSpace(c.Username) == "" || c.Password == "" || c.Password == "change-me") {
		return fmt.Errorf("[auth] enabled=true yêu cầu username/password riêng; không được dùng password mặc định change-me")
	}
	if c.SharedServerEnabled {
		if !c.TLS() {
			return fmt.Errorf("[shared_server] enabled=true yêu cầu server.protocol=https")
		}
		projectID := strings.TrimSpace(c.SharedProjectID)
		if !sharedProjectIDPattern.MatchString(projectID) {
			return fmt.Errorf("[shared_server] enabled=true yêu cầu project_id hợp lệ (1-128 ký tự: chữ, số, '.', '_' hoặc '-')")
		}
		if identityDB := strings.TrimSpace(c.SharedIdentityDB); identityDB != "" && !filepath.IsAbs(identityDB) {
			return fmt.Errorf("[shared_server] identity_db phải là đường dẫn tuyệt đối khi được cấu hình")
		}
	}
	if err := c.validateRemoteAuthHost(c.Bind); err != nil {
		return err
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("tls_cert và tls_key phải được cấu hình cùng nhau")
	}
	if protocol == ProtocolHTTP && (c.TLSCert != "" || c.TLSKey != "") {
		return fmt.Errorf("tls_cert/tls_key chỉ dùng khi server.protocol=https")
	}
	return nil
}

func (c Config) validateRemoteAuthHost(host string) error {
	if c.SharedServerEnabled {
		if !c.TLS() {
			return fmt.Errorf("shared-server listener requires HTTPS")
		}
		return nil
	}
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
func (c Config) TLS() bool       { return strings.EqualFold(strings.TrimSpace(c.Protocol), ProtocolHTTPS) }
func (c Config) CustomTLS() bool { return c.TLSCert != "" && c.TLSKey != "" }

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
# https là mặc định. Đặt http chỉ khi chủ động muốn tắt TLS.
protocol = %s
bind = %s
port = %d
# Set khi bind = 0.0.0.0/:: và muốn URL/certificate SAN dùng IP/hostname truy cập từ xa.
# advertise_host = 192.168.1.20
open_browser = %t
# Với protocol=https: nếu bỏ trống cả hai dòng dưới, tool tự tạo/reuse self-signed certificate.
# tls_cert = /absolute/path/to/server.crt
# tls_key = /absolute/path/to/server.key

[auth]
enabled = false
username = %s
password = change-me

[shared_server]
# Multi-user shared-project mode. Mặc định tắt để giữ nguyên behavior hiện tại.
enabled = false
# Bắt buộc khi enabled=true. Đây là project identity ổn định trong shared identity DB.
# project_id = my-project
# Optional absolute path tới identity DB dùng chung giữa nhiều TaskDeck process.
# identity_db = /var/lib/taskdeck/identity.db
`, cfg.Protocol, cfg.Bind, cfg.Port, cfg.OpenBrowser, cfg.Username)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("tạo %s: %w", path, err)
	}
	return nil
}
