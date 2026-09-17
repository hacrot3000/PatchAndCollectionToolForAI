package tasks

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Input struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Task struct {
	ID        int            `json:"id"`
	Label     string         `json:"label"`
	MenuLabel string         `json:"menu_label"`
	Group     []string       `json:"group"`
	Detail    string         `json:"detail"`
	Type      string         `json:"type"`
	Command   any            `json:"command"`
	Args      any            `json:"args,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Inputs    []Input        `json:"inputs,omitempty"`
	Raw       map[string]any `json:"raw"`
}

type document struct {
	Tasks  []map[string]any `json:"tasks"`
	Inputs []map[string]any `json:"inputs"`
}

var inputReferencePattern = regexp.MustCompile(`\$\{input:([^}]+)\}`)

func Load(workspace string) ([]Task, error) {
	path := filepath.Join(workspace, ".vscode", "tasks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", path, err)
	}
	clean, err := stripJSONC(data)
	if err != nil {
		return nil, fmt.Errorf("parse JSONC %s: %w", path, err)
	}
	var doc document
	if err := json.Unmarshal(clean, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	inputDefs, inputOrder := parseInputs(doc.Inputs)
	out := make([]Task, 0, len(doc.Tasks))
	identities := make(map[int]string, len(doc.Tasks))
	for _, raw := range doc.Tasks {
		label := stringValue(raw["label"])
		if label == "" {
			continue
		}
		id, identity, err := stableTaskID(raw)
		if err != nil {
			return nil, fmt.Errorf("tạo task id cho %q: %w", label, err)
		}
		if previous, exists := identities[id]; exists && previous != identity {
			return nil, fmt.Errorf("task id collision cho %q; sửa tasks.json rồi reload", label)
		}
		identities[id] = identity
		menuLabel := stringValue(raw["menuLabel"])
		if menuLabel == "" {
			menuLabel = label
		}
		out = append(out, Task{
			ID: id, Label: label, MenuLabel: menuLabel,
			Group: menuGroup(raw), Detail: stringValue(raw["detail"]),
			Type: stringValue(raw["type"]), Command: raw["command"], Args: raw["args"],
			Options: mapValue(raw["options"]), Inputs: referencedInputs(raw, inputDefs, inputOrder), Raw: raw,
		})
	}
	return out, nil
}

func parseInputs(raw []map[string]any) (map[string]Input, []string) {
	out := make(map[string]Input, len(raw))
	order := make([]string, 0, len(raw))
	for _, value := range raw {
		id := stringValue(value["id"])
		if id == "" {
			continue
		}
		input := Input{
			ID:          id,
			Type:        stringValue(value["type"]),
			Description: stringValue(value["description"]),
			Default:     fmt.Sprint(value["default"]),
		}
		if value["default"] == nil {
			input.Default = ""
		}
		if values, ok := value["options"].([]any); ok {
			for _, option := range values {
				input.Options = append(input.Options, fmt.Sprint(option))
			}
		}
		if _, exists := out[id]; !exists {
			order = append(order, id)
		}
		out[id] = input
	}
	return out, order
}

func referencedInputs(raw map[string]any, defs map[string]Input, definitionOrder []string) []Input {
	seen := make(map[string]bool)
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case string:
			for _, match := range inputReferencePattern.FindAllStringSubmatch(v, -1) {
				if len(match) == 2 {
					seen[match[1]] = true
				}
			}
		case []any:
			for _, item := range v {
				visit(item)
			}
		case map[string]any:
			for _, item := range v {
				visit(item)
			}
		}
	}
	visit(raw)

	out := make([]Input, 0, len(seen))
	for _, id := range definitionOrder {
		if !seen[id] {
			continue
		}
		out = append(out, defs[id])
		delete(seen, id)
	}
	missing := make([]string, 0, len(seen))
	for id := range seen {
		missing = append(missing, id)
	}
	sort.Strings(missing)
	for _, id := range missing {
		out = append(out, Input{ID: id, Type: "missing", Description: "Input chưa được khai báo trong tasks.json"})
	}
	return out
}

// stableTaskID intentionally depends on the task definition instead of its array
// position. A browser that still shows an older menu can therefore never execute a
// different task merely because tasks.json was reordered or a task was inserted.
//
// Keep the identifier within positive signed 31-bit range so JSON/JavaScript and
// every Go target represent it exactly. A hash collision is detected by Load and
// fails closed instead of risking execution of the wrong command.
func stableTaskID(raw map[string]any) (int, string, error) {
	canonical, err := json.Marshal(raw)
	if err != nil {
		return 0, "", err
	}
	sum := sha256.Sum256(canonical)
	id := int(binary.BigEndian.Uint32(sum[:4]) & 0x7fffffff)
	if id == 0 {
		id = 1
	}
	return id, string(canonical), nil
}

func menuGroup(raw map[string]any) []string {
	if value := stringValue(raw["menuGroup"]); value != "" {
		parts := strings.Split(value, "/")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	switch group := raw["group"].(type) {
	case string:
		if strings.EqualFold(group, "build") {
			return []string{"build"}
		}
		if strings.EqualFold(group, "test") {
			return []string{"Tests"}
		}
	case map[string]any:
		kind := stringValue(group["kind"])
		if strings.EqualFold(kind, "build") {
			return []string{"build"}
		}
		if strings.EqualFold(kind, "test") {
			return []string{"Tests"}
		}
	}
	label := stringValue(raw["label"])
	if strings.HasPrefix(label, "M3 Client VM:") {
		return []string{"M3 Client", "VM"}
	}
	if strings.HasPrefix(label, "M3 Client:") {
		return []string{"M3 Client"}
	}
	for _, prefix := range []string{"Git", "Docker", "Cocos", "Patchs"} {
		if strings.HasPrefix(label, prefix+":") {
			return []string{prefix}
		}
	}
	return []string{"Khác"}
}

func stringValue(v any) string      { s, _ := v.(string); return strings.TrimSpace(s) }
func mapValue(v any) map[string]any { m, _ := v.(map[string]any); return m }

func stripJSONC(src []byte) ([]byte, error) {
	out := make([]byte, 0, len(src))
	inString, escaped := false, false
	for i := 0; i < len(src); {
		c := src[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			i++
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			i++
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			i += 2
			for i < len(src) && src[i] != '\n' && src[i] != '\r' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' || src[i] == '\r' {
					out = append(out, src[i])
				}
				i++
			}
			if i+1 >= len(src) {
				return nil, fmt.Errorf("comment /* ... */ chưa đóng")
			}
			i += 2
			continue
		}
		out = append(out, c)
		i++
	}
	if inString {
		return nil, fmt.Errorf("chuỗi JSON chưa đóng")
	}

	src = out
	out = make([]byte, 0, len(src))
	inString, escaped = false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r' || src[j] == '\n') {
				j++
			}
			if j < len(src) && (src[j] == ']' || src[j] == '}') {
				continue
			}
		}
		out = append(out, c)
	}
	return out, nil
}
