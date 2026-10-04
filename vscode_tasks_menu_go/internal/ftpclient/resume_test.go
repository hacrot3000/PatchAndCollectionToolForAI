package ftpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func runResumeFTPServer(t *testing.T, handler func(string, func(string), func() (net.Listener, error), func() (string, error)) error) (string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	done := make(chan error, 1)
	go func() {
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil { done <- err; return }
		defer conn.Close()
		reader := newLineReader(conn)
		write := func(value string) { _, _ = io.WriteString(conn, value+"\r\n") }
		write("220 fake ftp")
		newData := func() (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }
		for {
			line, err := reader()
			if err != nil { done <- err; return }
			switch line {
			case "USER deploy":
				write("331 password required")
			case "PASS secret":
				write("230 logged in")
			case "TYPE I":
				write("200 type set")
			case "QUIT":
				write("221 bye"); done <- nil; return
			default:
				if err := handler(line, write, newData, reader); err != nil {
					done <- err
					return
				}
			}
		}
	}()
	return listener.Addr().String(), done
}

func TestRetrieveFromUsesRESTBeforeRETR(t *testing.T) {
	var dataListener net.Listener
	address, done := runResumeFTPServer(t, func(line string, write func(string), newData func() (net.Listener, error), read func() (string, error)) error {
		switch {
		case line == "REST 3":
			write("350 restart accepted")
			return nil
		case line == "EPSV":
			var err error
			dataListener, err = newData()
			if err != nil { return err }
			port := dataListener.Addr().(*net.TCPAddr).Port
			write(fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", port))
			command, err := read()
			if err != nil { return err }
			if command != "RETR hello.txt" { return fmt.Errorf("unexpected data command %q", command) }
			conn, err := dataListener.Accept()
			_ = dataListener.Close()
			if err != nil { return err }
			write("150 opening data")
			_, _ = io.WriteString(conn, "lo")
			_ = conn.Close()
			write("226 done")
			return nil
		default:
			return fmt.Errorf("unexpected command %q", line)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Dial(ctx, address, "deploy", "secret", time.Second)
	if err != nil { t.Fatal(err) }
	var out strings.Builder
	if err := client.RetrieveFrom(ctx, "hello.txt", 3, &out); err != nil { t.Fatal(err) }
	if out.String() != "lo" { t.Fatalf("out=%q", out.String()) }
	if err := client.Close(); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestStoreFromUsesRESTBeforeSTOR(t *testing.T) {
	var dataListener net.Listener
	var uploaded strings.Builder
	address, done := runResumeFTPServer(t, func(line string, write func(string), newData func() (net.Listener, error), read func() (string, error)) error {
		switch {
		case line == "REST 3":
			write("350 restart accepted")
			return nil
		case line == "EPSV":
			var err error
			dataListener, err = newData()
			if err != nil { return err }
			port := dataListener.Addr().(*net.TCPAddr).Port
			write(fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", port))
			command, err := read()
			if err != nil { return err }
			if command != "STOR hello.txt" { return fmt.Errorf("unexpected data command %q", command) }
			conn, err := dataListener.Accept()
			_ = dataListener.Close()
			if err != nil { return err }
			write("150 opening data")
			data, _ := io.ReadAll(conn)
			uploaded.Write(data)
			_ = conn.Close()
			write("226 done")
			return nil
		default:
			return fmt.Errorf("unexpected command %q", line)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Dial(ctx, address, "deploy", "secret", time.Second)
	if err != nil { t.Fatal(err) }
	if err := client.StoreFrom(ctx, "hello.txt", 3, strings.NewReader("lo")); err != nil { t.Fatal(err) }
	if uploaded.String() != "lo" { t.Fatalf("uploaded=%q", uploaded.String()) }
	if err := client.Close(); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestResumeUnsupportedIsTyped(t *testing.T) {
	address, done := runResumeFTPServer(t, func(line string, write func(string), _ func() (net.Listener, error), _ func() (string, error)) error {
		if line != "REST 5" { return fmt.Errorf("unexpected command %q", line) }
		write("502 REST unsupported")
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Dial(ctx, address, "deploy", "secret", time.Second)
	if err != nil { t.Fatal(err) }
	err = client.RetrieveFrom(ctx, "hello.txt", 5, io.Discard)
	if !errors.Is(err, ErrResumeUnsupported) {
		t.Fatalf("err=%v", err)
	}
	if err := client.Close(); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
}
