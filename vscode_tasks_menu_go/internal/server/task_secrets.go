package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func taskWorkflowSecretRefs(items []tasks.Task, rootLabel string) (map[string]string, error) {
	graph, err := tasks.BuildWorkflowGraph(items, rootLabel)
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for _, label := range graph.Order {
		task, ok := tasks.TaskByLabel(items, label)
		if !ok {
			return nil, fmt.Errorf("task %q disappeared while resolving secrets", label)
		}
		for envName, secretID := range task.SecretEnv {
			envName = strings.TrimSpace(envName)
			secretID = strings.TrimSpace(secretID)
			if envName == "" || secretID == "" {
				return nil, fmt.Errorf("task %q has an invalid secret environment mapping", label)
			}
			if previous, exists := refs[envName]; exists && previous != secretID {
				return nil, fmt.Errorf("workflow secret environment %q maps to multiple secret ids", envName)
			}
			refs[envName] = secretID
		}
	}
	return refs, nil
}

func sortedSecretEnvironmentNames(refs map[string]string) []string {
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Server) injectTaskSecrets(spec *tasks.Execution, refs map[string]string) error {
	if len(refs) == 0 {
		return nil
	}
	if s.ConnectionSecrets == nil {
		return fmt.Errorf("TaskDeck secret store is unavailable")
	}
	values := make(map[string]string, len(refs))
	for envName, secretID := range refs {
		secret, err := s.ConnectionSecrets.Get(secretID)
		if err != nil {
			return fmt.Errorf("secret for task environment %q is unavailable", envName)
		}
		values[envName] = string(secret)
		for i := range secret {
			secret[i] = 0
		}
	}
	if err := tasks.ApplyEnvironmentOverrides(spec, values); err != nil {
		return err
	}
	for key := range values {
		values[key] = ""
	}
	return nil
}

func (s *Server) authorizeTaskSecretUse(w http.ResponseWriter, r *http.Request, refs map[string]string, taskLabel string) bool {
	if len(refs) == 0 {
		return true
	}
	return s.requireSharedActionPermission(
		w,
		r,
		identity.PermissionSecretsUse,
		"task.secret_use",
		taskLabel,
	)
}
