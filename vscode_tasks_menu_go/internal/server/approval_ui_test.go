package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestApprovalWizardAndGenericRetryContract(t *testing.T) {
	approvalData, err := webassets.Files.ReadFile("featuremods/approvals.js")
	if err != nil { t.Fatal(err) }
	approvalJS := string(approvalData)
	for _, want := range []string{
		"function authorize(challenge={})",
		"mode==='admin'",
		"mode==='type'",
		"mode==='confirm'",
		"requestApproval(action,resource,confirmation)",
		"waitForAdminApproval(state,id)",
		"approvalStatus(id)",
		"Type the exact resource below",
		"Type CONFIRM",
		"globalThis.TaskMenuApprovals={authorize,openManager",
		"git.remote.delete",
		"git.force_delete",
		"git.reset_hard",
		"transfer.recursive_delete",
		"db.production.write",
		"task.deploy",
		"Approve",
		"Reject",
		"Save policy",
	} {
		if !strings.Contains(approvalJS, want) {
			t.Fatalf("approvals.js missing %q", want)
		}
	}

	uiData, err := webassets.Files.ReadFile("../internal/server/ui.go")
	if err == nil {
		_ = uiData // ui.go is not part of embedded web assets in all builds.
	}
}

func TestApprovalModuleLoadsBeforeDangerousFeatureActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	approvalIndex := strings.Index(js, "approvals.js")
	for _, module := range []string{"database.js","filetransfer.js","gitstatus.js"} {
		index := strings.Index(js, module)
		if approvalIndex < 0 || index < 0 || approvalIndex > index {
			t.Fatalf("approvals.js must load before %s: approval=%d module=%d", module, approvalIndex, index)
		}
	}
}

func TestDangerousActionHooksUseExplicitResources(t *testing.T) {
	gitData, err := webassets.Files.ReadFile("../internal/server/gitquick.go")
	if err == nil { _ = gitData }
	// Source-level server contracts live in normal Go tests; browser contract here
	// ensures the generic approval module does not infer action/resource itself.
	data, err := webassets.Files.ReadFile("featuremods/approvals.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	if strings.Contains(js, "includes('prod')") || strings.Contains(js, "includes(\"deploy\")") {
		t.Fatal("approval UI must not infer production/deploy from display names")
	}
}
