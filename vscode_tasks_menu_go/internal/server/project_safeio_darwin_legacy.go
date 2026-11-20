//go:build darwin && !go1.25

package server

// Native macOS editing requires os.Root.Rename (Go 1.25). Refuse the
// unsafe !linux fallback rather than exposing a symlink swap race.
var _ = requires_Go_1_25_or_newer_for_safe_macOS_editor
