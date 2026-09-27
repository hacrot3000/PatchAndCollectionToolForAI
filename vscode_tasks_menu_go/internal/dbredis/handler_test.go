package dbredis

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type redisHandlerFixture struct {
	listener net.Listener
	mu       sync.Mutex
	commands [][]string
}

func newRedisHandlerFixture(t *testing.T) *redisHandlerFixture {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &redisHandlerFixture{listener: listener}
	t.Cleanup(func() { _ = listener.Close() })
	go fixture.serve()
	return fixture
}

func (f *redisHandlerFixture) endpoint() (string, int) {
	addr := f.listener.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func (f *redisHandlerFixture) commandCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, args := range f.commands {
		if len(args) > 0 && strings.EqualFold(args[0], name) {
			count++
		}
	}
	return count
}

func (f *redisHandlerFixture) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.serveConn(conn)
	}
}

func (f *redisHandlerFixture) serveConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		value, err := ReadValue(reader)
		if err != nil {
			return
		}
		if value.Kind != KindArray {
			_, _ = conn.Write([]byte("-ERR expected array\r\n"))
			continue
		}
		args := make([]string, len(value.Array))
		valid := true
		for i, item := range value.Array {
			if item.Kind != KindBulkString {
				valid = false
				break
			}
			args[i] = item.Text
		}
		if !valid || len(args) == 0 {
			_, _ = conn.Write([]byte("-ERR invalid command\r\n"))
			continue
		}
		f.mu.Lock()
		f.commands = append(f.commands, append([]string(nil), args...))
		f.mu.Unlock()
		f.respond(conn, args)
	}
}

func (f *redisHandlerFixture) respond(conn net.Conn, args []string) {
	command := strings.ToUpper(args[0])
	switch command {
	case "AUTH", "SELECT":
		_, _ = conn.Write([]byte("+OK\r\n"))
	case "PING":
		_, _ = conn.Write([]byte("+PONG\r\n"))
	case "SCAN":
		_, _ = conn.Write([]byte("*2\r\n$1\r\n0\r\n*2\r\n$6\r\nuser:1\r\n$7\r\ncounter\r\n"))
	case "TYPE":
		if len(args) > 1 && args[1] == "counter" {
			_, _ = conn.Write([]byte("+string\r\n"))
		} else {
			_, _ = conn.Write([]byte("+string\r\n"))
		}
	case "TTL":
		_, _ = conn.Write([]byte(":60\r\n"))
	case "GET":
		value := "Alice"
		if len(args) > 1 && args[1] == "counter" {
			value = "42"
		}
		_, _ = fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(value), value)
	default:
		_, _ = conn.Write([]byte("-ERR fixture unsupported command\r\n"))
	}
}

func redisAdapterRequest(t *testing.T, id string, operation dbadapter.Operation, payload interface{}) dbadapter.Envelope {
	t.Helper()
	request, err := dbadapter.NewRequest(id, operation, payload)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func connectRedisHandler(t *testing.T, fixture *redisHandlerFixture) *Handler {
	t.Helper()
	handler, err := NewHandler(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	host, port := fixture.endpoint()
	_, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "connect-1", dbadapter.OpConnect, dbadapter.ConnectPayload{
		Host:     host,
		Port:     port,
		Username: "app",
		Secret:   "top-secret",
		Database: "2",
		ReadOnly: true,
	}))
	if protocolErr != nil {
		t.Fatalf("connect error=%+v", protocolErr)
	}
	return handler
}

func TestHandlerConnectBrowseDescribeExecuteAndDisconnect(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandler(t, fixture)
	defer handler.disconnect()

	if !handler.connected || handler.config.Password != "" || handler.config.Database != 2 {
		t.Fatalf("handler connection state=%+v", handler.config)
	}
	if fixture.commandCount("AUTH") != 1 || fixture.commandCount("SELECT") != 1 || fixture.commandCount("PING") != 1 {
		t.Fatalf("unexpected connect commands=%+v", fixture.commands)
	}

	catalogPayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "catalog-1", dbadapter.OpListCatalogs, nil))
	if protocolErr != nil {
		t.Fatalf("catalog error=%+v", protocolErr)
	}
	catalogs := catalogPayload.([]dbadapter.Object)
	if len(catalogs) != 1 || catalogs[0].Name != "db2" {
		t.Fatalf("catalogs=%+v", catalogs)
	}

	objectsPayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "objects-1", dbadapter.OpListObjects, dbadapter.ListObjectsPayload{Catalog: "db2"}))
	if protocolErr != nil {
		t.Fatalf("objects error=%+v", protocolErr)
	}
	objectsMap := objectsPayload.(map[string]interface{})
	objects := objectsMap["objects"].([]dbadapter.Object)
	if len(objects) != 2 || objects[0].Name != "counter" || objects[1].Name != "user:1" {
		t.Fatalf("objects=%+v", objects)
	}

	detailPayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "describe-1", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{
		Catalog: "db2",
		Name:    "user:1",
	}))
	if protocolErr != nil {
		t.Fatalf("describe error=%+v", protocolErr)
	}
	detail := detailPayload.(map[string]interface{})
	if detail["type"] != "string" || detail["ttl_seconds"] != int64(60) || detail["preview"] != "Alice" {
		t.Fatalf("detail=%+v", detail)
	}

	executePayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "execute-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "GET counter",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("execute error=%+v", protocolErr)
	}
	result := executePayload.(dbadapter.ExecuteResult)
	if len(result.Rows) != 1 || result.Rows[0][0] != "42" {
		t.Fatalf("result=%+v", result)
	}

	beforeSet := fixture.commandCount("SET")
	_, protocolErr = handler.Handle(context.Background(), redisAdapterRequest(t, "execute-write", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SET counter 43",
		MaxRows:   10,
	}))
	if protocolErr == nil || protocolErr.Code != "COMMAND_NOT_ALLOWED" {
		t.Fatalf("write protocol error=%+v", protocolErr)
	}
	if fixture.commandCount("SET") != beforeSet {
		t.Fatal("blocked Redis SET reached the server")
	}

	_, protocolErr = handler.Handle(context.Background(), redisAdapterRequest(t, "disconnect-1", dbadapter.OpDisconnect, nil))
	if protocolErr != nil {
		t.Fatalf("disconnect error=%+v", protocolErr)
	}
	if handler.connected || handler.client != nil {
		t.Fatal("handler remained connected")
	}
}

func TestHandlerRejectsOtherRedisCatalog(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandler(t, fixture)
	defer handler.disconnect()

	_, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "objects-other", dbadapter.OpListObjects, dbadapter.ListObjectsPayload{Catalog: "db3"}))
	if protocolErr == nil || protocolErr.Code != "CATALOG_UNAVAILABLE" {
		t.Fatalf("protocol error=%+v", protocolErr)
	}
}

func TestRedisManifestAndCatalogNaming(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != AdapterID || manifest.Kind != AdapterKind || !manifest.Capabilities.Execute {
		t.Fatalf("manifest=%+v", manifest)
	}
	if redisCatalogName(12) != "db12" {
		t.Fatalf("catalog=%q", redisCatalogName(12))
	}
	if _, err := strconv.Atoi(strings.TrimPrefix(redisCatalogName(12), "db")); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerCommandTimeoutIsBounded(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	host, port := fixture.endpoint()
	handler, err := NewHandler(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	_, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "connect-timeout", dbadapter.OpConnect, dbadapter.ConnectPayload{
		Host: host,
		Port: port,
		Options: map[string]string{
			"command_timeout_seconds": strconv.Itoa(int((2 * time.Second) / time.Second)),
		},
	}))
	if protocolErr != nil {
		t.Fatalf("connect error=%+v", protocolErr)
	}
	handler.disconnect()
}
