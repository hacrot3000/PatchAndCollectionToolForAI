package agentchat

// RequestDisplay is trusted, optional local UI metadata. It is neither
// provider input nor part of tool authorization, receipt identity or retry
// decisions. All fields are comparable; old journals decode as empty.
type RequestDisplay struct {
 ProviderID string `json:"provider_id,omitempty"`
 ModelID string `json:"model_id,omitempty"`
 RequestEffort string `json:"request_effort,omitempty"`
 RequestContext string `json:"request_context,omitempty"`
 RequestAt string `json:"request_at,omitempty"`
 RequestInputTokens int `json:"request_input_tokens,omitempty"`
}

func (d RequestDisplay) WithHistoryItem(item HistoryItem) HistoryItem {
 if d.RequestAt=="" {return item}
 item.ProviderID=d.ProviderID
 item.ModelID=d.ModelID
 item.RequestEffort=d.RequestEffort
 item.RequestContext=d.RequestContext
 item.RequestAt=d.RequestAt
 item.RequestInputTokens=d.RequestInputTokens
 return item
}
