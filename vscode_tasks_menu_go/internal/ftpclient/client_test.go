package ftpclient

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseEPSVAndPASV(t *testing.T) {
	port, err := parseEPSVPort("Entering Extended Passive Mode (|||49152|)")
	if err != nil || port != 49152 {
		t.Fatalf("EPSV port=%d err=%v", port, err)
	}
	port, err = parsePASVPort("Entering Passive Mode (10,0,0,1,192,0)")
	if err != nil || port != 49152 {
		t.Fatalf("PASV port=%d err=%v", port, err)
	}
}

func TestParseMLSD(t *testing.T) {
	entries, err := ParseMLSD(strings.Join([]string{
		"type=cdir;modify=20260930120000; .",
		"type=pdir;modify=20260929120000; ..",
		"type=file;size=123;modify=20260930120102; app config.txt",
		"type=dir;modify=20260929120000; uploads",
	}, "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%#v", entries)
	}
	if entries[0].Name != "app config.txt" || entries[0].Size != 123 || entries[0].Type != "file" {
		t.Fatalf("file=%#v", entries[0])
	}
	if entries[1].Name != "uploads" || entries[1].Type != "directory" {
		t.Fatalf("dir=%#v", entries[1])
	}
}

func TestParseLISTUnixAndDOS(t *testing.T) {
	entries, err := ParseLIST(strings.Join([]string{
		"-rw-r--r-- 1 1000 1000 42 Sep 30 12:00 file with spaces.txt",
		"drwxr-xr-x 2 1000 1000 4096 Sep 29 2026 uploads",
		"09-30-26  01:22PM       1234 windows file.txt",
		"09-29-26  12:00PM       <DIR> windows-dir",
	}, "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("entries=%#v", entries)
	}
	if entries[0].Name != "file with spaces.txt" || entries[2].Name != "windows file.txt" || entries[3].Type != "directory" {
		t.Fatalf("entries=%#v", entries)
	}
}

func TestClientLoginEPSVListAndRetrieve(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		reader := newLineReader(conn)
		write := func(value string) { _, _ = io.WriteString(conn, value+"\r\n") }
		write("220 fake ftp")
		for {
			line, err := reader()
			if err != nil {
				done <- err
				return
			}
			switch {
			case line == "USER deploy":
				write("331 password required")
			case line == "PASS secret":
				write("230 logged in")
			case line == "TYPE I":
				write("200 type set")
			case line == "EPSV":
				dataListener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil { done <- err; return }
				port := dataListener.Addr().(*net.TCPAddr).Port
				write(fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", port))
				command, err := reader()
				if err != nil { dataListener.Close(); done <- err; return }
				dataConn, err := dataListener.Accept()
				dataListener.Close()
				if err != nil { done <- err; return }
				switch {
				case command == "MLSD":
					write("150 opening data")
					_, _ = io.WriteString(dataConn, "type=file;size=5;modify=20260930120102; hello.txt\r\n")
					dataConn.Close()
					write("226 done")
				case command == "RETR hello.txt":
					write("150 opening data")
					_, _ = io.WriteString(dataConn, "hello")
					dataConn.Close()
					write("226 done")
				default:
					dataConn.Close()
					done <- fmt.Errorf("unexpected data command %q", command)
					return
				}
			case line == "QUIT":
				write("221 bye")
				done <- nil
				return
			default:
				done <- fmt.Errorf("unexpected command %q", line)
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Dial(ctx, listener.Addr().String(), "deploy", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := client.List(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "hello.txt" {
		t.Fatalf("entries=%#v", entries)
	}
	var out strings.Builder
	if err := client.Retrieve(ctx, "hello.txt", &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello" {
		t.Fatalf("download=%q", out.String())
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func newLineReader(conn net.Conn) func() (string, error) {
	reader := make([]byte, 0, 256)
	buf := make([]byte, 1)
	return func() (string, error) {
		reader = reader[:0]
		for {
			n, err := conn.Read(buf)
			if n == 1 {
				if buf[0] == '\n' {
					return strings.TrimSuffix(string(reader), "\r"), nil
				}
				reader = append(reader, buf[0])
			}
			if err != nil {
				return "", err
			}
		}
	}
}


func testFTPSCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil { t.Fatal(err) }
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"},
		NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil { t.Fatal(err) }
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	parsed, err := x509.ParseCertificate(der)
	if err != nil { t.Fatal(err) }
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return cert, pool
}

func TestClientExplicitFTPSProtectsControlAndData(t *testing.T) {
	cert, roots := testFTPSCertificate(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer listener.Close()
	serverTLS := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	done := make(chan error, 1)

	go func() {
		raw, err := listener.Accept()
		if err != nil { done <- err; return }
		defer raw.Close()
		read := newLineReader(raw)
		write := func(conn net.Conn, value string) { _, _ = io.WriteString(conn, value+"\r\n") }
		write(raw, "220 explicit ftps fixture")
		line, err := read()
		if err != nil { done <- err; return }
		if line != "AUTH TLS" { done <- fmt.Errorf("expected AUTH TLS, got %q", line); return }
		write(raw, "234 proceed with negotiation")

		secure := tls.Server(raw, serverTLS)
		if err := secure.Handshake(); err != nil { done <- err; return }
		read = newLineReader(secure)
		for {
			line, err := read()
			if err != nil { done <- err; return }
			switch line {
			case "USER deploy":
				write(secure, "331 password required")
			case "PASS secret":
				write(secure, "230 logged in")
			case "PBSZ 0":
				write(secure, "200 PBSZ=0")
			case "PROT P":
				write(secure, "200 protection private")
			case "TYPE I":
				write(secure, "200 type set")
			case "EPSV":
				dataListener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil { done <- err; return }
				port := dataListener.Addr().(*net.TCPAddr).Port
				write(secure, fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", port))
				dataRaw, err := dataListener.Accept()
				dataListener.Close()
				if err != nil { done <- err; return }
				dataTLS := tls.Server(dataRaw, serverTLS)
				if err := dataTLS.Handshake(); err != nil { dataRaw.Close(); done <- err; return }
				command, err := read()
				if err != nil { dataTLS.Close(); done <- err; return }
				if command != "MLSD" {
					dataTLS.Close(); done <- fmt.Errorf("expected MLSD, got %q", command); return
				}
				write(secure, "150 opening protected data")
				_, _ = io.WriteString(dataTLS, "type=file;size=5;modify=20260930120102; hello.txt\r\n")
				_ = dataTLS.Close()
				write(secure, "226 protected transfer complete")
			case "QUIT":
				write(secure, "221 bye")
				done <- nil
				return
			default:
				done <- fmt.Errorf("unexpected protected command %q", line)
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := DialTLS(ctx, listener.Addr().String(), "deploy", "secret", time.Second, TLSOptions{
		Mode: TLSExplicit, ServerName: "localhost", RootCAs: roots,
	})
	if err != nil { t.Fatal(err) }
	entries, err := client.List(ctx, ".")
	if err != nil { t.Fatal(err) }
	if len(entries) != 1 || entries[0].Name != "hello.txt" {
		t.Fatalf("entries=%#v", entries)
	}
	if err := client.Close(); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestClientImplicitFTPSVerifiesServerCertificate(t *testing.T) {
	cert, _ := testFTPSCertificate(t)
	listener, err := tls.Listen("tcp4", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12,
	})
	if err != nil { t.Fatal(err) }
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil { return }
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 implicit ftps fixture\r\n")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = DialTLS(ctx, listener.Addr().String(), "deploy", "secret", time.Second, TLSOptions{
		Mode: TLSImplicit, ServerName: "localhost",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Fatalf("expected certificate verification failure, got %v", err)
	}
	<-done
}
