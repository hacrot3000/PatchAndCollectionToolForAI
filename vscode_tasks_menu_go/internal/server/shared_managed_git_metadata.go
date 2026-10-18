package server

// Git object databases can outlive deleted source files. Shared users with
// Git read privileges must not enumerate historical TaskDeck-managed object
// IDs or sensitive paths through branch-tree/commit metadata endpoints.
func filterManagedGitTreeEntries(entries []gitTreeCompareEntry) []gitTreeCompareEntry {
    filtered:=make([]gitTreeCompareEntry,0,len(entries))
    for _,entry:=range entries {
        if !isTaskDeckManagedProjectPath(entry.Path){filtered=append(filtered,entry)}
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
