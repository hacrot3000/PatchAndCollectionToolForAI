package session

import "testing"

func TestProtocolArtifactStateAcceptsSafeProjectArtifacts(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"artifact","seq":2,"artifact_kind":"fail_handoff_zip","path":"artifacts/ptv_to_ai/FAIL_HANDOFF_demo.zip","primary":true,"item_name":"demo.zip","item_kind":"PATCH","index":1,"total":2}`))
	if s.protocol.Error != "" {
		t.Fatalf("safe artifact rejected: %s", s.protocol.Error)
	}
	if len(s.protocol.Artifacts) != 1 {
		t.Fatalf("artifacts=%#v", s.protocol.Artifacts)
	}
	got := s.protocol.Artifacts[0]
	if got.Path != "artifacts/ptv_to_ai/FAIL_HANDOFF_demo.zip" || got.ArtifactKind != "fail_handoff_zip" || !got.Primary || got.Index != 1 {
		t.Fatalf("unexpected artifact state: %#v", got)
	}
}

func TestProtocolArtifactStateRejectsUnsafePaths(t *testing.T) {
	for _, path := range []string{
		"../artifacts/x.zip",
		"/tmp/x.zip",
		"artifacts/../x.zip",
		"artifacts//x.zip",
		"artifacts\\x.zip",
		"patchs/x.zip",
	} {
		s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
		line := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"artifact","seq":2,"artifact_kind":"collect_result_zip","path":"` + path + `"}`)
		s.applyProtocolLine(line)
		if len(s.protocol.Artifacts) != 0 || s.protocol.Error == "" {
			t.Fatalf("unsafe artifact path accepted: %q state=%#v", path, s.protocol)
		}
	}
}

func TestProtocolArtifactStateIsDeduplicatedAndBounded(t *testing.T) {
	items := []ProtocolArtifactState{}
	first := ProtocolArtifactState{ArtifactKind: "collect_result_zip", Path: "artifacts/a.zip"}
	items = upsertProtocolArtifact(items, first)
	items = upsertProtocolArtifact(items, ProtocolArtifactState{ArtifactKind: "collect_result_zip", Path: "artifacts/a.zip", Primary: true})
	if len(items) != 1 || !items[0].Primary {
		t.Fatalf("artifact dedupe failed: %#v", items)
	}
	for i := 0; i < maxProtocolArtifacts+20; i++ {
		items = upsertProtocolArtifact(items, ProtocolArtifactState{ArtifactKind: "x", Path: "artifacts/file-" + string(rune(i+1000))})
	}
	if len(items) != maxProtocolArtifacts {
		t.Fatalf("artifact bound=%d want %d", len(items), maxProtocolArtifacts)
	}
}
