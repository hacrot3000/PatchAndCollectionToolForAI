package sshtunnel

import (
	"net"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func testSSHProfile() sshprofile.Profile {
	return sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "jump.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	}
}

func TestNormalizeSpec(t *testing.T) {
	spec, err := normalizeSpec(Spec{
		SSHProfile: testSSHProfile(),
		RemoteHost: " db.internal ",
		RemotePort: 3306,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.RemoteHost != "db.internal" || spec.RemotePort != 3306 {
		t.Fatalf("spec=%+v", spec)
	}
}

func TestNormalizeSpecRejectsUnsafeRemoteEndpoint(t *testing.T) {
	for _, spec := range []Spec{
		{SSHProfile: testSSHProfile(), RemoteHost: "", RemotePort: 3306},
		{SSHProfile: testSSHProfile(), RemoteHost: "db host", RemotePort: 3306},
		{SSHProfile: testSSHProfile(), RemoteHost: "db/host", RemotePort: 3306},
		{SSHProfile: testSSHProfile(), RemoteHost: "db.internal", RemotePort: 0},
		{SSHProfile: testSSHProfile(), RemoteHost: "db.internal", RemotePort: 70000},
	} {
		if _, err := normalizeSpec(spec); err == nil {
			t.Fatalf("unsafe spec unexpectedly accepted: %+v", spec)
		}
	}
}

func TestReserveLoopbackPortHoldsPortUntilReleased(t *testing.T) {
	reservation, err := reserveLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	defer reservation.Close()

	if reservation.Port() < 1 || reservation.Endpoint() == "" {
		t.Fatalf("reservation=%+v endpoint=%q", reservation, reservation.Endpoint())
	}
	if _, err := net.Listen("tcp4", reservation.Endpoint()); err == nil {
		t.Fatal("reserved loopback port was bindable by a second listener")
	}

	endpoint := reservation.Endpoint()
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", endpoint)
	if err != nil {
		t.Fatalf("released loopback port could not be rebound: %v", err)
	}
	_ = listener.Close()
}

func TestNormalizeStartupTimeout(t *testing.T) {
	if got := normalizeStartupTimeout(0); got != defaultStartupTimeout {
		t.Fatalf("default timeout=%v", got)
	}
	if got := normalizeStartupTimeout(2 * time.Second); got != 2*time.Second {
		t.Fatalf("explicit timeout=%v", got)
	}
	if got := normalizeStartupTimeout(10 * time.Minute); got != maxStartupTimeout {
		t.Fatalf("capped timeout=%v", got)
	}
}
