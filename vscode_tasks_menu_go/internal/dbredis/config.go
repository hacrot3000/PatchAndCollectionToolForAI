package dbredis

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	defaultHost           = "127.0.0.1"
	defaultPort           = 6379
	defaultConnectTimeout = 10 * time.Second
	defaultCommandTimeout = 15 * time.Second
	maxTimeout            = 5 * time.Minute
	maxRedisDatabase      = 65535
)

type Config struct {
	Host           string
	Port           int
	Username       string
	Password       string
	Database       int
	ReadOnly       bool
	ConnectTimeout time.Duration
	CommandTimeout time.Duration
}

func ConfigFromConnectPayload(payload dbadapter.ConnectPayload) (Config, error) {
	if strings.TrimSpace(payload.File) != "" {
		return Config{}, errors.New("Redis adapter does not use a database file")
	}
	config := Config{
		Host:           strings.TrimSpace(payload.Host),
		Port:           payload.Port,
		Username:       strings.TrimSpace(payload.Username),
		Password:       payload.Secret,
		ReadOnly:       payload.ReadOnly,
		ConnectTimeout: defaultConnectTimeout,
		CommandTimeout: defaultCommandTimeout,
	}
	if config.Host == "" {
		config.Host = defaultHost
	}
	if config.Port == 0 {
		config.Port = defaultPort
	}
	if config.Port < 1 || config.Port > 65535 {
		return Config{}, errors.New("Redis port must be between 1 and 65535")
	}
	if strings.ContainsAny(config.Host, " \t/\x00\r\n") {
		return Config{}, errors.New("Redis host must not contain whitespace or slash")
	}
	if strings.ContainsAny(config.Username, "\x00\r\n") {
		return Config{}, errors.New("Redis username contains control characters")
	}
	if strings.ContainsAny(config.Password, "\x00\r\n") {
		return Config{}, errors.New("Redis password must not contain NUL or line breaks")
	}
	if database := strings.TrimSpace(payload.Database); database != "" {
		value, err := strconv.Atoi(database)
		if err != nil || value < 0 || value > maxRedisDatabase {
			return Config{}, fmt.Errorf("Redis database must be between 0 and %d", maxRedisDatabase)
		}
		config.Database = value
	}
	for rawKey, rawValue := range payload.Options {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		value := strings.TrimSpace(rawValue)
		switch key {
		case "connect_timeout_seconds":
			timeout, err := parseTimeoutSeconds(value)
			if err != nil {
				return Config{}, fmt.Errorf("Redis connect_timeout_seconds: %w", err)
			}
			config.ConnectTimeout = timeout
		case "command_timeout_seconds":
			timeout, err := parseTimeoutSeconds(value)
			if err != nil {
				return Config{}, fmt.Errorf("Redis command_timeout_seconds: %w", err)
			}
			config.CommandTimeout = timeout
		default:
			return Config{}, fmt.Errorf("unsupported Redis option %q", rawKey)
		}
	}
	if config.Username != "" && config.Password == "" {
		return Config{}, errors.New("Redis username authentication requires a password")
	}
	return config, nil
}

func parseTimeoutSeconds(value string) (time.Duration, error) {
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 1 {
		return 0, errors.New("must be a positive integer")
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > maxTimeout {
		return 0, fmt.Errorf("must not exceed %d seconds", int(maxTimeout/time.Second))
	}
	return timeout, nil
}
