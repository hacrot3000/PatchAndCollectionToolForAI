package dbmongo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func writeMongoshHandlerFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mongosh")
	marker := resultMarker
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo '2.5.1-fixture'; exit 0; fi\n" +
		"file=''\nprev=''\n" +
		"for arg in \"$@\"; do if [ \"$prev\" = \"--file\" ]; then file=\"$arg\"; break; fi; prev=\"$arg\"; done\n" +
		"if grep -q 'listDatabases' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"databases\":[{\"name\":\"admin\"},{\"name\":\"main\"}]}}'\n" +
		"elif grep -q 'getCollectionInfos({name:__name})' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"info\":[{\"name\":\"users\",\"type\":\"collection\"}],\"indexes\":[{\"name\":\"_id_\"}]}}'\n" +
		"elif grep -q 'getCollectionInfos();' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":[{\"name\":\"users\",\"type\":\"collection\"},{\"name\":\"active_users\",\"type\":\"view\"}]}'\n" +
		"elif grep -q '\\.find(' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"documents\":[{\"_id\":1,\"name\":\"Alice\"}],\"truncated\":false}}'\n" +
		"elif grep -q 'ping:1' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"ok\":true}}'\n" +
		"else\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":false,\"error\":\"fixture operation not recognized\"}'\n" +
		"fi\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func mongoAdapterRequest(t *testing.T, id string, operation dbadapter.Operation, payload interface{}) dbadapter.Envelope {
	t.Helper()
	request, err := dbadapter.NewRequest(id, operation, payload)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func connectMongoHandler(t *testing.T) *Handler {
	t.Helper()
	writeMongoshHandlerFixture(t)
	handler, err := NewHandler(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	_, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "connect-1", dbadapter.OpConnect, dbadapter.ConnectPayload{
		Host:     "db.example.com",
		Port:     27017,
		Username: "app",
		Secret:   "top-secret",
		Database: "main",
		ReadOnly: true,
	}))
	if protocolErr != nil {
		t.Fatalf("connect error=%+v", protocolErr)
	}
	return handler
}

func TestHandlerConnectBrowseDescribeExecuteAndDisconnect(t *testing.T) {
	handler := connectMongoHandler(t)

	if !handler.connected || handler.client.Path == "" || handler.config.Password != "top-secret" {
		t.Fatalf("handler state=%+v config=%+v", handler, handler.config)
	}
	if _, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "ping-1", dbadapter.OpPing, nil)); protocolErr != nil {
		t.Fatalf("ping error=%+v", protocolErr)
	}

	catalogPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "catalog-1", dbadapter.OpListCatalogs, nil))
	if protocolErr != nil {
		t.Fatalf("catalog error=%+v", protocolErr)
	}
	catalogMap := catalogPayload.(map[string]interface{})
	catalogs := catalogMap["catalogs"].([]dbadapter.Object)
	if len(catalogs) != 2 || catalogs[1].Name != "main" {
		t.Fatalf("catalogs=%+v", catalogPayload)
	}

	objectsPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "objects-1", dbadapter.OpListObjects, dbadapter.ListObjectsPayload{Catalog: "main"}))
	if protocolErr != nil {
		t.Fatalf("objects error=%+v", protocolErr)
	}
	objectsMap := objectsPayload.(map[string]interface{})
	objects := objectsMap["objects"].([]dbadapter.Object)
	if len(objects) != 2 || objects[0].Name != "users" || objects[1].Kind != "view" {
		t.Fatalf("objects=%+v", objects)
	}

	detailPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "describe-1", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{Catalog: "main", Name: "users"}))
	if protocolErr != nil {
		t.Fatalf("describe error=%+v", protocolErr)
	}
	detail := detailPayload.(map[string]interface{})
	if detail["name"] != "users" || detail["catalog"] != "main" {
		t.Fatalf("detail=%+v", detail)
	}

	executePayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "execute-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "{\"op\":\"find\",\"collection\":\"users\",\"filter\":{\"active\":true}}",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("execute error=%+v", protocolErr)
	}
	result := executePayload.(dbadapter.ExecuteResult)
	if len(result.Rows) != 1 {
		t.Fatalf("result=%+v", result)
	}
	document := result.Rows[0][0].(map[string]interface{})
	if document["name"] != "Alice" {
		t.Fatalf("document=%+v", document)
	}

	_, protocolErr = handler.Handle(context.Background(), mongoAdapterRequest(t, "execute-js", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "{\"op\":\"find\",\"collection\":\"users\",\"filter\":{\"$where\":\"sleep(1000)\"}}",
		MaxRows:   10,
	}))
	if protocolErr == nil || protocolErr.Code != "INVALID_QUERY" {
		t.Fatalf("server-side JS was not rejected: %+v", protocolErr)
	}

	_, protocolErr = handler.Handle(context.Background(), mongoAdapterRequest(t, "disconnect-1", dbadapter.OpDisconnect, nil))
	if protocolErr != nil {
		t.Fatalf("disconnect error=%+v", protocolErr)
	}
	if handler.connected || handler.config.Password != "" {
		t.Fatalf("disconnect retained state: %+v", handler.config)
	}
}

func TestMongoOperationBodiesKeepNamesInsideStringBoundary(t *testing.T) {
	body, err := describeObjectOperationBody("main", "users\"; throw new Error('boom');//")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "users\"; throw new Error('boom');//") {
		t.Fatalf("raw collection name escaped script boundary: %s", body)
	}
	if !strings.Contains(body, "getCollectionInfos") {
		t.Fatalf("body=%s", body)
	}
}
