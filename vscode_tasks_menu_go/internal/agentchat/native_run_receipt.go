package agentchat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// nativeRunReceiptVersion is the schema version of the canonical native
// tool-call receipt envelope. It is server-owned: provider text, model claims
// and browser-supplied values cannot create or refresh one.
const nativeRunReceiptVersion = 1

// nativeRunReceiptPayload is the immutable, versioned identity of one
// completed native operation for a run-managed conversation. It binds the
// owner/project/conversation/run/attempt/tool call together with canonical
// request and result digests and the pinned workspace snapshot. It never
// carries credentials or raw provider payloads.
type nativeRunReceiptPayload struct {
	Version               int    `json:"version"`
	OwnerWorkspaceID      string `json:"owner_workspace_id"`
	ProjectID             string `json:"project_id"`
	ConversationID        string `json:"conversation_id"`
	RunID                 string `json:"run_id"`
	AttemptID             string `json:"attempt_id"`
	CallID                string `json:"call_id"`
	AgentRunID            string `json:"agent_run_id,omitempty"`
	ProviderID            string `json:"provider_id"`
	ModelID               string `json:"model_id"`
	ProviderConfigVersion string `json:"provider_config_version"`
	RequestDigest         string `json:"request_digest"`
	NativeSessionID       string `json:"native_session_id,omitempty"`
	QueuePath             string `json:"queue_path,omitempty"`
	PackageSHA256         string `json:"package_sha256,omitempty"`
	Status                string `json:"status"`
	ResultDigest          string `json:"result_digest"`
	ExpectedHead          string `json:"expected_head,omitempty"`
	ExpectedBranch        string `json:"expected_branch,omitempty"`
	WorkspaceStatusSHA256 string `json:"workspace_status_sha256,omitempty"`
}

// nativeRunReceiptEnvelope pairs the canonical payload with a digest of its
// exact JSON. No client-supplied signature is invented: integrity comes from
// the private store that holds the envelope, as with native Task receipts.
type nativeRunReceiptEnvelope struct {
	Payload nativeRunReceiptPayload `json:"payload"`
	Digest  string                 `json:"digest"`
}

func nativeRunReceiptSHA256(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func nativeRunReceiptPayloadDigest(p nativeRunReceiptPayload) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// newNativeRunReceipt builds a receipt only from server-owned inputs. It
// refuses incomplete bindings, a call from another run, and any native result
// that is not a final acknowledged PASS. It does not persist anything.
func newNativeRunReceipt(spec RunSpec, call ToolCallRecord, projectID string) (nativeRunReceiptEnvelope, error) {
	if err := spec.Validate(); err != nil {
		return nativeRunReceiptEnvelope{}, err
	}
	if projectID == "" || call.CallID == "" {
		return nativeRunReceiptEnvelope{}, errors.New("native receipt binding fields are incomplete")
	}
	if call.AgentRunID != "" && call.AgentRunID != spec.RunID {
		return nativeRunReceiptEnvelope{}, errors.New("native receipt call belongs to another run")
	}
	if err := validateTaskNativePatchSuccess(call); err != nil {
		return nativeRunReceiptEnvelope{}, err
	}
	payload := nativeRunReceiptPayload{
		Version:               nativeRunReceiptVersion,
		OwnerWorkspaceID:      spec.OwnerWorkspaceID,
		ProjectID:             projectID,
		ConversationID:        spec.ConversationID,
		RunID:                 spec.RunID,
		AttemptID:             spec.AttemptID,
		CallID:                call.CallID,
		AgentRunID:            call.AgentRunID,
		ProviderID:            spec.ProviderID,
		ModelID:               spec.ModelID,
		ProviderConfigVersion: spec.ProviderConfigVersion,
		RequestDigest: nativeRunReceiptSHA256(call.ArgumentsSHA256, spec.RunID,
			spec.AttemptID, spec.ProviderID, spec.ModelID, spec.ProviderConfigVersion),
		NativeSessionID:       call.PatchSessionID,
		QueuePath:             call.QueuePath,
		PackageSHA256:         call.PackageSHA256,
		Status:                call.Status,
		ResultDigest:          nativeRunReceiptSHA256(call.Result),
		ExpectedHead:          call.ExpectedHead,
		ExpectedBranch:        call.ExpectedBranch,
		WorkspaceStatusSHA256: call.WorkspaceStatusSHA256,
	}
	digest, err := nativeRunReceiptPayloadDigest(payload)
	if err != nil {
		return nativeRunReceiptEnvelope{}, err
	}
	return nativeRunReceiptEnvelope{Payload: payload, Digest: digest}, nil
}

// verifyNativeRunReceipt rechecks the envelope against the run binding and
// its own canonical digest. A tampered payload, changed spec or malformed
// digest fails closed without contacting a provider or reading workspace text.
func verifyNativeRunReceipt(env nativeRunReceiptEnvelope, spec RunSpec, projectID string) error {
	p := env.Payload
	if p.Version != nativeRunReceiptVersion {
		return errors.New("unsupported native receipt version")
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	if p.OwnerWorkspaceID != spec.OwnerWorkspaceID || p.ConversationID != spec.ConversationID ||
		p.RunID != spec.RunID || p.AttemptID != spec.AttemptID ||
		p.ProviderID != spec.ProviderID || p.ModelID != spec.ModelID ||
		p.ProviderConfigVersion != spec.ProviderConfigVersion || p.ProjectID != projectID {
		return errors.New("native receipt run binding mismatch")
	}
	if p.AgentRunID != "" && p.AgentRunID != spec.RunID {
		return errors.New("native receipt call binding mismatch")
	}
	if p.CallID == "" || p.Status != "done" ||
		len(p.RequestDigest) != 64 || len(p.ResultDigest) != 64 || len(env.Digest) != 64 {
		return errors.New("native receipt is incomplete")
	}
	if strings.Trim(p.RequestDigest, "0123456789abcdef") != "" ||
		strings.Trim(p.ResultDigest, "0123456789abcdef") != "" ||
		strings.Trim(env.Digest, "0123456789abcdef") != "" {
		return errors.New("native receipt digest is malformed")
	}
	want, err := nativeRunReceiptPayloadDigest(p)
	if err != nil {
		return err
	}
	if want != env.Digest {
		return errors.New("native receipt integrity mismatch")
	}
	return nil
}
