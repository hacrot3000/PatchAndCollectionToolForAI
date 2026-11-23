package agentrun

import (
	"path/filepath"
	"sync"
)

// coordinationJournalLocks is the single in-process lock registry shared by
// CoordinationHub and migration on every platform. The advisory file lock,
// where available, is always acquired after this mutex.
var coordinationJournalLocks sync.Map

func coordinationJournalMutex(path string) *sync.Mutex {
	key := filepath.Clean(path)
	value, _ := coordinationJournalLocks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// LockCoordinationJournalProcess locks the canonical in-process journal mutex.
func LockCoordinationJournalProcess(path string) func() {
	mutex := coordinationJournalMutex(path)
	mutex.Lock()
	return mutex.Unlock
}
