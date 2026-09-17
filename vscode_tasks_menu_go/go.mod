module bletonfc/vscode_tasks_menu

go 1.19

require (
	github.com/coder/websocket v1.8.13
	github.com/creack/pty v1.1.24
)

replace github.com/coder/websocket => ./third_party/github.com/coder/websocket

replace github.com/creack/pty => ./third_party/github.com/creack/pty
