package terminal

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

const rootBuildGroup = "build"

type entry struct {
	kind      string
	label     string
	task      *tasks.Task
	groupPath []string
}

type ttyState struct {
	encoded string
}

func Run(workspace string) error {
	if !isTTY(os.Stdin) || !isTTY(os.Stdout) {
		return errors.New("terminal mode cần TTY tương tác")
	}
	original, err := saveTTYState()
	if err != nil {
		return err
	}
	if err := makeRaw(); err != nil {
		return err
	}
	defer restoreTTY(original)

	items, err := tasks.Load(workspace)
	if err != nil {
		return err
	}
	groupPath := []string{}
	selected := 0

	for {
		entries := buildEntries(items, groupPath)
		if len(entries) == 0 {
			groupPath = nil
			selected = 0
			continue
		}
		if selected >= len(entries) {
			selected = len(entries) - 1
		}
		if selected < 0 {
			selected = 0
		}
		render(workspace, groupPath, entries, selected)
		key, err := readKey()
		if err != nil {
			return err
		}
		switch key {
		case "up", "k":
			selected--
			if selected < 0 {
				selected = len(entries) - 1
			}
		case "down", "j":
			selected++
			if selected >= len(entries) {
				selected = 0
			}
		case "home":
			selected = 0
		case "end":
			selected = len(entries) - 1
		case "reload":
			loaded, loadErr := tasks.Load(workspace)
			if loadErr != nil {
				showMessage(original, fmt.Sprintf("Reload tasks.json lỗi: %v", loadErr))
				if err := makeRaw(); err != nil {
					return err
				}
				continue
			}
			items = loaded
			if !groupExists(items, groupPath) {
				groupPath = nil
			}
			selected = 0
		case "back", "quit":
			if len(groupPath) == 0 {
				fmt.Print("\x1b[2J\x1b[H")
				return nil
			}
			groupPath = append([]string(nil), groupPath[:len(groupPath)-1]...)
			selected = 0
		case "enter":
			chosen := entries[selected]
			switch chosen.kind {
			case "exit":
				fmt.Print("\x1b[2J\x1b[H")
				return nil
			case "back":
				groupPath = append([]string(nil), chosen.groupPath...)
				selected = 0
			case "group":
				groupPath = append([]string(nil), chosen.groupPath...)
				selected = 0
			case "task":
				if chosen.task != nil {
					if err := runTask(original, workspace, *chosen.task); err != nil {
						showMessage(original, fmt.Sprintf("Lỗi: %v", err))
					}
					if err := makeRaw(); err != nil {
						return err
					}
				}
			}
		}
	}
}

func buildEntries(items []tasks.Task, groupPath []string) []entry {
	if len(groupPath) == 0 {
		out := make([]entry, 0)
		for i := range items {
			if samePath(items[i].Group, []string{rootBuildGroup}) {
				task := &items[i]
				out = append(out, entry{kind: "task", label: task.MenuLabel, task: task})
			}
		}
		seen := map[string]bool{}
		for i := range items {
			path := items[i].Group
			if len(path) == 0 || samePath(path, []string{rootBuildGroup}) {
				continue
			}
			name := path[0]
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, entry{kind: "group", label: "[Group] " + name, groupPath: []string{name}})
		}
		return append(out, entry{kind: "exit", label: "Thoát"})
	}

	out := make([]entry, 0)
	depth := len(groupPath)
	seen := map[string]bool{}
	for i := range items {
		path := items[i].Group
		if len(path) <= depth || !hasPrefix(path, groupPath) {
			continue
		}
		child := path[depth]
		if seen[child] {
			continue
		}
		seen[child] = true
		childPath := append(append([]string(nil), groupPath...), child)
		out = append(out, entry{kind: "group", label: "[Group] " + child, groupPath: childPath})
	}
	for i := range items {
		if samePath(items[i].Group, groupPath) {
			task := &items[i]
			out = append(out, entry{kind: "task", label: task.MenuLabel, task: task})
		}
	}
	parent := append([]string(nil), groupPath[:len(groupPath)-1]...)
	return append(out, entry{kind: "back", label: "< Quay lại", groupPath: parent})
}

func render(workspace string, groupPath []string, entries []entry, selected int) {
	fmt.Print("\x1b[2J\x1b[H")
	title := " VS CODE TASKS "
	if len(groupPath) > 0 {
		title = " VS CODE TASKS / " + strings.Join(groupPath, " / ") + " "
	}
	fmt.Printf("\x1b[1m%s\x1b[0m\n\n", title)
	for i, e := range entries {
		prefix := "  "
		if i == selected {
			prefix = "› "
			fmt.Printf("\x1b[7m%s%s\x1b[0m\n", prefix, e.label)
		} else {
			fmt.Printf("%s%s\n", prefix, e.label)
		}
	}
	fmt.Println("\n──────────────────────────────────────────────────────────────────────────────")
	chosen := entries[selected]
	if chosen.task != nil {
		if spec, err := tasks.ResolveExecution(*chosen.task, workspace); err == nil {
			fmt.Printf("Lệnh: %s\n", spec.Preview)
		}
		if chosen.task.Detail != "" {
			fmt.Printf("Mô tả: %s\n", chosen.task.Detail)
		}
	} else if chosen.kind == "group" {
		fmt.Printf("Group: %s\n", strings.Join(chosen.groupPath, " / "))
	}
	fmt.Println("\n↑/↓ hoặc j/k: chọn | Enter: mở/chạy | r: reload | Home/End | q/Esc: quay lại/thoát")
}

func runTask(original ttyState, workspace string, task tasks.Task) error {
	spec, err := tasks.ResolveExecution(task, workspace)
	if err != nil {
		return err
	}
	if err := restoreTTY(original); err != nil {
		return err
	}
	fmt.Print("\x1b[2J\x1b[H")
	fmt.Println(strings.Repeat("=", 78))
	fmt.Printf("Task       : %s\n", task.Label)
	fmt.Printf("Thư mục    : %s\n", spec.Cwd)
	fmt.Printf("Lệnh       : %s\n", spec.Preview)
	fmt.Printf("Mô tả      : %s\n", task.Detail)
	fmt.Println(strings.Repeat("=", 78))
	fmt.Println()

	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Cwd
	cmd.Env = spec.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}
	fmt.Printf("\nTask kết thúc với mã trả về: %d\n", code)
	fmt.Print("Nhấn Enter để quay lại menu...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}

func showMessage(original ttyState, message string) {
	_ = restoreTTY(original)
	fmt.Print("\x1b[2J\x1b[H")
	fmt.Println(message)
	fmt.Print("\nNhấn Enter để tiếp tục...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func readKey() (string, error) {
	buf := make([]byte, 16)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	s := string(buf[:n])
	if strings.HasPrefix(s, "\x1b[A") {
		return "up", nil
	}
	if strings.HasPrefix(s, "\x1b[B") {
		return "down", nil
	}
	if strings.HasPrefix(s, "\x1b[H") || strings.HasPrefix(s, "\x1b[1~") {
		return "home", nil
	}
	if strings.HasPrefix(s, "\x1b[F") || strings.HasPrefix(s, "\x1b[4~") {
		return "end", nil
	}
	switch buf[0] {
	case '\r', '\n':
		return "enter", nil
	case 'r', 'R':
		return "reload", nil
	case 'q', 'Q':
		return "quit", nil
	case 'k':
		return "k", nil
	case 'j':
		return "j", nil
	case 0x1b:
		return "back", nil
	}
	return "", nil
}

func groupExists(items []tasks.Task, path []string) bool {
	if len(path) == 0 {
		return true
	}
	for i := range items {
		if hasPrefix(items[i].Group, path) {
			return true
		}
	}
	return false
}

func samePath(a, b []string) bool {
	return len(a) == len(b) && hasPrefix(a, b)
}

func hasPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func saveTTYState() (ttyState, error) {
	cmd := exec.Command("stty", "-g")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return ttyState{}, fmt.Errorf("stty -g: %w", err)
	}
	return ttyState{encoded: strings.TrimSpace(string(out))}, nil
}

func makeRaw() error {
	cmd := exec.Command("stty", "raw", "-echo")
	cmd.Stdin = os.Stdin
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stty raw: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func restoreTTY(state ttyState) error {
	if state.encoded == "" {
		return nil
	}
	cmd := exec.Command("stty", state.encoded)
	cmd.Stdin = os.Stdin
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore stty: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
