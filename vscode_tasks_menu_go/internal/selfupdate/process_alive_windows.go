//go:build windows

package selfupdate

func UpdaterProcessAlive(pid int) bool {
	// Keep Windows conservative: do not declare an updater abandoned from a
	// PID-only probe here. The timestamp fallback still clears stale requests.
	return pid > 0
}
