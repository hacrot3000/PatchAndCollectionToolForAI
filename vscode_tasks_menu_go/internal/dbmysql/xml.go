package dbmysql

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

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
	decoder := xml.NewDecoder(bytes.NewReader(data))
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
