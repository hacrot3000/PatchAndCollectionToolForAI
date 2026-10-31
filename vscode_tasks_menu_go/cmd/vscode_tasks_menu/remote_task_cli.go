package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

// runHostRemoteTaskCLI is an offline host-only inspection tool. It never
// changes daemon approvals and never contacts PHP/Relay. The operator must
// review the exact task definition before pinning the complete digest in the
// daemon's private environment and restarting it.
func runHostRemoteTaskCLI(args []string) error {
	usage := errors.New("usage: taskdeck remote-task [--workspace /absolute/project] list")
	flags := flag.NewFlagSet("remote-task", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspace := flags.String("workspace", "", "absolute TaskDeck workspace (host only)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) != 1 || flags.Args()[0] != "list" {
		return usage
	}
	ws, err := resolveWorkspace(*workspace)
	if err != nil {
		return err
	}
	items, err := tasks.Load(ws)
	if err != nil {
		return err
	}
	found := 0
	for _, task := range items {
		if task.Type != "process" || task.Raw == nil ||
			task.Raw["taskdeckRemoteAllowed"] != true ||
			task.Raw["dependsOn"] != nil || task.Raw["taskdeckApproval"] != nil ||
			len(task.Inputs) != 0 || len(task.SecretEnv) != 0 ||
			task.Options["env"] != nil || task.Options["shell"] != nil {
			continue
		}
		command, ok := task.Command.(string)
		if !ok || strings.Contains(command, "${env:") {
			continue
		}
		dynamic := false
		if task.Args != nil {
			values, ok := task.Args.([]any)
			if !ok {
				dynamic = true
			} else {
				for _, raw := range values {
					value, ok := raw.(string)
					if !ok || strings.Contains(value, "${env:") {
						dynamic = true
						break
					}
				}
			}
		}
		if dynamic {
			continue
		}
		spec, err := tasks.ResolveExecution(task, ws)
		if err != nil || filepath.Clean(spec.Cwd) != filepath.Clean(ws) ||
			spec.TargetType != "" || spec.TargetProfileID != "" ||
			spec.ProtocolEvents || spec.ProtocolCommands {
			continue
		}
		token, err := tasks.RemoteApprovalToken(task)
		if err != nil {
			continue
		}
		fmt.Printf("%s  label=%q\n", token, task.Label)
		found++
	}
	if found == 0 {
		fmt.Println("No eligible pre-marked process tasks in", ws)
	}
	fmt.Println("Review tasks.json on the host. Approval requires TASKDECK_REMOTE_TASK_RUN=1")
	fmt.Println("and TASKDECK_REMOTE_APPROVED_TASKS=<ID@SHA256[,ID@SHA256]> on the daemon.")
	fmt.Println("The old numeric-only TASKDECK_REMOTE_APPROVED_TASK_IDS is ignored.")
	return nil
}
