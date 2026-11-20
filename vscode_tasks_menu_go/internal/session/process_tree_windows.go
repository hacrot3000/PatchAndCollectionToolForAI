//go:build windows

package session

import "fmt"

func collectProcessTree(rootPID int) ([]ProcessInfo, error) {
	return nil, fmt.Errorf("process tree inspection is not available on Windows")
}
