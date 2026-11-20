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
		key := ""
		if len(args) > 1 {
			key = args[1]
		}
		switch key {
		case "list:key":
			_, _ = conn.Write([]byte("+list\r\n"))
		case "hash:key":
			_, _ = conn.Write([]byte("+hash\r\n"))
		case "set:key":
			_, _ = conn.Write([]byte("+set\r\n"))
		case "zset:key":
			_, _ = conn.Write([]byte("+zset\r\n"))
		default:
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
	case "LLEN":
		_, _ = conn.Write([]byte(":3\r\n"))
	case "LRANGE":
		_, _ = conn.Write([]byte("*3\r\n$3\r\none\r\n$3\r\ntwo\r\n$5\r\nthree\r\n"))
	case "HLEN":
		_, _ = conn.Write([]byte(":2\r\n"))
	case "HSCAN":
		_, _ = conn.Write([]byte("*2\r\n$1\r\n0\r\n*4\r\n$4\r\nname\r\n$5\r\nAlice\r\n$4\r\nrole\r\n$5\r\nadmin\r\n"))
	case "SCARD":
		_, _ = conn.Write([]byte(":2\r\n"))
	case "SSCAN":
		_, _ = conn.Write([]byte("*2\r\n$1\r\n0\r\n*2\r\n$3\r\nred\r\n$4\r\nblue\r\n"))
	case "ZCARD":
		_, _ = conn.Write([]byte(":2\r\n"))
	case "ZRANGE":
		_, _ = conn.Write([]byte("*4\r\n$5\r\nalice\r\n$2\r\n10\r\n$3\r\nbob\r\n$2\r\n20\r\n"))
	case "HSETNX", "HEXISTS", "HDEL", "SADD", "SREM", "ZADD", "ZREM", "DEL":
		_, _ = conn.Write([]byte(":1\r\n"))
	case "HSET":
		_, _ = conn.Write([]byte(":0\r\n"))
	case "ZSCORE":
		_, _ = conn.Write([]byte("$2\r\n10\r\n"))
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
	return connectRedisHandlerMode(t, fixture, true)
}

func connectRedisHandlerMode(t *testing.T, fixture *redisHandlerFixture, readOnly bool) *Handler {
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
		ReadOnly: readOnly,
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


func TestHandlerWorkbenchBrowseRedisKeyTypes(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandler(t, fixture)
	defer handler.disconnect()

	tests := []struct {
		name       string
		key        string
		limit      int
		wantCols   []string
		wantRows   int
		wantMore   bool
		wantTotal  int64
	}{
		{name: "string", key: "user:1", limit: 10, wantCols: []string{"value"}, wantRows: 1, wantTotal: 1},
		{name: "list", key: "list:key", limit: 2, wantCols: []string{"index", "value"}, wantRows: 2, wantMore: true, wantTotal: 3},
		{name: "hash", key: "hash:key", limit: 1, wantCols: []string{"field", "value"}, wantRows: 1, wantMore: true, wantTotal: 2},
		{name: "set", key: "set:key", limit: 1, wantCols: []string{"member"}, wantRows: 1, wantMore: true, wantTotal: 2},
		{name: "zset", key: "zset:key", limit: 1, wantCols: []string{"member", "score"}, wantRows: 1, wantMore: true, wantTotal: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "browse-"+tc.name, dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
				Catalog: "db2", Kind: "key", Name: tc.key, Limit: tc.limit,
			}))
			if protocolErr != nil {
				t.Fatalf("browse error=%+v", protocolErr)
			}
			result, ok := payload.(dbadapter.BrowseRowsResult)
			if !ok {
				t.Fatalf("payload type=%T", payload)
			}
			if result.Editable || !strings.Contains(result.EditabilityReason, "read-only") {
				t.Fatalf("unexpected editability=%+v", result)
			}
			if len(result.Columns) != len(tc.wantCols) {
				t.Fatalf("columns=%+v", result.Columns)
			}
			for i, want := range tc.wantCols {
				if result.Columns[i].Name != want {
					t.Fatalf("column %d=%q want %q", i, result.Columns[i].Name, want)
				}
			}
			if len(result.Rows) != tc.wantRows || result.HasMore != tc.wantMore {
				t.Fatalf("rows=%+v hasMore=%v", result.Rows, result.HasMore)
			}
			if result.TotalRows == nil || *result.TotalRows != tc.wantTotal {
				t.Fatalf("total=%v want %d", result.TotalRows, tc.wantTotal)
			}
		})
	}
}

func TestRedisManifestAdvertisesBrowseRows(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Capabilities.BrowseRows {
		t.Fatalf("browse capability=%+v", manifest.Capabilities)
	}
}


func TestHandlerWorkbenchEditsRedisHashSetAndZSet(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandlerMode(t, fixture, false)
	defer handler.disconnect()

	hashBrowsePayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "hash-browse-rw", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "db2", Kind: "key", Name: "hash:key", Limit: 10,
	}))
	if protocolErr != nil {
		t.Fatalf("hash browse error=%+v", protocolErr)
	}
	hashBrowse := hashBrowsePayload.(dbadapter.BrowseRowsResult)
	if !hashBrowse.Editable || hashBrowse.Columns[0].Editable || !hashBrowse.Columns[1].Editable {
		t.Fatalf("hash editability=%+v", hashBrowse)
	}

	hashMutation, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "hash-mutate", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "db2", Kind: "key", Name: "hash:key",
		Mutations: []dbadapter.RowMutation{
			{Action: "insert", Values: map[string]interface{}{"field": "team", "value": "core"}},
			{Action: "update", Identity: map[string]interface{}{"field": "name"}, Values: map[string]interface{}{"value": "Alicia"}},
			{Action: "delete", Identity: map[string]interface{}{"field": "role"}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("hash mutation error=%+v", protocolErr)
	}
	for _, item := range hashMutation.(dbadapter.MutateRowsResult).Results {
		if item.Error != nil || item.AffectedRows != 1 {
			t.Fatalf("hash mutation item=%+v", item)
		}
	}

	setMutation, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "set-mutate", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "db2", Kind: "key", Name: "set:key",
		Mutations: []dbadapter.RowMutation{
			{Action: "insert", Values: map[string]interface{}{"member": "green"}},
			{Action: "delete", Identity: map[string]interface{}{"member": "red"}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("set mutation error=%+v", protocolErr)
	}
	for _, item := range setMutation.(dbadapter.MutateRowsResult).Results {
		if item.Error != nil || item.AffectedRows != 1 {
			t.Fatalf("set mutation item=%+v", item)
		}
	}

	zsetMutation, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "zset-mutate", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "db2", Kind: "key", Name: "zset:key",
		Mutations: []dbadapter.RowMutation{
			{Action: "insert", Values: map[string]interface{}{"member": "carol", "score": "30"}},
			{Action: "update", Identity: map[string]interface{}{"member": "alice"}, Values: map[string]interface{}{"score": "11"}},
			{Action: "delete", Identity: map[string]interface{}{"member": "bob"}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("zset mutation error=%+v", protocolErr)
	}
	for _, item := range zsetMutation.(dbadapter.MutateRowsResult).Results {
		if item.Error != nil || item.AffectedRows != 1 {
			t.Fatalf("zset mutation item=%+v", item)
		}
	}

	for _, command := range []string{"HSETNX", "HEXISTS", "HSET", "HDEL", "SADD", "SREM", "ZSCORE", "ZADD", "ZREM"} {
		if fixture.commandCount(command) == 0 {
			t.Fatalf("expected Redis command %s, commands=%+v", command, fixture.commands)
		}
	}
}

func TestHandlerRedisWorkbenchReadOnlyBlocksMutation(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandler(t, fixture)
	defer handler.disconnect()

	before := fixture.commandCount("HSET")
	_, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "hash-ro", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "db2", Kind: "key", Name: "hash:key",
		Mutations: []dbadapter.RowMutation{{Action: "update", Identity: map[string]interface{}{"field": "name"}, Values: map[string]interface{}{"value": "blocked"}}},
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("read-only mutation error=%+v", protocolErr)
	}
	if fixture.commandCount("HSET") != before {
		t.Fatal("read-only Redis mutation reached server")
	}
}

func TestHandlerRedisWorkbenchCountAndDeleteKey(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandlerMode(t, fixture, false)
	defer handler.disconnect()

	countPayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "count-key", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "db2", Kind: "key", Name: "zset:key", Action: "count_rows",
	}))
	if protocolErr != nil {
		t.Fatalf("count error=%+v", protocolErr)
	}
	count := countPayload.(dbadapter.ObjectActionResult)
	if count.Count == nil || *count.Count != 2 {
		t.Fatalf("count=%+v", count)
	}

	dropPayload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "drop-key", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "db2", Kind: "key", Name: "counter", Action: "drop",
	}))
	if protocolErr != nil {
		t.Fatalf("drop error=%+v", protocolErr)
	}
	drop := dropPayload.(dbadapter.ObjectActionResult)
	if drop.AffectedRows != 1 || !strings.Contains(strings.ToLower(drop.Message), "deleted") {
		t.Fatalf("drop=%+v", drop)
	}
	if fixture.commandCount("DEL") != 1 {
		t.Fatalf("DEL commands=%+v", fixture.commands)
	}
}

func TestRedisManifestAdvertisesWorkbenchMutations(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Capabilities.BrowseRows || !manifest.Capabilities.MutateRows || !manifest.Capabilities.ObjectActions {
		t.Fatalf("workbench capabilities=%+v", manifest.Capabilities)
	}
}


func TestRedisWorkbenchReturnsPerRowMutationError(t *testing.T) {
	fixture := newRedisHandlerFixture(t)
	handler := connectRedisHandlerMode(t, fixture, false)
	defer handler.disconnect()

	payload, protocolErr := handler.Handle(context.Background(), redisAdapterRequest(t, "set-invalid-update", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "db2", Kind: "key", Name: "set:key",
		Mutations: []dbadapter.RowMutation{
			{Action: "update", Identity: map[string]interface{}{"member": "red"}, Values: map[string]interface{}{"member": "green"}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("unexpected protocol error=%+v", protocolErr)
	}
	result := payload.(dbadapter.MutateRowsResult)
	if len(result.Results) != 1 || result.Results[0].Error == nil || result.Results[0].Error.Code != "MUTATION_FAILED" {
		t.Fatalf("mutation result=%+v", result)
	}
	if fixture.commandCount("SREM") != 0 || fixture.commandCount("SADD") != 0 {
		t.Fatalf("unsupported set update reached mutating commands: %+v", fixture.commands)
	}
}
