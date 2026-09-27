package sshtunnel

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const (
	defaultStartupTimeout = 15 * time.Second
	maxStartupTimeout     = 5 * time.Minute
)

type Spec struct {
	SSHProfile sshprofile.Profile
	RemoteHost string
	RemotePort int
}

type Metadata struct {
	ID           string `json:"id"`
	SSHProfileID string `json:"ssh_profile_id"`
	LocalHost    string `json:"local_host"`
	LocalPort    int    `json:"local_port"`
	RemoteHost   string `json:"remote_host"`
	RemotePort   int    `json:"remote_port"`
	StartedAt    string `json:"started_at"`
}

func normalizeSpec(spec Spec) (Spec, error) {
	profile, err := sshprofile.Normalize(spec.SSHProfile)
	if err != nil {
		return Spec{}, err
	}
	spec.SSHProfile = profile
	spec.RemoteHost = strings.TrimSpace(spec.RemoteHost)
	if spec.RemoteHost == "" {
		return Spec{}, errors.New("SSH tunnel remote host is required")
	}
	if strings.ContainsAny(spec.RemoteHost, " \t/\x00\r\n") {
		return Spec{}, errors.New("SSH tunnel remote host must not contain whitespace or slash")
	}
	if spec.RemotePort < 1 || spec.RemotePort > 65535 {
		return Spec{}, errors.New("SSH tunnel remote port must be between 1 and 65535")
	}
	return spec, nil
}

type portReservation struct {
	listener *net.TCPListener
	port     int
}

func reserveLoopbackPort() (*portReservation, error) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return nil, fmt.Errorf("reserve SSH tunnel loopback port: %w", err)
	}
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.Port < 1 || address.Port > 65535 {
		_ = listener.Close()
		return nil, errors.New("SSH tunnel loopback reservation returned an invalid port")
	}
	return &portReservation{listener: listener, port: address.Port}, nil
}

func (r *portReservation) Port() int {
	if r == nil {
		return 0
	}
	return r.port
}

func (r *portReservation) Endpoint() string {
	if r == nil || r.port == 0 {
		return ""
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(r.port))
}

func (r *portReservation) Close() error {
	if r == nil || r.listener == nil {
		return nil
	}
	err := r.listener.Close()
	r.listener = nil
	return err
}

func normalizeStartupTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return defaultStartupTimeout
	}
	if value > maxStartupTimeout {
		return maxStartupTimeout
	}
	return value
}
