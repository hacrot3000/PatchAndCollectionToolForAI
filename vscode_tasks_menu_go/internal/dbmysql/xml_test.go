package dbmysql

import (
	"strings"
	"testing"
)

func TestParseXMLResultNormalizesRowsAndNulls(t *testing.T) {
	xml := "<?xml version=\"1.0\"?>\n" +
		"<resultset statement=\"SELECT id,name,note FROM users\">\n" +
		"<row><field name=\"id\">1</field><field name=\"name\">Alice</field><field name=\"note\" xsi:nil=\"true\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" /></row>\n" +
		"<row><field name=\"id\">2</field><field name=\"name\">Bob &amp; Carol</field><field name=\"note\"></field></row>\n" +
		"</resultset>"
	result, err := parseXMLResult([]byte(xml), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) != 3 || result.Columns[0].Name != "id" || result.Columns[1].Name != "name" {
		t.Fatalf("columns=%+v", result.Columns)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows=%+v", result.Rows)
	}
	if result.Rows[0][2] != nil {
		t.Fatalf("NULL cell=%#v", result.Rows[0][2])
	}
	if got := result.Rows[1][1]; got != "Bob & Carol" {
		t.Fatalf("decoded XML text=%#v", got)
	}
	if got := result.Rows[1][2]; got != "" {
		t.Fatalf("empty cell=%#v", got)
	}
	if result.Truncated {
		t.Fatal("unexpected truncation")
	}
}

func TestParseXMLResultTruncatesRows(t *testing.T) {
	xml := "<resultset>" +
		"<row><field name=\"id\">1</field></row>" +
		"<row><field name=\"id\">2</field></row>" +
		"<row><field name=\"id\">3</field></row>" +
		"</resultset>"
	result, err := parseXMLResult([]byte(xml), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || !result.Truncated {
		t.Fatalf("result=%+v", result)
	}
}

func TestParseXMLResultRejectsMultipleResultsets(t *testing.T) {
	_, err := parseXMLResult([]byte("<resultset></resultset><resultset></resultset>"), 10)
	if err == nil || !strings.Contains(err.Error(), "multiple resultsets") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseXMLResultRejectsChangedRowShape(t *testing.T) {
	xml := "<resultset>" +
		"<row><field name=\"id\">1</field><field name=\"name\">Alice</field></row>" +
		"<row><field name=\"name\">Bob</field><field name=\"id\">2</field></row>" +
		"</resultset>"
	_, err := parseXMLResult([]byte(xml), 10)
	if err == nil || !strings.Contains(err.Error(), "column order changed") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseXMLResultRejectsNonXMLOutput(t *testing.T) {
	if _, err := parseXMLResult([]byte("not xml"), 10); err == nil {
		t.Fatal("expected non-XML output to fail")
	}
}

func TestParseXMLResultAllowsEmptySuccessfulOutput(t *testing.T) {
	result, err := parseXMLResult(nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) != 0 || len(result.Rows) != 0 {
		t.Fatalf("result=%+v", result)
	}
}


func TestParseXMLResultEscapesUnsafeLegacyBlobBytesWithoutChangingOtherCells(t *testing.T) {
	data := []byte("<resultset><row><field name=\"msg\">中文 &amp; ok</field><field name=\"extra\">")
	data = append(data, 0x18, 0x00, 0xff)
	data = append(data, []byte("</field><field name=\"note\" xsi:nil=\"true\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" /></row></resultset>")...)
	result, err := parseXMLResult(data, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || len(result.Rows[0]) != 3 {
		t.Fatalf("result=%+v", result)
	}
	if got := result.Rows[0][0]; got != "中文 & ok" {
		t.Fatalf("regular text changed: %#v", got)
	}
	if got := result.Rows[0][1]; got != "\\x18\\x00\\xFF" {
		t.Fatalf("binary preview does not expose unsafe bytes: %#v", got)
	}
	if result.Rows[0][2] != nil {
		t.Fatalf("NULL changed: %#v", result.Rows[0][2])
	}
}

func TestParseXMLResultEscapesForbiddenNumericReferencesInField(t *testing.T) {
	data := []byte("<resultset><row><field name=\"extra\">&#x18;:&#0;:&#31;</field></row></resultset>")
	result, err := parseXMLResult(data, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Rows[0][0]; got != "\\u0018:\\u0000:\\u001F" {
		t.Fatalf("numeric references result=%#v", got)
	}
}

func TestParseXMLResultDoesNotHideMalformedMarkupOutsideField(t *testing.T) {
	if _, err := parseXMLResult([]byte("<resultset>\x18</resultset>"), 10); err == nil {
		t.Fatal("expected malformed XML outside fields to be rejected")
	}
}

func TestParseXMLResultPreservesNativeHexBlob(t *testing.T) {
	result, err := parseXMLResult([]byte("<resultset><row><field name=\"extra\">0x0018FF</field></row></resultset>"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Rows[0][0]; got != "0x0018FF" {
		t.Fatalf("hex blob changed: %#v", got)
	}
}
