package server

import "bletonfc/vscode_tasks_menu/internal/agentchat"

// agentInitialOutputBudget allocates substantial headroom for long-running
// developer tasks (including complete native Patch and Collection calls).
// This is an upper limit, NOT a request to always generate this much text.
// The configured model's output ceiling remains authoritative; the provider
// may enforce a lower limit than the local configuration advertises.
func agentInitialOutputBudget(model agentchat.ModelConfig, effort string, toolsEnabled bool) int {
 budget := 65536
 if toolsEnabled {
  switch effort {
  case "high", "xhigh", "max":
   budget = 98304
  }
 }
 if model.MaxOutputTokens > 0 && model.MaxOutputTokens < budget {
  budget = model.MaxOutputTokens
 }
 // The request builder caps output at 200k, independent of model metadata.
 if budget > 200000 {
  budget = 200000
 }
 return budget
}
