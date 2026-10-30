package dbmysql

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type xmlField struct {
	Name       string     `xml:"name,attr"`
	Attributes []xml.Attr `xml:",any,attr"`
	Value      string     `xml:",chardata"`
}

func parseXMLResult(data []byte, maxRows int) (dbadapter.ExecuteResult, error) {
	if maxRows < 1 || maxRows > dbadapter.MaxRows {
		return dbadapter.ExecuteResult{}, fmt.Errorf("MySQL max rows must be between 1 and %d", dbadapter.MaxRows)
	}
	// mysql --binary-as-hex preserves BLOBs; MariaDB's XML output can still
	// contain forbidden XML 1.0 control bytes. Escape these visibly inside
	// field text only, never silently drop bytes or alter SQL/markup.
	decoder := xml.NewDecoder(bytes.NewReader(escapeUnsafeXMLFields(data)))
	result := dbadapter.ExecuteResult{}
	var (
		inResultset bool
		resultsets  int
		columns     []string
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return dbadapter.ExecuteResult{}, fmt.Errorf("parse MySQL XML output: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "resultset":
			resultsets++
			if resultsets > 1 {
				return dbadapter.ExecuteResult{}, errors.New("MySQL output contains multiple resultsets")
			}
			inResultset = true
		case "row":
			if !inResultset {
				return dbadapter.ExecuteResult{}, errors.New("MySQL XML row appears outside resultset")
			}
			fields, err := decodeXMLRow(decoder, start)
			if err != nil {
				return dbadapter.ExecuteResult{}, err
			}
			if columns == nil {
				if len(fields) > dbadapter.MaxColumns {
					return dbadapter.ExecuteResult{}, fmt.Errorf("MySQL result exceeds %d columns", dbadapter.MaxColumns)
				}
				columns = make([]string, len(fields))
				result.Columns = make([]dbadapter.Column, len(fields))
				for i, field := range fields {
					name := strings.TrimSpace(field.Name)
					if name == "" {
						return dbadapter.ExecuteResult{}, fmt.Errorf("MySQL column %d has no name", i+1)
					}
					columns[i] = name
					result.Columns[i] = dbadapter.Column{Name: name}
				}
			} else {
				if len(fields) != len(columns) {
					return dbadapter.ExecuteResult{}, fmt.Errorf(
						"MySQL row has %d fields for %d columns",
						len(fields),
						len(columns),
					)
				}
				for i, field := range fields {
					if strings.TrimSpace(field.Name) != columns[i] {
						return dbadapter.ExecuteResult{}, fmt.Errorf(
							"MySQL column order changed at position %d: %q != %q",
							i+1,
							field.Name,
							columns[i],
						)
					}
				}
			}

			if len(result.Rows) >= maxRows {
				result.Truncated = true
				continue
			}
			row := make([]interface{}, len(fields))
			for i, field := range fields {
				if xmlFieldIsNil(field) {
					row[i] = nil
				} else {
					row[i] = field.Value
				}
			}
			result.Rows = append(result.Rows, row)
		}
	}
	if resultsets == 0 {
		if len(bytes.TrimSpace(data)) == 0 {
			return result, nil
		}
		return dbadapter.ExecuteResult{}, errors.New("MySQL XML output contains no resultset")
	}
	if err := dbadapter.ValidateExecuteResult(result); err != nil {
		return dbadapter.ExecuteResult{}, err
	}
	return result, nil
}

func decodeXMLRow(decoder *xml.Decoder, start xml.StartElement) ([]xmlField, error) {
	var row struct {
		Fields []xmlField `xml:"field"`
	}
	if err := decoder.DecodeElement(&row, &start); err != nil {
		return nil, fmt.Errorf("decode MySQL XML row: %w", err)
	}
	return row.Fields, nil
}

func xmlFieldIsNil(field xmlField) bool {
	for _, attr := range field.Attributes {
		if strings.EqualFold(attr.Name.Local, "nil") {
			value := strings.ToLower(strings.TrimSpace(attr.Value))
			return value == "true" || value == "1"
		}
	}
	return false
}


// escapeUnsafeXMLFields makes legacy MariaDB --xml output parseable when a
// BLOB/text field contains raw XML 1.0 forbidden code points or invalid UTF-8.
// Non-XML bytes are represented visibly as \xNN (not silently discarded).
// This is a read-only presentation fallback; MySQL's --binary-as-hex is used
// whenever available to preserve the full original binary value.
func escapeUnsafeXMLFields(data []byte) []byte {
	var out bytes.Buffer
	for len(data) > 0 {
		start := bytes.Index(data, []byte("<field"))
		if start < 0 {
			out.Write(data)
			break
		}
		next := start + len("<field")
		if next == len(data) {
			out.Write(data)
			break
		}
		switch data[next] {
		case ' ', '\t', '\r', '\n', '>', '/':
		default:
			out.Write(data[:next])
			data = data[next:]
			continue
		}
		end := xmlStartTagEnd(data, start)
		if end < 0 {
			out.Write(data)
			break
		}
		out.Write(data[:end+1])
		if bytes.HasSuffix(bytes.TrimSpace(data[start:end]), []byte("/")) {
			data = data[end+1:]
			continue
		}
		closing := bytes.Index(data[end+1:], []byte("</field>"))
		if closing < 0 {
			out.Write(data[end+1:])
			break
		}
		closing += end + 1
		out.Write(escapeUnsafeXMLText(data[end+1 : closing]))
		data = data[closing:]
	}
	return out.Bytes()
}

func xmlStartTagEnd(data []byte, start int) int {
	var quote byte
	for i := start + 1; i < len(data); i++ {
		switch {
		case quote != 0 && data[i] == quote:
			quote = 0
		case quote == 0 && (data[i] == '"' || data[i] == '\''):
			quote = data[i]
		case quote == 0 && data[i] == '>':
			return i
		}
	}
	return -1
}

func validXML10Rune(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

func escapeUnsafeXMLText(data []byte) []byte {
	var out bytes.Buffer
	for i := 0; i < len(data); {
		// Some clients emit numeric references for forbidden control bytes.
		// Go's XML decoder rejects those just like literal control bytes.
		if data[i] == '&' && i+2 < len(data) && data[i+1] == '#' {
			if end := bytes.IndexByte(data[i+2:], ';'); end >= 0 && end <= 12 {
				entity := string(data[i+2 : i+2+end])
				base := 10
				if strings.HasPrefix(strings.ToLower(entity), "x") {
					entity = entity[1:]
					base = 16
				}
				if value, err := strconv.ParseUint(entity, base, 32); err == nil && !validXML10Rune(rune(value)) {
					fmt.Fprintf(&out, "\\u%04X", value)
					i += end + 3
					continue
				}
			}
		}
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&out, "\\x%02X", data[i])
		} else if !validXML10Rune(r) {
			if r <= 0xFF {
				fmt.Fprintf(&out, "\\x%02X", r)
			} else {
				fmt.Fprintf(&out, "\\u%04X", r)
			}
		} else {
			out.Write(data[i : i+size])
		}
		i += size
	}
	return out.Bytes()
}
