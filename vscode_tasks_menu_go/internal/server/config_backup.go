package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const (
	configBackupVersion  = 1
	configBackupMaxBytes = 8 << 20
)

type configBackupSettings struct {
	PageTitle        string                          `json:"page_title,omitempty"`
	TerminalCWD      config.TerminalCWDSettings      `json:"terminal_cwd"`
	RunningIndicator config.RunningIndicatorSettings `json:"running_indicator"`
	SelfUpdate       config.SelfUpdateSettings       `json:"self_update"`
}

type configBackupSSHProfile struct {
	sshprofile.Profile
	CredentialRequired bool `json:"credential_required,omitempty"`
}
type configBackupDBProfile struct {
	dbprofile.Profile
	CredentialRequired bool `json:"credential_required,omitempty"`
}
type configBackupTransferProfile struct {
	filetransferprofile.Profile
	CredentialRequired bool `json:"credential_required,omitempty"`
}
type configBackupConnections struct {
	SSH          []configBackupSSHProfile      `json:"ssh,omitempty"`
	Database     []configBackupDBProfile       `json:"database,omitempty"`
	FileTransfer []configBackupTransferProfile `json:"file_transfer,omitempty"`
}

type configBackupClient struct {
	EnvironmentProfiles map[string]map[string]string `json:"environment_profiles,omitempty"`
	ActiveEnvironment    string                       `json:"active_environment,omitempty"`
}

type configBackupBundle struct {
	Version         int                       `json:"version"`
	GeneratedAt     string                    `json:"generated_at"`
	Settings        configBackupSettings      `json:"settings"`
	CommandPresets  projectCommandPresetState `json:"command_presets"`
	TaskState       projectTaskState           `json:"task_state"`
	TerminalDesktop projectTerminalState       `json:"terminal_desktop"`
	TerminalMobile  projectTerminalState       `json:"terminal_mobile"`
	ProjectProfiles projectProfileStore        `json:"project_profiles"`
	Connections     configBackupConnections    `json:"connections"`
	Client          configBackupClient         `json:"client,omitempty"`
	IncludesEnvironment bool                    `json:"includes_environment,omitempty"`
}

type configRestoreRequest struct {
	Mode   string             `json:"mode,omitempty"`
	Bundle configBackupBundle `json:"bundle"`
}

type localConfigRestoreState struct {
	Bundle       configBackupBundle
	SSH          []sshprofile.Profile
	Database     []dbprofile.Profile
	FileTransfer []filetransferprofile.Profile
}

func stripConnectionSecrets(sshProfiles []sshprofile.Profile, dbProfiles []dbprofile.Profile, transferProfiles []filetransferprofile.Profile) configBackupConnections {
	out := configBackupConnections{}
	for _, profile := range sshProfiles {
		required := strings.TrimSpace(profile.SecretRef) != ""
		profile.SecretRef = ""
		out.SSH = append(out.SSH, configBackupSSHProfile{Profile: profile, CredentialRequired: required})
	}
	for _, profile := range dbProfiles {
		required := strings.TrimSpace(profile.SecretRef) != ""
		profile.SecretRef = ""
		out.Database = append(out.Database, configBackupDBProfile{Profile: profile, CredentialRequired: required})
	}
	for _, profile := range transferProfiles {
		required := strings.TrimSpace(profile.SecretRef) != ""
		profile.SecretRef = ""
		out.FileTransfer = append(out.FileTransfer, configBackupTransferProfile{Profile: profile, CredentialRequired: required})
	}
	return out
}

func portableTerminalState(value projectTerminalState) projectTerminalState {
	value = normalizeProjectTerminalState(value)
	for i := range value.Terminals {
		value.Terminals[i].SessionID = ""
	}
	value.LiveSplits = nil
	value.Split = nil
	return value
}

func (s *Server) captureConfigBackup() (configBackupBundle, localConfigRestoreState, error) {
	var out configBackupBundle
	out.Version = configBackupVersion
	out.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	pageTitle, err := config.ReadPageTitle(s.Workspace)
	if err != nil { return out, localConfigRestoreState{}, err }
	terminalCWD, err := config.ReadTerminalCWDSettings(s.Workspace)
	if err != nil { return out, localConfigRestoreState{}, err }
	indicator, err := config.ReadRunningIndicatorSettings(s.Workspace)
	if err != nil { return out, localConfigRestoreState{}, err }
	selfUpdate, err := config.ReadSelfUpdateSettings(s.Workspace)
	if err != nil { return out, localConfigRestoreState{}, err }
	out.Settings = configBackupSettings{
		PageTitle: pageTitle, TerminalCWD: terminalCWD,
		RunningIndicator: indicator, SelfUpdate: selfUpdate,
	}

	if out.CommandPresets, err = readProjectCommandPresetState(s.Workspace); err != nil { return out, localConfigRestoreState{}, err }
	if out.TaskState, err = readProjectTaskState(s.Workspace); err != nil { return out, localConfigRestoreState{}, err }
	if out.TerminalDesktop, err = readProjectTerminalStateProfile(s.Workspace, "desktop"); err != nil { return out, localConfigRestoreState{}, err }
	if out.TerminalMobile, err = readProjectTerminalStateProfile(s.Workspace, "mobile"); err != nil { return out, localConfigRestoreState{}, err }
	out.TerminalDesktop = portableTerminalState(out.TerminalDesktop)
	out.TerminalMobile = portableTerminalState(out.TerminalMobile)
	if out.ProjectProfiles, err = readProjectProfileStore(s.Workspace); err != nil { return out, localConfigRestoreState{}, err }

	sshStore, err := s.sshProfileStore()
	if err != nil { return out, localConfigRestoreState{}, err }
	dbStore, err := s.databaseProfileStore()
	if err != nil { return out, localConfigRestoreState{}, err }
	transferStore, err := s.fileTransferProfileStore()
	if err != nil { return out, localConfigRestoreState{}, err }
	sshProfiles, err := sshStore.Load()
	if err != nil { return out, localConfigRestoreState{}, err }
	dbProfiles, err := dbStore.Load()
	if err != nil { return out, localConfigRestoreState{}, err }
	transferProfiles, err := transferStore.Load()
	if err != nil { return out, localConfigRestoreState{}, err }

	actual := localConfigRestoreState{
		Bundle: out,
		SSH: append([]sshprofile.Profile(nil), sshProfiles...),
		Database: append([]dbprofile.Profile(nil), dbProfiles...),
		FileTransfer: append([]filetransferprofile.Profile(nil), transferProfiles...),
	}
	out.Connections = stripConnectionSecrets(sshProfiles, dbProfiles, transferProfiles)
	return out, actual, nil
}

func normalizeRestoreMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "merge":
		return "merge", nil
	case "replace":
		return "replace", nil
	default:
		return "", errors.New("restore mode must be merge or replace")
	}
}

func missingImportedSecretRef(kind, id string) string {
	id = strings.TrimSpace(id)
	if id == "" { id = "unknown" }
	return "managed/import/" + kind + "/" + id
}

func mergeCommandPresets(current, incoming projectCommandPresetState) (projectCommandPresetState, error) {
	byID := map[string]commandPreset{}
	order := []string{}
	for _, preset := range current.Presets {
		if _, exists := byID[preset.ID]; !exists { order=append(order,preset.ID) }
		byID[preset.ID]=preset
	}
	for _, preset := range incoming.Presets {
		if _, exists := byID[preset.ID]; !exists { order=append(order,preset.ID) }
		byID[preset.ID]=preset
	}
	out:=projectCommandPresetState{Version:projectCommandPresetVersion}
	for _, id:=range order { out.Presets=append(out.Presets,byID[id]) }
	return normalizeProjectCommandPresetState(out)
}

func mergeProjectProfiles(current, incoming projectProfileStore, replace, includesEnvironment bool) projectProfileStore {
	byID:=map[string]projectProfile{};order:=[]string{}
	currentByID:=map[string]projectProfile{}
	for _, profile:=range current.Profiles {
		currentByID[profile.ID]=profile
		if !replace {
			if _,exists:=byID[profile.ID];!exists{order=append(order,profile.ID)}
			byID[profile.ID]=profile
		}
	}
	for _, profile:=range incoming.Profiles {
		if !includesEnvironment {
			if existing,ok:=currentByID[profile.ID];ok {
				profile.Environment=existing.Environment
				profile.EnvironmentProfile=existing.EnvironmentProfile
			} else {
				profile.Environment=nil
				profile.EnvironmentProfile=""
			}
		}
		if _,exists:=byID[profile.ID];!exists{order=append(order,profile.ID)}
		byID[profile.ID]=profile
	}
	out:=projectProfileStore{Version:1}
	for _,id:=range order{out.Profiles=append(out.Profiles,byID[id])}
	return out
}

func validateBackupProjectProfiles(store projectProfileStore) (projectProfileStore,error) {
	if len(store.Profiles)>projectProfilesMaxCount{return store,fmt.Errorf("project profiles exceed %d entries",projectProfilesMaxCount)}
	seen:=map[string]bool{}
	store.Version=1
	for i:=range store.Profiles{
		store.Profiles[i]=normalizeProjectProfile(store.Profiles[i])
		p:=store.Profiles[i]
		if p.ID==""||p.Name==""||seen[p.ID]{return store,fmt.Errorf("project profile %d has invalid or duplicate identity",i+1)}
		if err:=validateProjectProfileEnvironment(p.Environment);err!=nil{return store,err}
		seen[p.ID]=true
	}
	return store,nil
}

func mergeTaskState(current, incoming projectTaskState) projectTaskState {
	out:=incoming
	out.Favorites=append(append([]int{},incoming.Favorites...),current.Favorites...)
	out.Recent=append(append([]int{},incoming.Recent...),current.Recent...)
	out.History=append(append([]taskHistoryItem{},incoming.History...),current.History...)
	return normalizeProjectTaskState(out)
}

func mergeSSHProfiles(current []sshprofile.Profile, incoming []configBackupSSHProfile, replace bool) ([]sshprofile.Profile, error) {
	byID := map[string]sshprofile.Profile{}
	if !replace { for _, p := range current { byID[p.ID] = p } }
	currentSecrets := map[string]string{}
	for _, p := range current { currentSecrets[p.ID] = p.SecretRef }
	for _, item := range incoming {
		p := item.Profile
		p.SecretRef = currentSecrets[p.ID]
		if p.SecretRef == "" && item.CredentialRequired {
			p.SecretRef = missingImportedSecretRef("ssh", p.ID)
		}
		normalized, err := sshprofile.Normalize(p)
		if err != nil { return nil, err }
		byID[normalized.ID] = normalized
	}
	out := make([]sshprofile.Profile,0,len(byID))
	for _, p := range byID { out=append(out,p) }
	return out,nil
}

func mergeDBProfiles(current []dbprofile.Profile, incoming []configBackupDBProfile, replace bool) ([]dbprofile.Profile, error) {
	byID := map[string]dbprofile.Profile{}
	if !replace { for _, p := range current { byID[p.ID] = p } }
	currentSecrets := map[string]string{}
	for _, p := range current { currentSecrets[p.ID] = p.SecretRef }
	for _, item := range incoming {
		p := item.Profile
		p.SecretRef = currentSecrets[p.ID]
		if p.SecretRef == "" && item.CredentialRequired {
			p.SecretRef = missingImportedSecretRef("database", p.ID)
		}
		normalized, err := dbprofile.Normalize(p)
		if err != nil { return nil, err }
		byID[normalized.ID] = normalized
	}
	out := make([]dbprofile.Profile,0,len(byID))
	for _, p := range byID { out=append(out,p) }
	return out,nil
}

func mergeTransferProfiles(current []filetransferprofile.Profile, incoming []configBackupTransferProfile, replace bool) ([]filetransferprofile.Profile, error) {
	byID := map[string]filetransferprofile.Profile{}
	if !replace { for _, p := range current { byID[p.ID] = p } }
	currentSecrets := map[string]string{}
	for _, p := range current { currentSecrets[p.ID] = p.SecretRef }
	for _, item := range incoming {
		p := item.Profile
		p.SecretRef = currentSecrets[p.ID]
		if p.SecretRef == "" && item.CredentialRequired {
			p.SecretRef = missingImportedSecretRef("file-transfer", p.ID)
		}
		normalized, err := filetransferprofile.Normalize(p)
		if err != nil { return nil, err }
		byID[normalized.ID] = normalized
	}
	out := make([]filetransferprofile.Profile,0,len(byID))
	for _, p := range byID { out=append(out,p) }
	return out,nil
}

func validateConfigBackupBundle(bundle configBackupBundle) (configBackupBundle, error) {
	if bundle.Version != configBackupVersion {
		return bundle, fmt.Errorf("unsupported TaskDeck config backup version %d", bundle.Version)
	}
	var err error
	if bundle.Settings.TerminalCWD, err = config.NormalizeTerminalCWDSettings(bundle.Settings.TerminalCWD); err != nil { return bundle, err }
	if bundle.Settings.RunningIndicator, err = config.NormalizeRunningIndicatorSettings(bundle.Settings.RunningIndicator); err != nil { return bundle, err }
	if bundle.Settings.SelfUpdate.Branch, err = config.NormalizeSelfUpdateBranch(bundle.Settings.SelfUpdate.Branch); err != nil { return bundle, err }
	if bundle.CommandPresets, err = normalizeProjectCommandPresetState(bundle.CommandPresets); err != nil { return bundle, err }
	bundle.TaskState = normalizeProjectTaskState(bundle.TaskState)
	bundle.TerminalDesktop = portableTerminalState(bundle.TerminalDesktop)
	bundle.TerminalMobile = portableTerminalState(bundle.TerminalMobile)
	if bundle.ProjectProfiles,err=validateBackupProjectProfiles(bundle.ProjectProfiles);err!=nil{return bundle,err}
	if !bundle.IncludesEnvironment {
		for i:=range bundle.ProjectProfiles.Profiles{
			bundle.ProjectProfiles.Profiles[i].Environment=nil
			bundle.ProjectProfiles.Profiles[i].EnvironmentProfile=""
		}
		bundle.Client=configBackupClient{}
	}
	for i := range bundle.Connections.SSH {
		bundle.Connections.SSH[i].Profile.SecretRef = ""
		p := bundle.Connections.SSH[i].Profile
		if bundle.Connections.SSH[i].CredentialRequired { p.SecretRef = missingImportedSecretRef("ssh",p.ID) }
		if _, err := sshprofile.Normalize(p); err != nil { return bundle, err }
	}
	for i := range bundle.Connections.Database {
		bundle.Connections.Database[i].Profile.SecretRef = ""
		p := bundle.Connections.Database[i].Profile
		if bundle.Connections.Database[i].CredentialRequired { p.SecretRef = missingImportedSecretRef("database",p.ID) }
		if _, err := dbprofile.Normalize(p); err != nil { return bundle, err }
	}
	for i := range bundle.Connections.FileTransfer {
		bundle.Connections.FileTransfer[i].Profile.SecretRef = ""
		p := bundle.Connections.FileTransfer[i].Profile
		if bundle.Connections.FileTransfer[i].CredentialRequired { p.SecretRef = missingImportedSecretRef("file-transfer",p.ID) }
		if _, err := filetransferprofile.Normalize(p); err != nil { return bundle, err }
	}
	return bundle,nil
}

func (s *Server) applyConfigBackup(bundle configBackupBundle, mode string, actual localConfigRestoreState) error {
	replace := mode == "replace"
	sshProfiles, err := mergeSSHProfiles(actual.SSH, bundle.Connections.SSH, replace)
	if err != nil { return err }
	dbProfiles, err := mergeDBProfiles(actual.Database, bundle.Connections.Database, replace)
	if err != nil { return err }
	transferProfiles, err := mergeTransferProfiles(actual.FileTransfer, bundle.Connections.FileTransfer, replace)
	if err != nil { return err }

	sshStore, err := s.sshProfileStore(); if err != nil { return err }
	dbStore, err := s.databaseProfileStore(); if err != nil { return err }
	transferStore, err := s.fileTransferProfileStore(); if err != nil { return err }

	presets:=bundle.CommandPresets
	taskState:=bundle.TaskState
	projectProfiles:=bundle.ProjectProfiles
	if replace {
		projectProfiles=mergeProjectProfiles(actual.Bundle.ProjectProfiles,bundle.ProjectProfiles,true,bundle.IncludesEnvironment)
	}
	if !replace {
		if presets,err=mergeCommandPresets(actual.Bundle.CommandPresets,bundle.CommandPresets);err!=nil{return err}
		taskState=mergeTaskState(actual.Bundle.TaskState,bundle.TaskState)
		projectProfiles=mergeProjectProfiles(actual.Bundle.ProjectProfiles,bundle.ProjectProfiles,false,bundle.IncludesEnvironment)
	}
	if err := config.SetPageTitle(s.Workspace, bundle.Settings.PageTitle); err != nil { return err }
	if err := config.SetTerminalCWDSettings(s.Workspace, bundle.Settings.TerminalCWD); err != nil { return err }
	if err := config.SetRunningIndicatorSettings(s.Workspace, bundle.Settings.RunningIndicator); err != nil { return err }
	if err := config.SetSelfUpdateSettings(s.Workspace, bundle.Settings.SelfUpdate); err != nil { return err }
	if err := writeProjectCommandPresetState(s.Workspace, presets); err != nil { return err }
	if err := writeProjectTaskState(s.Workspace, taskState); err != nil { return err }
	if err := writeProjectTerminalStateProfile(s.Workspace, "desktop", bundle.TerminalDesktop); err != nil { return err }
	if err := writeProjectTerminalStateProfile(s.Workspace, "mobile", bundle.TerminalMobile); err != nil { return err }
	if err := writeProjectProfileStore(s.Workspace, projectProfiles); err != nil { return err }
	if err := sshStore.Save(sshProfiles); err != nil { return err }
	if err := dbStore.Save(dbProfiles); err != nil { return err }
	if err := transferStore.Save(transferProfiles); err != nil { return err }
	return nil
}

func (s *Server) rollbackConfigRestore(actual localConfigRestoreState) error {
	sshStore, err := s.sshProfileStore(); if err != nil { return err }
	dbStore, err := s.databaseProfileStore(); if err != nil { return err }
	transferStore, err := s.fileTransferProfileStore(); if err != nil { return err }
	var errs []string
	if err := config.SetPageTitle(s.Workspace, actual.Bundle.Settings.PageTitle); err != nil { errs=append(errs,err.Error()) }
	if err := config.SetTerminalCWDSettings(s.Workspace, actual.Bundle.Settings.TerminalCWD); err != nil { errs=append(errs,err.Error()) }
	if err := config.SetRunningIndicatorSettings(s.Workspace, actual.Bundle.Settings.RunningIndicator); err != nil { errs=append(errs,err.Error()) }
	if err := config.SetSelfUpdateSettings(s.Workspace, actual.Bundle.Settings.SelfUpdate); err != nil { errs=append(errs,err.Error()) }
	if err := writeProjectCommandPresetState(s.Workspace, actual.Bundle.CommandPresets); err != nil { errs=append(errs,err.Error()) }
	if err := writeProjectTaskState(s.Workspace, actual.Bundle.TaskState); err != nil { errs=append(errs,err.Error()) }
	if err := writeProjectTerminalStateProfile(s.Workspace,"desktop",actual.Bundle.TerminalDesktop); err != nil { errs=append(errs,err.Error()) }
	if err := writeProjectTerminalStateProfile(s.Workspace,"mobile",actual.Bundle.TerminalMobile); err != nil { errs=append(errs,err.Error()) }
	if err := writeProjectProfileStore(s.Workspace, actual.Bundle.ProjectProfiles); err != nil { errs=append(errs,err.Error()) }
	if err := sshStore.Save(actual.SSH); err != nil { errs=append(errs,err.Error()) }
	if err := dbStore.Save(actual.Database); err != nil { errs=append(errs,err.Error()) }
	if err := transferStore.Save(actual.FileTransfer); err != nil { errs=append(errs,err.Error()) }
	if len(errs)>0 { return errors.New(strings.Join(errs,"; ")) }
	return nil
}

func (s *Server) configBackup(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		bundle, _, err := s.captureConfigBackup()
		if err != nil { http.Error(w,err.Error(),http.StatusInternalServerError); return }
		bundle.IncludesEnvironment = r.URL.Query().Get("include_environment") == "1"
		if !bundle.IncludesEnvironment {
			for i:=range bundle.ProjectProfiles.Profiles{
				bundle.ProjectProfiles.Profiles[i].Environment=nil
				bundle.ProjectProfiles.Profiles[i].EnvironmentProfile=""
			}
		}
		writeJSON(w,http.StatusOK,bundle)
	case http.MethodPost:
		var req configRestoreRequest
		dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,configBackupMaxBytes))
		dec.DisallowUnknownFields()
		if err:=dec.Decode(&req); err!=nil { http.Error(w,"invalid config backup JSON",http.StatusBadRequest); return }
		mode,err:=normalizeRestoreMode(req.Mode)
		if err!=nil { http.Error(w,err.Error(),http.StatusBadRequest); return }
		bundle,err:=validateConfigBackupBundle(req.Bundle)
		if err!=nil { http.Error(w,err.Error(),http.StatusBadRequest); return }
		_,actual,err:=s.captureConfigBackup()
		if err!=nil { http.Error(w,err.Error(),http.StatusInternalServerError); return }
		if err:=s.applyConfigBackup(bundle,mode,actual); err!=nil {
			if rollbackErr:=s.rollbackConfigRestore(actual); rollbackErr!=nil {
				http.Error(w,fmt.Sprintf("restore failed: %v; rollback failed: %v",err,rollbackErr),http.StatusInternalServerError)
				return
			}
			http.Error(w,"restore failed and was rolled back: "+err.Error(),http.StatusConflict)
			return
		}
		s.auditSharedSuccess(r,"settings.backup.restore","configuration","project",map[string]any{
			"mode":mode,
			"ssh_profiles":len(bundle.Connections.SSH),
			"database_profiles":len(bundle.Connections.Database),
			"transfer_profiles":len(bundle.Connections.FileTransfer),
		})
		writeJSON(w,http.StatusOK,map[string]any{"ok":true,"mode":mode,"restart_recommended":true})
	default:
		http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
	}
}
