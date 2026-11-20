package dbmongo

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	defaultHost           = "127.0.0.1"
	defaultPort           = 27017
	defaultConnectTimeout = 10 * time.Second
	maxConnectTimeout     = 5 * time.Minute
)

type Config struct {
	Host           string
	Port           int
	Username       string
	Password       string
	Database       string
	AuthSource     string
	ReadOnly       bool
	TLS            bool
	ConnectTimeout time.Duration
}

func ConfigFromConnectPayload(payload dbadapter.ConnectPayload) (Config, error) {
	if strings.TrimSpace(payload.File) != "" {
		return Config{}, errors.New("MongoDB adapter does not use a database file")
	}
	config := Config{
		Host:           strings.TrimSpace(payload.Host),
		Port:           payload.Port,
		Username:       strings.TrimSpace(payload.Username),
		Password:       payload.Secret,
		Database:       strings.TrimSpace(payload.Database),
		ReadOnly:       payload.ReadOnly,
		ConnectTimeout: defaultConnectTimeout,
	}
	if config.Host == "" {
		config.Host = defaultHost
	}
	if config.Port == 0 {
		config.Port = defaultPort
	}
	if config.Port < 1 || config.Port > 65535 {
		return Config{}, errors.New("MongoDB port must be between 1 and 65535")
	}
	if strings.ContainsAny(config.Host, " \t/\x00\r\n") {
		return Config{}, errors.New("MongoDB host must not contain whitespace or slash")
	}
	if strings.ContainsAny(config.Username, "\x00\r\n") {
		return Config{}, errors.New("MongoDB username contains control characters")
	}
	if strings.ContainsAny(config.Password, "\x00\r\n") {
		return Config{}, errors.New("MongoDB password must not contain NUL or line breaks")
	}
	if config.Username == "" && config.Password != "" {
		return Config{}, errors.New("MongoDB password requires a username")
	}
	if err := validateDatabaseName(config.Database, false); err != nil {
		return Config{}, err
	}

	for rawKey, rawValue := range payload.Options {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		value := strings.TrimSpace(rawValue)
		switch key {
		case "auth_source":
			if err := validateDatabaseName(value, true); err != nil {
				return Config{}, fmt.Errorf("MongoDB auth_source: %w", err)
			}
			config.AuthSource = value
		case "connect_timeout_seconds":
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds < 1 {
				return Config{}, errors.New("MongoDB connect_timeout_seconds must be a positive integer")
			}
			timeout := time.Duration(seconds) * time.Second
			if timeout > maxConnectTimeout {
				return Config{}, fmt.Errorf("MongoDB connect_timeout_seconds must not exceed %d", int(maxConnectTimeout/time.Second))
			}
			config.ConnectTimeout = timeout
		case "tls":
			switch strings.ToLower(value) {
			case "true", "1", "yes":
				config.TLS = true
			case "false", "0", "no", "":
				config.TLS = false
			default:
				return Config{}, errors.New("MongoDB tls must be true or false")
			}
		default:
			return Config{}, fmt.Errorf("unsupported MongoDB option %q", rawKey)
		}
	}
	return config, nil
}

func (c Config) URI() (string, error) {
	if c.Host == "" || c.Port < 1 || c.Port > 65535 {
		return "", errors.New("MongoDB connection endpoint is invalid")
	}
	u := &url.URL{
		Scheme: "mongodb",
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
	}
	if c.Username != "" {
		u.User = url.UserPassword(c.Username, c.Password)
	}
	if c.Database != "" {
		u.Path = "/" + c.Database
	}
	query := url.Values{}
	if c.AuthSource != "" {
		query.Set("authSource", c.AuthSource)
	}
	timeoutMS := c.ConnectTimeout.Milliseconds()
	if timeoutMS > 0 {
		query.Set("connectTimeoutMS", strconv.FormatInt(timeoutMS, 10))
		query.Set("serverSelectionTimeoutMS", strconv.FormatInt(timeoutMS, 10))
	}
	if c.TLS {
		query.Set("tls", "true")
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func validateDatabaseName(value string, required bool) error {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return errors.New("database name is required")
	}
	if len(value) > 128 {
		return errors.New("database name exceeds 128 bytes")
	}
	if strings.ContainsAny(value, "\x00/\\.\"$*<>:|?\r\n") {
		return errors.New("database name contains unsupported characters")
	}
	return nil
}
