package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"

	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

const agentRunWorkspacePackRefPrefix = "execution-pack-sha256:"

func agentRunWorkspacePackSourceRef(workspacePath, projectID string, customBuilder func(string, string) (patchtool.AIPackResult, error)) (string, error) {
	if workspacePath == "" || projectID == "" {
		return "", errors.New("execution workspace binding is incomplete")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	builder := patchtool.BuildAIPack
	if customBuilder != nil {
		builder = customBuilder
	}
	pack, err := builder(workspacePath, executable)
	if err != nil {
		return "", err
	}
	if pack.ProjectIdentity.ID != projectID {
		return "", errors.New("execution workspace AI Pack identity mismatch")
	}
	if len(pack.TextContent) > 400<<10 {
		return "", errors.New("execution workspace AI Pack exceeds run context limit")
	}
	content := scrubAIContext(pack.TextContent, 400<<10)
	return agentRunWorkspacePackRef(content), nil
}

func agentRunWorkspacePackRef(content string) string {
	digest := sha256.Sum256([]byte(content))
	return agentRunWorkspacePackRefPrefix + hex.EncodeToString(digest[:])
}

func verifyAgentRunWorkspacePackSnapshot(sourceRefs []string, currentRef string) error {
	for _, ref := range sourceRefs {
		if len(ref) >= len(agentRunWorkspacePackRefPrefix) && ref[:len(agentRunWorkspacePackRefPrefix)] == agentRunWorkspacePackRefPrefix {
			if ref != currentRef {
				return errors.New("execution workspace context changed after the run snapshot was confirmed")
			}
			return nil
		}
	}
	return errors.New("run context snapshot does not pin the execution workspace pack")
}
