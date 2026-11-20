package dbredis

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startRedisFixture(t *testing.T, handler func(net.Conn)) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	addr := listener.Addr().(*net.TCPAddr)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handler(conn)
		}
	}()
	return "127.0.0.1", addr.Port
}

func commandArgsFromValue(t *testing.T, value Value) []string {
	t.Helper()
	if value.Kind != KindArray {
		t.Fatalf("command kind=%q want array", value.Kind)
	}
	out := make([]string, len(value.Array))
	for i, item := range value.Array {
		if item.Kind != KindBulkString {
			t.Fatalf("command item %d kind=%q", i, item.Kind)
		}
		out[i] = item.Text
	}
	return out
}

func TestClientDoRoundTrip(t *testing.T) {
	host, port := startRedisFixture(t, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		value, err := ReadValue(reader)
		if err != nil {
			return
		}
		args := commandArgsFromValue(t, value)
		if len(args) == 1 && args[0] == "PING" {
			_, _ = conn.Write([]byte("+PONG\r\n"))
			return
		}
		_, _ = conn.Write([]byte("-ERR unexpected\r\n"))
	})
	client, err := Dial(context.Background(), Config{
		Host: host, Port: port,
		ConnectTimeout: time.Second, CommandTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	value, err := client.Do(context.Background(), "PING")
	if err != nil {
		t.Fatal(err)
	}
	if value.Kind != KindSimpleString || value.Text != "PONG" {
		t.Fatalf("value=%+v", value)
	}
}

func TestClientConvertsRedisErrors(t *testing.T) {
	host, port := startRedisFixture(t, func(conn net.Conn) {
		defer conn.Close()
		_, _ = ReadValue(bufio.NewReader(conn))
		_, _ = conn.Write([]byte("-NOAUTH authentication required\r\n"))
	})
	client, err := Dial(context.Background(), Config{
		Host: host, Port: port,
		ConnectTimeout: time.Second, CommandTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.Do(context.Background(), "GET", "x")
	if err == nil || !strings.Contains(err.Error(), "NOAUTH") {
		t.Fatalf("error=%v", err)
	}
}

func TestClientCommandContextCancellationInterruptsRead(t *testing.T) {
	host, port := startRedisFixture(t, func(conn net.Conn) {
		defer conn.Close()
		_, _ = ReadValue(bufio.NewReader(conn))
		time.Sleep(5 * time.Second)
	})
	client, err := Dial(context.Background(), Config{
		Host: host, Port: port,
		ConnectTimeout: time.Second, CommandTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = client.Do(ctx, "PING")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error=%v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("cancellation too slow: %v", time.Since(started))
	}
}

func TestDialUsesConfiguredEndpoint(t *testing.T) {
	host, port := startRedisFixture(t, func(conn net.Conn) {
		_ = conn.Close()
	})
	client, err := Dial(context.Background(), Config{
		Host: host, Port: port,
		ConnectTimeout: time.Second, CommandTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
	_ = client.Close()
	if portText := strconv.Itoa(port); portText == "" {
		t.Fatal("invalid fixture port")
	}
}
