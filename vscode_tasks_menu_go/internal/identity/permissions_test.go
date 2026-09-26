package identity

import "testing"

func TestPermissionRegistryDeniesUnknownAndRoleDefaultsLimitPrivilege(t *testing.T) {
	if (Principal{Permissions: map[string]bool{"invented.allow_all": true}}).Allowed("invented.allow_all") {
		t.Fatal("unknown permission allowed")
	}
	for _, role := range SystemRoles() {
		p := Principal{Permissions: map[string]bool{}}
		for _, key := range role.Permissions {
			if !KnownPermission(key) || p.Permissions[key] {
				t.Fatalf("invalid role grant %s/%s", role.Name, key)
			}
			p.Permissions[key] = true
		}
		if role.Name == "admin" {
			for _, permission := range PermissionRegistry() {
				if !p.Allowed(permission.Key) {
					t.Fatalf("admin missing %s", permission.Key)
				}
			}
		} else if p.Allowed(PermissionUsersManage) || p.Allowed(PermissionRolesManage) || p.Allowed(PermissionTerminalControlAll) || p.Allowed(PermissionSelfupdateRun) {
			t.Fatalf("privileged default grant in %s", role.Name)
		}
		if role.Name == "viewer" && (p.Allowed(PermissionTasksRun) || p.Allowed(PermissionFilesWrite) || p.Allowed(PermissionPatchRun)) {
			t.Fatal("viewer can mutate workspace")
		}
	}
}
