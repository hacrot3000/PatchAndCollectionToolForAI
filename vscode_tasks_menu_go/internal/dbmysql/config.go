package dbmysql

import (
	"fmt"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	defaultHost           = "127.0.0.1"
	defaultPort           = 3306
	defaultCharset        = "utf8mb4"
	defaultConnectTimeout = 10
	maxConnectTimeout     = 300
)

type Config struct {
	Host                  string
	Port                  int
	Username              string
	Database              string
	Secret                string
	ReadOnly              bool
	Charset               string
	ConnectTimeoutSeconds int
}

func ConfigFromConnectPayload(payload dbadapter.ConnectPayload) (Config, error) {
	config := Config{
		Host:                  strings.TrimSpace(payload.Host),
		Port:                  payload.Port,
		Username:              strings.TrimSpace(payload.Username),
		Database:              strings.TrimSpace(payload.Database),
		Secret:                payload.Secret,
		ReadOnly:              payload.ReadOnly,
		Charset:               defaultCharset,
		ConnectTimeoutSeconds: defaultConnectTimeout,
	}
	if config.Host == "" {
		config.Host = defaultHost
	}
	if config.Port == 0 {
		config.Port = defaultPort
	}
	if config.Port < 1 || config.Port > 65535 {
		return Config{}, fmt.Errorf("MySQL port must be between 1 and 65535")
	}
	if strings.ContainsAny(config.Host, " \t/\x00\r\n") {
		return Config{}, fmt.Errorf("MySQL host must not contain whitespace or slash")
	}
	if strings.ContainsAny(config.Username, "\x00\r\n") {
		return Config{}, fmt.Errorf("MySQL username contains control characters")
	}
	if strings.ContainsAny(config.Database, "\x00\r\n") {
		return Config{}, fmt.Errorf("MySQL database name contains control characters")
	}
	if strings.ContainsAny(config.Secret, "\x00\r\n") {
		return Config{}, fmt.Errorf("MySQL password must not contain NUL or line breaks")
	}

	for rawKey, rawValue := range payload.Options {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		value := strings.TrimSpace(rawValue)
		switch key {
		case "charset":
			if value == "" || len(value) > 64 {
				return Config{}, fmt.Errorf("MySQL charset is invalid")
			}
			for _, r := range value {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') || r == '_' || r == '-' {
					continue
				}
				return Config{}, fmt.Errorf("MySQL charset contains unsupported character %q", r)
			}
			config.Charset = value
		case "connect_timeout_seconds":
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds < 1 || seconds > maxConnectTimeout {
				return Config{}, fmt.Errorf("MySQL connect_timeout_seconds must be between 1 and %d", maxConnectTimeout)
			}
			config.ConnectTimeoutSeconds = seconds
		default:
			return Config{}, fmt.Errorf("unsupported MySQL option %q", rawKey)
		}
	}
	return config, nil
}

func clientArgs(config Config, credentials *credentialFile) []string {
	args := make([]string, 0, 10)
	if credentials != nil {
		args = append(args, credentials.Arg())
	}
	args = append(args,
		"--protocol=tcp",
		"--batch",
		"--xml",
		"--default-character-set="+config.Charset,
		"--connect-timeout="+strconv.Itoa(config.ConnectTimeoutSeconds),
		"--host="+config.Host,
		"--port="+strconv.Itoa(config.Port),
	)
	if config.Username != "" {
		args = append(args, "--user="+config.Username)
	}
	if config.Database != "" {
		args = append(args, "--database="+config.Database)
	}
	if config.ReadOnly {
		args = append(args, "--init-command=SET SESSION TRANSACTION READ ONLY")
	}
	return args
}
