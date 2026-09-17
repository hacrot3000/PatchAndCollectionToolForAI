package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestNotificationFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/notifications.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Notification.requestPermission()",
		"document.hidden",
		"Notify: On",
		"taskmenu:session",
		"new Notification",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("notifications module missing %q", want)
		}
	}
	if !strings.Contains(indexHTML, `/featuremods/next.js`) {
		t.Fatalf("index must load progressive next module")
	}
}

func TestGenericFeatureModuleRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/featuremods/notifications.js", nil)
	rr := httptest.NewRecorder()
	staticUI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("content type = %q", got)
	}
}
