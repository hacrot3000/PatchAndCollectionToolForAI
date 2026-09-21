package tlscert

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
)

const (
	autoCertFile = "server.crt"
	autoKeyFile  = "server.key"
	autoLifetime = 397 * 24 * time.Hour
	renewBefore  = 30 * 24 * time.Hour
)

type Result struct {
	CertPath string
	KeyPath  string
	Auto     bool
	Created  bool
}

func FingerprintSHA256(certPath string) (string, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("invalid PEM certificate")
	}
	sum := sha256.Sum256(block.Bytes)
	raw := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.Join(parts, ":"), nil
}

func Resolve(workspace string, cfg config.Config) (Result, error) {
	if !cfg.TLS() {
		return Result{}, nil
	}
	if cfg.CustomTLS() {
		if _, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey); err != nil {
			return Result{}, fmt.Errorf("load configured TLS certificate/key: %w", err)
		}
		return Result{CertPath: cfg.TLSCert, KeyPath: cfg.TLSKey}, nil
	}
	return ensureAuto(workspace, cfg)
}

func AutoDir(workspace string) (string, error) {
	base := strings.TrimSpace(os.Getenv("VSCODE_TASKS_MENU_CONFIG_DIR"))
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user config dir: %w", err)
		}
		base = filepath.Join(base, "vscode_tasks_menu")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(base, "certs", hex.EncodeToString(sum[:8])), nil
}

func ensureAuto(workspace string, cfg config.Config) (Result, error) {
	dir, err := AutoDir(workspace)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create self-signed TLS dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("protect self-signed TLS dir: %w", err)
	}
	certPath := filepath.Join(dir, autoCertFile)
	keyPath := filepath.Join(dir, autoKeyFile)
	dnsNames, ipAddresses := desiredSANs(cfg)

	if reusableAutoPair(certPath, keyPath, dnsNames, ipAddresses, time.Now()) {
		_ = os.Chmod(certPath, 0o600)
		_ = os.Chmod(keyPath, 0o600)
		return Result{CertPath: certPath, KeyPath: keyPath, Auto: true}, nil
	}
	certPEM, keyPEM, err := generateSelfSigned(workspace, dnsNames, ipAddresses, time.Now())
	if err != nil {
		return Result{}, err
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return Result{}, fmt.Errorf("validate generated TLS key pair: %w", err)
	}
	if err := atomicWrite(certPath, certPEM, 0o600); err != nil {
		return Result{}, fmt.Errorf("write self-signed certificate: %w", err)
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return Result{}, fmt.Errorf("write self-signed private key: %w", err)
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		return Result{}, fmt.Errorf("reload generated TLS key pair: %w", err)
	}
	return Result{CertPath: certPath, KeyPath: keyPath, Auto: true, Created: true}, nil
}

func desiredSANs(cfg config.Config) ([]string, []net.IP) {
	dnsSet := map[string]bool{"localhost": true}
	ipSet := map[string]net.IP{
		net.ParseIP("127.0.0.1").String(): net.ParseIP("127.0.0.1"),
		net.ParseIP("::1").String():       net.ParseIP("::1"),
	}
	addHost := func(value string) {
		value = strings.TrimSpace(strings.Trim(value, "[]"))
		if value == "" || value == "0.0.0.0" || value == "::" {
			return
		}
		if ip := net.ParseIP(value); ip != nil {
			ipSet[ip.String()] = ip
			return
		}
		dnsSet[strings.ToLower(value)] = true
	}
	addHost(cfg.Bind)
	addHost(cfg.AdvertiseHost)
	if host, err := os.Hostname(); err == nil {
		addHost(host)
	}
	if cfg.Bind == "0.0.0.0" || cfg.Bind == "::" || cfg.Bind == "[::]" {
		if addrs, err := net.InterfaceAddrs(); err == nil {
			for _, addr := range addrs {
				var raw string
				switch value := addr.(type) {
				case *net.IPNet:
					raw = value.IP.String()
				case *net.IPAddr:
					raw = value.IP.String()
				default:
					raw = strings.Split(addr.String(), "/")[0]
				}
				ip := net.ParseIP(raw)
				if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
					continue
				}
				ipSet[ip.String()] = ip
			}
		}
	}
	dnsNames := make([]string, 0, len(dnsSet))
	for name := range dnsSet {
		dnsNames = append(dnsNames, name)
	}
	sort.Strings(dnsNames)
	ipKeys := make([]string, 0, len(ipSet))
	for key := range ipSet {
		ipKeys = append(ipKeys, key)
	}
	sort.Strings(ipKeys)
	ips := make([]net.IP, 0, len(ipKeys))
	for _, key := range ipKeys {
		ips = append(ips, ipSet[key])
	}
	return dnsNames, ips
}

func reusableAutoPair(certPath, keyPath string, dnsNames []string, ips []net.IP, now time.Time) bool {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil || len(pair.Certificate) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false
	}
	if now.Before(cert.NotBefore) || !cert.NotAfter.After(now.Add(renewBefore)) {
		return false
	}
	dnsSet := make(map[string]bool, len(cert.DNSNames))
	for _, name := range cert.DNSNames {
		dnsSet[strings.ToLower(name)] = true
	}
	for _, name := range dnsNames {
		if !dnsSet[strings.ToLower(name)] {
			return false
		}
	}
	for _, want := range ips {
		found := false
		for _, got := range cert.IPAddresses {
			if got.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func generateSelfSigned(workspace string, dnsNames []string, ips []net.IP, now time.Time) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate TLS private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	name := filepath.Base(filepath.Clean(workspace))
	if name == "." || name == string(filepath.Separator) || strings.TrimSpace(name) == "" {
		name = "workspace"
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "vscode_tasks_menu self-signed",
			Organization: []string{"vscode_tasks_menu"},
			OrganizationalUnit: []string{name},
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(autoLifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create self-signed certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal TLS private key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, nil, fmt.Errorf("encode self-signed TLS material")
	}
	return certPEM, keyPEM, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := bytes.NewReader(data).WriteTo(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}
