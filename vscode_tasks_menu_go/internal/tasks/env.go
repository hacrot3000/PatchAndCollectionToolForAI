package tasks

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ApplyEnvironmentOverrides merges explicit browser-selected profile values into
// an already resolved execution environment. Values are passed directly to the
// child process; they are never interpreted as shell syntax.
func ApplyEnvironmentOverrides(spec *Execution, overrides map[string]string) error {
	if len(overrides) == 0 {
		return nil
	}
	values := make(map[string]string, len(spec.Env)+len(overrides))
	for _, item := range spec.Env {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		if !environmentNamePattern.MatchString(key) {
			return fmt.Errorf("tên biến môi trường không hợp lệ %q", key)
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	spec.Env = spec.Env[:0]
	for _, key := range keys {
		spec.Env = append(spec.Env, key+"="+values[key])
	}
	return nil
}
