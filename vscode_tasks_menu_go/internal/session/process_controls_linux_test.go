//go:build linux

package session

import (
	"strings"
	"testing"
)

func TestParseProcessTableBuildsOnlySessionDescendants(t *testing.T) {
	raw := strings.Join([]string{
		"10 1 10 Ss 100 2.5 2048 shell /bin/sh",
		"11 10 10 S 90 1.0 1024 sleep sleep 30",
		"12 11 10 S 80 0.5 512 helper helper --child",
		"99 1 99 S 70 9.9 4096 other other",
	}, "\n")
	rows := parseProcessTable(raw, 10)
	if len(rows) != 3 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].PID != 10 || rows[0].Depth != 0 || rows[1].PID != 11 || rows[1].Depth != 1 || rows[2].PID != 12 || rows[2].Depth != 2 {
		t.Fatalf("unexpected tree=%+v", rows)
	}
	if rows[0].CPUPercent != 2.5 || rows[0].RSSBytes != 2048*1024 {
		t.Fatalf("root resources=%+v", rows[0])
	}
	if rows[1].CPUPercent != 1.0 || rows[1].RSSBytes != 1024*1024 {
		t.Fatalf("child resources=%+v", rows[1])
	}
}

