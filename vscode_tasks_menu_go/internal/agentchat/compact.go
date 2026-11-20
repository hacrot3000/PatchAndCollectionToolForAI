package agentchat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CompactionConfig keeps the newest turns plus any tool exchange that is not
// yet resolved. Nothing here can alter the immutable system instructions,
// project identity or the pending user input; those are supplied separately.
type CompactionConfig struct {
	// KeepTail is the minimum number of trailing history items to retain.
	KeepTail int
	// MaxDropped bounds how much a single pass may remove.
	MaxDropped int
}

// CompactionResult describes exactly what a pass removed so the decision is
// auditable even though the dropped text is no longer replayed to a provider.
type CompactionResult struct {
	History    []HistoryItem
	Dropped    int
	DroppedSHA string
}

// CompactHistory removes only completed plain text turns from the oldest end.
// It never drops a function_call, a function_call_output, or an assistant
// message that still precedes an unresolved tool call. If no safe candidate
// exists the history is returned unchanged and Dropped is zero, so the caller
// fails closed (e.g. HTTP 413) instead of silently truncating a commitment.
func CompactHistory(history []HistoryItem, cfg CompactionConfig) CompactionResult {
	if cfg.KeepTail <= 0 {
		cfg.KeepTail = 64
	}
	if cfg.MaxDropped <= 0 {
		cfg.MaxDropped = 32
	}
	if len(history) <= cfg.KeepTail {
		return CompactionResult{History: history}
	}
	// Everything from the first tool exchange onward is protected: a
	// function_call and its output must never be separated, and the assistant
	// text that introduced the call belongs to the same commitment. Also
	// protect the tail. If nothing safe remains, refuse to compact.
	protectedFrom := len(history)
	for i := range history {
		if history[i].Type == "function_call" || history[i].Type == "function_call_output" {
			protectedFrom = i
			break
		}
	}
	if protectedFrom > 0 && protectedFrom < len(history) && history[protectedFrom-1].Type == "assistant" {
		protectedFrom--
	}
	if tailFrom := len(history) - cfg.KeepTail; tailFrom < protectedFrom {
		protectedFrom = tailFrom
	}
	if protectedFrom <= 0 {
		return CompactionResult{History: history}
	}
	dropped := make([]HistoryItem, 0, protectedFrom)
	kept := make([]HistoryItem, 0, len(history))
	for i, item := range history {
		if i < protectedFrom && (item.Type == "user" || item.Type == "assistant") {
			dropped = append(dropped, item)
			continue
		}
		kept = append(kept, item)
	}
	if len(dropped) == 0 || len(dropped) > cfg.MaxDropped {
		return CompactionResult{History: history}
	}

	raw, _ := json.Marshal(dropped)
	sum := sha256.Sum256(raw)
	return CompactionResult{
		History:    kept,
		Dropped:    len(dropped),
		DroppedSHA: hex.EncodeToString(sum[:]),
	}
}
