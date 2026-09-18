package session

import "bletonfc/vscode_tasks_menu/internal/tasks"

// Service is the session capability surface consumed by the HTTP/WebSocket
// server. Manager implements it directly. A broker-backed client can implement
// the same contract without changing public browser APIs.
type Service interface {
	List() []Metadata
	Start(tasks.Execution) (Metadata, error)
	Metadata(string) (Metadata, bool)
	SetTitle(string, string) (Metadata, error)
	Input(string, []byte) error
	Resize(string, uint16, uint16) error
	Stop(string) error
	Kill(string) error
	Clear(string) error
	Remove(string) error
	Subscribe(string) ([]byte, <-chan []byte, func(), error)
	CurrentCwd(string) (string, error)
}

var _ Service = (*Manager)(nil)
