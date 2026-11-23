package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func (s *Server) agentCoordinationAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	permission := identity.PermissionSettingsRead
	if r.Method == http.MethodPost {
		permission = identity.PermissionSettingsWrite
	}
	if !s.requireSharedActionPermission(w, r, permission, "agent.coordination", "agent_coordination") {
		return
	}
	hub, err := s.agentCoordinationHub()
	if err != nil {
		http.Error(w, "coordination hub unavailable", 500)
		return
	}
	switch r.Method {
	case http.MethodGet:
		groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
		conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
		if conversationID != "" {
			var validateErr error
			if groupID != "" {
				validateErr = s.validateAgentConversationGroupMember(owner, groupID, conversationID)
			} else {
				validateErr = s.validateAgentConversationOwner(owner, conversationID)
			}
			if validateErr != nil {
				http.Error(w, "conversation coordination data not found", 404)
				return
			}
		}
		after := uint64(0)
		if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
			parsed, parseErr := strconv.ParseUint(raw, 10, 64)
			if parseErr != nil {
				http.Error(w, "invalid event cursor", 400)
				return
			}
			after = parsed
		}
		peers, err := hub.Peers(groupID)
		if err != nil {
			http.Error(w, "cannot list peers", 500)
			return
		}
		messages, err := hub.PeerMessages(conversationID, groupID)
		if err != nil {
			http.Error(w, "cannot list peer messages", 500)
			return
		}
		contracts, err := hub.Contracts(groupID)
		if err != nil {
			http.Error(w, "cannot list contracts", 500)
			return
		}
		artifacts, err := hub.Artifacts()
		if err != nil {
			http.Error(w, "cannot list artifacts", 500)
			return
		}
		dependencies, err := hub.Dependencies()
		if err != nil {
			http.Error(w, "cannot list dependencies", 500)
			return
		}
		events, err := hub.Events(after, 100)
		if err != nil {
			http.Error(w, "cannot list coordination events", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"peers": peers, "messages": messages, "contracts": contracts, "artifacts": artifacts, "dependencies": dependencies, "events": events})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var request struct {
			Kind             string               `json:"kind"`
			MessageKind      string               `json:"message_kind,omitempty"`
			ConversationID   string               `json:"conversation_id"`
			GroupID          string               `json:"group_id,omitempty"`
			ToConversationID string               `json:"to_conversation_id,omitempty"`
			Body             string               `json:"body,omitempty"`
			CorrelationID    string               `json:"correlation_id,omitempty"`
			Status           string               `json:"status,omitempty"`
			RunID            string               `json:"run_id,omitempty"`
			Contract         *agentrun.Contract   `json:"contract,omitempty"`
			Artifact         *agentrun.Artifact   `json:"artifact,omitempty"`
			Dependency       *agentrun.Dependency `json:"dependency,omitempty"`
			ContractKey      string               `json:"contract_key,omitempty"`
			Revision         uint64               `json:"revision,omitempty"`
			IntegratedRef    string               `json:"integrated_ref,omitempty"`
			TestEvidence     string               `json:"test_evidence,omitempty"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&request) != nil {
			http.Error(w, "invalid coordination request", 400)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			http.Error(w, "exactly one coordination object required", 400)
			return
		}
		switch request.Kind {
		case "peer_progress":
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "peer conversation not found for owner", 404)
				return
			}
			if request.GroupID != "" && s.validateAgentConversationGroupMember(owner, request.GroupID, request.ConversationID) != nil {
				http.Error(w, "peer conversation is not a member of the group", 403)
				return
			}
			peer, err := hub.UpsertPeer(agentrun.Peer{ConversationID: request.ConversationID, GroupID: request.GroupID, Status: agentrun.PeerStatus(request.Status), Summary: request.Body, RunID: request.RunID})
			if err != nil {
				http.Error(w, "peer progress rejected", 400)
				return
			}
			writeJSON(w, 200, map[string]any{"peer": peer})
		case "peer_message":
			messageKind := request.MessageKind
			if messageKind == "" {
				messageKind = "ask"
			}
			if s.validateAgentConversationGroupMember(owner, request.GroupID, request.ConversationID) != nil ||
				s.validateAgentConversationGroupMember(owner, request.GroupID, request.ToConversationID) != nil {
				http.Error(w, "peer message participants must be members of the group", 403)
				return
			}
			message, err := hub.PostPeerMessage(agentrun.PeerMessage{GroupID: request.GroupID, FromConversation: request.ConversationID, ToConversation: request.ToConversationID, Kind: messageKind, Body: request.Body, CorrelationID: request.CorrelationID})
			if err != nil {
				http.Error(w, "peer message rejected", 400)
				return
			}
			writeJSON(w, 200, map[string]any{"message": message})
		case "contract_publish":
			if request.Contract == nil {
				http.Error(w, "contract payload is required", 400)
				return
			}
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "contract owner conversation not found", 404)
				return
			}
			if request.GroupID != "" {
				if s.validateAgentConversationGroupMember(owner, request.GroupID, request.ConversationID) != nil ||
					request.Contract.OwnerGroup != "" && request.Contract.OwnerGroup != request.GroupID {
					http.Error(w, "contract publisher is not authorized for the group", 403)
					return
				}
				request.Contract.OwnerGroup = request.GroupID
			}
			contract, err := hub.UpsertContract(*request.Contract)
			if err != nil {
				http.Error(w, "contract rejected", 400)
				return
			}
			writeJSON(w, 200, map[string]any{"contract": contract})
		case "contract_advance":
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "contract owner conversation not found", 404)
				return
			}
			if request.GroupID != "" {
				if s.validateAgentConversationGroupMember(owner, request.GroupID, request.ConversationID) != nil {
					http.Error(w, "contract publisher is not authorized for the group", 403)
					return
				}
				contracts, listErr := hub.Contracts(request.GroupID)
				found := false
				for _, contract := range contracts {
					found = found || contract.Key == request.ContractKey
				}
				if listErr != nil || !found {
					http.Error(w, "contract is not available in the group", 404)
					return
				}
			}
			contract, err := hub.AdvanceContract(request.ContractKey, request.Revision, agentrun.ContractStatus(request.Status), request.IntegratedRef, request.TestEvidence)
			if err != nil {
				http.Error(w, "contract transition rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"contract": contract})
		case "artifact_publish":
			if request.Artifact == nil {
				http.Error(w, "artifact payload is required", 400)
				return
			}
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "artifact producer conversation not found", 404)
				return
			}
			if request.GroupID != "" && s.validateAgentConversationGroupMember(owner, request.GroupID, request.ConversationID) != nil {
				http.Error(w, "artifact producer is not a member of the group", 403)
				return
			}
			if request.Artifact.Conversation != "" && request.Artifact.Conversation != request.ConversationID {
				http.Error(w, "artifact conversation does not match its producer", 403)
				return
			}
			request.Artifact.Conversation = request.ConversationID
			artifact, err := hub.PublishArtifact(*request.Artifact)
			if err != nil {
				http.Error(w, "artifact rejected", 400)
				return
			}
			writeJSON(w, 200, map[string]any{"artifact": artifact})
		case "dependency_register":
			if request.Dependency == nil {
				http.Error(w, "dependency payload is required", 400)
				return
			}
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "dependency owner conversation not found", 404)
				return
			}
			if request.GroupID != "" {
				if s.validateAgentConversationGroupMember(owner, request.GroupID, request.ConversationID) != nil {
					http.Error(w, "dependency publisher is not a member of the group", 403)
					return
				}
				contracts, listErr := hub.Contracts(request.GroupID)
				found := false
				for _, contract := range contracts {
					found = found || contract.Key == request.Dependency.ContractKey
				}
				if listErr != nil || !found {
					http.Error(w, "dependency contract is not available in the group", 404)
					return
				}
			}
			dependency, err := hub.RegisterDependency(*request.Dependency)
			if err != nil {
				http.Error(w, "dependency rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"dependency": dependency})
		default:
			http.Error(w, "unknown coordination action", 400)
		}
	default:
		http.Error(w, "method not allowed", 405)
	}
}
