package broker

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

const maxBrokerInput = 1 << 20

type sessionAPI struct {
	manager *session.Manager
}

func registerSessionRoutes(mux *http.ServeMux, manager *session.Manager) {
	api := &sessionAPI{manager: manager}
	mux.HandleFunc("/v1/sessions", api.sessionsRoot)
	mux.HandleFunc("/v1/sessions/", api.sessionItem)
}

func executionFromWire(spec ExecutionSpec) tasks.Execution {
	return tasks.Execution{
		TaskID: spec.TaskID, Label: spec.Label, Detail: spec.Detail,
		Command: spec.Command, Args: append([]string(nil), spec.Args...),
		Cwd: spec.Cwd, Env: append([]string(nil), spec.Env...), Preview: spec.Preview,
		ProtocolEvents:   spec.ProtocolEvents,
		ProtocolCommands: spec.ProtocolCommands,
		SessionKind:      spec.SessionKind, OwnerUserID: spec.OwnerUserID, ProjectID: spec.ProjectID,
	}
}

func executionToWire(spec tasks.Execution) ExecutionSpec {
	return ExecutionSpec{
		TaskID: spec.TaskID, Label: spec.Label, Detail: spec.Detail,
		Command: spec.Command, Args: append([]string(nil), spec.Args...),
		Cwd: spec.Cwd, Env: append([]string(nil), spec.Env...), Preview: spec.Preview,
		ProtocolEvents:   spec.ProtocolEvents,
		ProtocolCommands: spec.ProtocolCommands,
		SessionKind:      spec.SessionKind, OwnerUserID: spec.OwnerUserID, ProjectID: spec.ProjectID,
	}
}

func (a *sessionAPI) sessionsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeBrokerJSON(w, http.StatusOK, map[string]any{"sessions": a.manager.List()})
	case http.MethodPost:
		var wire ExecutionSpec
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&wire); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(wire.Command) == "" || strings.TrimSpace(wire.Cwd) == "" {
			http.Error(w, "command and cwd are required", http.StatusBadRequest)
			return
		}
		meta, err := a.manager.Start(executionFromWire(wire))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeBrokerJSON(w, http.StatusCreated, meta)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *sessionAPI) sessionItem(w http.ResponseWriter, r *http.Request) {
	tail := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/")
	parts := strings.Split(tail, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			meta, ok := a.manager.Metadata(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeBrokerJSON(w, http.StatusOK, meta)
		case http.MethodDelete:
			if err := a.manager.Remove(id); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch parts[1] {
	case "input":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, maxBrokerInput+1))
		if err != nil || len(data) > maxBrokerInput {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		if err := a.manager.Input(id, data); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "title":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Title string `json:"title"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		meta, err := a.manager.SetTitle(id, req.Title)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeBrokerJSON(w, http.StatusOK, meta)
	case "resize":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ResizeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := a.manager.Resize(id, req.Rows, req.Cols); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "stop":
		a.simpleAction(w, r, id, a.manager.Stop)
	case "kill":
		a.simpleAction(w, r, id, a.manager.Kill)
	case "clear":
		a.simpleAction(w, r, id, a.manager.Clear)
	case "command":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, maxBrokerInput+1))
		if err != nil || len(data) > maxBrokerInput {
			http.Error(w, "invalid protocol command", http.StatusBadRequest)
			return
		}
		if err := a.manager.ProtocolCommand(id, data); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "protocol":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		state, err := a.manager.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeBrokerJSON(w, http.StatusOK, state)
	case "cwd":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cwd, err := a.manager.CurrentCwd(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeBrokerJSON(w, http.StatusOK, map[string]string{"cwd": cwd})
	case "stream":
		a.stream(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (a *sessionAPI) simpleAction(w http.ResponseWriter, r *http.Request, id string, action func(string) error) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := action(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	meta, ok := a.manager.Metadata(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeBrokerJSON(w, http.StatusOK, meta)
}

func (a *sessionAPI) stream(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	backlog, stream, unsubscribe, err := a.manager.Subscribe(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer unsubscribe()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	if err := writeFrame(w, backlog); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-stream:
			if !ok {
				return
			}
			if err := writeFrame(w, data); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func writeFrame(w io.Writer, data []byte) error {
	if uint64(len(data)) > uint64(^uint32(0)) {
		return fmt.Errorf("broker frame too large")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	_, err := w.Write(data)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > 64<<20 {
		return nil, fmt.Errorf("broker frame exceeds limit")
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

func writeBrokerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
