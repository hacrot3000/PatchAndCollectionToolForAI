//go:build !windows

package session

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const maxProcessTreeOutput = 2 << 20

func collectProcessTree(rootPID int) ([]ProcessInfo, error) {
	if rootPID <= 0 {
		return nil, fmt.Errorf("invalid session process id")
	}
	cmd := exec.Command("ps", "-eo", "pid=,ppid=,pgid=,stat=,etimes=,pcpu=,rss=,comm=,args=")
	var out bytes.Buffer
	cmd.Stdout = &limitedProcessBuffer{limit: maxProcessTreeOutput}
	buffer := cmd.Stdout.(*limitedProcessBuffer)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("read process tree: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if buffer.truncated {
		return nil, fmt.Errorf("process table exceeds %d bytes", maxProcessTreeOutput)
	}
	out.Write(buffer.data)
	return parseProcessTable(out.String(), rootPID), nil
}

type limitedProcessBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *limitedProcessBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return n, nil
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.truncated = true
		return n, nil
	}
	b.data = append(b.data, p...)
	return n, nil
}

func parseProcessTable(raw string, rootPID int) []ProcessInfo {
	all := map[int]ProcessInfo{}
	children := map[int][]int{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 9 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		pgid, err3 := strconv.Atoi(fields[2])
		elapsed, err4 := strconv.ParseInt(fields[4], 10, 64)
		cpu, err5 := strconv.ParseFloat(fields[5], 64)
		rssKB, err6 := strconv.ParseInt(fields[6], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil || pid <= 0 {
			continue
		}
		item := ProcessInfo{
			PID: pid, PPID: ppid, PGID: pgid, State: fields[3],
			ElapsedSeconds: elapsed, CPUPercent: cpu, RSSBytes: rssKB * 1024,
			Command: fields[7], Args: strings.Join(fields[8:], " "),
		}
		all[pid] = item
		children[ppid] = append(children[ppid], pid)
	}
	if _, ok := all[rootPID]; !ok {
		return []ProcessInfo{}
	}
	for ppid := range children {
		sort.Ints(children[ppid])
	}
	result := make([]ProcessInfo, 0)
	var walk func(int, int)
	walk = func(pid, depth int) {
		item, ok := all[pid]
		if !ok {
			return
		}
		item.Depth = depth
		result = append(result, item)
		for _, child := range children[pid] {
			walk(child, depth+1)
		}
	}
	walk(rootPID, 0)
	return result
}
