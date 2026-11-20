package ftpclient

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Entry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

func ParseMLSD(data string) ([]Entry, error) {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	entries := make([]Entry, 0)
	for _, raw := range lines {
		line := strings.TrimLeft(strings.TrimRight(raw, "\r"), " \t")
		if line == "" {
			continue
		}
		space := strings.IndexByte(line, ' ')
		if space <= 0 || space == len(line)-1 {
			return nil, fmt.Errorf("malformed MLSD line")
		}
		factText := line[:space]
		name := line[space+1:]
		facts := make(map[string]string)
		for _, item := range strings.Split(factText, ";") {
			if item == "" {
				continue
			}
			eq := strings.IndexByte(item, '=')
			if eq <= 0 {
				continue
			}
			facts[strings.ToLower(item[:eq])] = item[eq+1:]
		}
		kind := strings.ToLower(facts["type"])
		if kind == "cdir" || kind == "pdir" || name == "." || name == ".." {
			continue
		}
		entryType := "other"
		switch kind {
		case "file":
			entryType = "file"
		case "dir":
			entryType = "directory"
		case "os.unix=slink", "slink":
			entryType = "symlink"
		}
		var size int64
		if rawSize := facts["size"]; rawSize != "" {
			parsed, err := strconv.ParseInt(rawSize, 10, 64)
			if err != nil || parsed < 0 {
				return nil, fmt.Errorf("malformed MLSD size %q", rawSize)
			}
			size = parsed
		}
		modified := normalizeModify(facts["modify"])
		entries = append(entries, Entry{Name: name, Type: entryType, Size: size, Modified: modified})
		if len(entries) > MaxListEntries {
			return nil, fmt.Errorf("ftp listing exceeds %d entries", MaxListEntries)
		}
	}
	return entries, nil
}

func normalizeModify(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 14 {
		return value
	}
	t, err := time.Parse("20060102150405", value[:14])
	if err != nil {
		return value
	}
	return t.UTC().Format(time.RFC3339)
}

func ParseLIST(data string) ([]Entry, error) {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	entries := make([]Entry, 0)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "total ") {
			continue
		}
		if entry, ok := parseUnixLIST(line); ok {
			if entry.Name != "." && entry.Name != ".." {
				entries = append(entries, entry)
			}
		} else if entry, ok := parseDOSLIST(line); ok {
			if entry.Name != "." && entry.Name != ".." {
				entries = append(entries, entry)
			}
		} else {
			return nil, fmt.Errorf("unsupported FTP LIST line %q", line)
		}
		if len(entries) > MaxListEntries {
			return nil, fmt.Errorf("ftp listing exceeds %d entries", MaxListEntries)
		}
	}
	return entries, nil
}

func parseUnixLIST(line string) (Entry, bool) {
	fields, rest := splitFieldsAndRest(line, 8)
	if len(fields) != 8 || len(fields[0]) < 10 || rest == "" {
		return Entry{}, false
	}
	switch fields[0][0] {
	case '-', 'd', 'l', 'b', 'c', 'p', 's':
	default:
		return Entry{}, false
	}
	size, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil || size < 0 {
		return Entry{}, false
	}
	name := strings.TrimSpace(rest)
	entryType := "file"
	switch fields[0][0] {
	case 'd':
		entryType = "directory"
	case 'l':
		entryType = "symlink"
		if idx := strings.LastIndex(name, " -> "); idx > 0 {
			name = name[:idx]
		}
	case '-':
	default:
		entryType = "other"
	}
	return Entry{Name: name, Type: entryType, Size: size, Modified: strings.Join(fields[5:8], " ")}, true
}

func parseDOSLIST(line string) (Entry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return Entry{}, false
	}
	if _, err := time.Parse("01-02-06 03:04PM", fields[0]+" "+strings.ToUpper(fields[1])); err != nil {
		if _, err24 := time.Parse("01-02-06 15:04", fields[0]+" "+fields[1]); err24 != nil {
			return Entry{}, false
		}
	}
	nameIndex := nthFieldOffset(line, 3)
	if nameIndex < 0 {
		return Entry{}, false
	}
	name := strings.TrimSpace(line[nameIndex:])
	if name == "" {
		return Entry{}, false
	}
	entryType := "file"
	var size int64
	if strings.EqualFold(fields[2], "<DIR>") {
		entryType = "directory"
	} else {
		parsed, err := strconv.ParseInt(strings.ReplaceAll(fields[2], ",", ""), 10, 64)
		if err != nil || parsed < 0 {
			return Entry{}, false
		}
		size = parsed
	}
	return Entry{Name: name, Type: entryType, Size: size, Modified: fields[0] + " " + fields[1]}, true
}

func splitFieldsAndRest(value string, count int) ([]string, string) {
	fields := make([]string, 0, count)
	i := 0
	for len(fields) < count {
		for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
			i++
		}
		if i >= len(value) {
			return fields, ""
		}
		start := i
		for i < len(value) && value[i] != ' ' && value[i] != '\t' {
			i++
		}
		fields = append(fields, value[start:i])
	}
	for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
		i++
	}
	return fields, value[i:]
}

func nthFieldOffset(value string, count int) int {
	i := 0
	for field := 0; field < count; field++ {
		for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
			i++
		}
		if i >= len(value) {
			return -1
		}
		for i < len(value) && value[i] != ' ' && value[i] != '\t' {
			i++
		}
	}
	for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
		i++
	}
	if i >= len(value) {
		return -1
	}
	return i
}
