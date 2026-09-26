package identity

// Permission keys are the server authorization vocabulary. Unknown keys deny.
const (
	PermissionTasksView          = "tasks.view"
	PermissionTasksRun           = "tasks.run"
	PermissionTerminalViewOwn    = "terminal.view_own"
	PermissionTerminalControlOwn = "terminal.control_own"
	PermissionTerminalCreate     = "terminal.create"
	PermissionTerminalViewAll    = "terminal.view_all"
	PermissionTerminalControlAll = "terminal.control_all"
	PermissionPatchView          = "patch.view"
	PermissionPatchRun           = "patch.run"
	PermissionPatchCollect       = "patch.collect"
	PermissionPatchHistory       = "patch.history"
	PermissionPatchCleanup       = "patch.cleanup"
	PermissionFilesRead          = "files.read"
	PermissionFilesDownload      = "files.download"
	PermissionFilesUpload        = "files.upload"
	PermissionFilesWrite         = "files.write"
	PermissionGitStatus          = "git.status"
	PermissionGitLog             = "git.log"
	PermissionGitDiff            = "git.diff"
	PermissionSettingsRead       = "settings.read"
	PermissionSettingsWrite      = "settings.write"
	PermissionSelfupdateCheck    = "selfupdate.check"
	PermissionSelfupdateRun      = "selfupdate.run"
	PermissionUsersView          = "users.view"
	PermissionUsersManage        = "users.manage"
	PermissionRolesView          = "roles.view"
	PermissionRolesManage        = "roles.manage"
	PermissionAuditView          = "audit.view"
	PermissionSessionsManage     = "sessions.manage"
)

type PermissionDefinition struct {
	Key    string `json:"key"`
	Module string `json:"module"`
}

func PermissionRegistry() []PermissionDefinition {
	return []PermissionDefinition{
		{Key: PermissionTasksView, Module: "tasks"},
		{Key: PermissionTasksRun, Module: "tasks"},
		{Key: PermissionTerminalViewOwn, Module: "terminal"},
		{Key: PermissionTerminalControlOwn, Module: "terminal"},
		{Key: PermissionTerminalCreate, Module: "terminal"},
		{Key: PermissionTerminalViewAll, Module: "terminal"},
		{Key: PermissionTerminalControlAll, Module: "terminal"},
		{Key: PermissionPatchView, Module: "patch"},
		{Key: PermissionPatchRun, Module: "patch"},
		{Key: PermissionPatchCollect, Module: "patch"},
		{Key: PermissionPatchHistory, Module: "patch"},
		{Key: PermissionPatchCleanup, Module: "patch"},
		{Key: PermissionFilesRead, Module: "files"},
		{Key: PermissionFilesDownload, Module: "files"},
		{Key: PermissionFilesUpload, Module: "files"},
		{Key: PermissionFilesWrite, Module: "files"},
		{Key: PermissionGitStatus, Module: "git"},
		{Key: PermissionGitLog, Module: "git"},
		{Key: PermissionGitDiff, Module: "git"},
		{Key: PermissionSettingsRead, Module: "settings"},
		{Key: PermissionSettingsWrite, Module: "settings"},
		{Key: PermissionSelfupdateCheck, Module: "selfupdate"},
		{Key: PermissionSelfupdateRun, Module: "selfupdate"},
		{Key: PermissionUsersView, Module: "users"},
		{Key: PermissionUsersManage, Module: "users"},
		{Key: PermissionRolesView, Module: "roles"},
		{Key: PermissionRolesManage, Module: "roles"},
		{Key: PermissionAuditView, Module: "audit"},
		{Key: PermissionSessionsManage, Module: "sessions"},
		{Key: PermissionProjectAdmin, Module: "project"},
	}
}

func KnownPermission(key string) bool {
	for _, permission := range PermissionRegistry() {
		if permission.Key == key {
			return true
		}
	}
	return false
}

type SystemRoleDefinition struct {
	ID          ID
	Name        string
	Permissions []string
}

// Return fresh slices so callers cannot alter the process-wide defaults.
func SystemRoles() []SystemRoleDefinition {
	var all []string
	for _, permission := range PermissionRegistry() {
		all = append(all, permission.Key)
	}
	read := []string{PermissionTasksView, PermissionPatchView, PermissionPatchHistory, PermissionFilesRead, PermissionFilesDownload, PermissionGitStatus, PermissionGitLog, PermissionGitDiff}
	developer := append(append([]string(nil), read...), PermissionTasksRun, PermissionTerminalViewOwn, PermissionTerminalControlOwn, PermissionTerminalCreate, PermissionPatchRun, PermissionPatchCollect, PermissionFilesUpload, PermissionFilesWrite, PermissionSettingsRead, PermissionSelfupdateCheck)
	operator := append(append([]string(nil), read...), PermissionTasksRun, PermissionPatchCollect, PermissionSelfupdateCheck)
	return []SystemRoleDefinition{
		{ID: "system:admin", Name: "admin", Permissions: all},
		{ID: "system:developer", Name: "developer", Permissions: developer},
		{ID: "system:operator", Name: "operator", Permissions: operator},
		{ID: "system:viewer", Name: "viewer", Permissions: read},
	}
}
