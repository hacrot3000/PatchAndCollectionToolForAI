package server

import "strings"

// Git object databases can outlive deleted source files. Shared users with
// Git read privileges must not enumerate historical TaskDeck-managed object
// IDs or sensitive paths through branch-tree/commit metadata endpoints.
func filterManagedGitTreeEntries(entries []gitTreeCompareEntry) []gitTreeCompareEntry {
    // A parent tree OID changes when a protected child changes. Keep the
    // ordinary parent folder browsable but strip its aggregate object ID,
    // otherwise a files.read user can fingerprint private configuration.
    protectedParents:=map[string]bool{}
    for _,entry:=range entries {
        if !isTaskDeckManagedProjectPath(entry.Path){continue}
        parent:=entry.Path
        for {
            slash:=strings.LastIndexByte(parent,'/')
            if slash<0 {break}
            parent=parent[:slash]
            protectedParents[parent]=true
        }
    }
    filtered:=make([]gitTreeCompareEntry,0,len(entries))
    for _,entry:=range entries {
        if isTaskDeckManagedProjectPath(entry.Path){continue}
        if entry.Kind=="tree"&&protectedParents[entry.Path] {entry.OID=""}
        filtered=append(filtered,entry)
    }
    return filtered
}

func filterManagedGitDiffChanges(changes []gitTreeDiffChange) []gitTreeDiffChange {
    filtered:=make([]gitTreeDiffChange,0,len(changes))
    for _,entry:=range changes {
        if !isTaskDeckManagedProjectPath(entry.Path){filtered=append(filtered,entry)}
    }
    return filtered
}

func filterManagedGitGraphFiles(files []gitGraphFile) []gitGraphFile {
    filtered:=make([]gitGraphFile,0,len(files))
    for _,entry:=range files {
        if isTaskDeckManagedProjectPath(entry.Path)||isTaskDeckManagedProjectPath(entry.OldPath){continue}
        filtered=append(filtered,entry)
    }
    return filtered
}

func (s *Server) sharedGitGraphFiles(raw string) []gitGraphFile {
    files:=parseGitGraphFiles(raw)
    if s.Config.SharedServerEnabled {return filterManagedGitGraphFiles(files)}
    return files
}

func filterManagedGitStatusChanges(changes []gitChange) []gitChange {
    filtered:=make([]gitChange,0,len(changes))
    for _,entry:=range changes {
        if isTaskDeckManagedProjectPath(entry.Path)||isTaskDeckManagedProjectPath(entry.OriginalPath){continue}
        filtered=append(filtered,entry)
    }
    return filtered
}

func filterManagedGitConflictFiles(files []gitConflictFile) []gitConflictFile {
    filtered:=make([]gitConflictFile,0,len(files))
    for _,item:=range files {
        if isTaskDeckManagedProjectPath(item.Path)||isTaskDeckManagedProjectPath(item.ProjectPath){continue}
        filtered=append(filtered,item)
    }
    return filtered
}
