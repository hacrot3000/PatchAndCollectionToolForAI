package remoterelay

import (
    "bytes"
    "encoding/json"
    "errors"
    "io"
    "strconv"
    "unicode/utf8"
)

// The Agent socket is authenticated, but neither Relay nor future endpoints
// may interpret ambiguous JSON differently from one another. Enforce exact,
// case-sensitive, unique keys and reject unknown fields before lease checks.
// JSON escape aliases resolve to the same decoded key and are duplicates.
var errAgentFrame = errors.New("invalid unambiguous Agent relay frame")

func relayExactObject(raw []byte, maxSize int, fields ...string) (map[string]json.RawMessage, error) {
    if len(raw) < 2 || len(raw) > maxSize || !utf8.Valid(raw) {
        return nil, errAgentFrame
    }
    allowed := make(map[string]bool, len(fields))
    for _, field := range fields { allowed[field] = true }
    values := make(map[string]json.RawMessage, len(fields))
    dec := json.NewDecoder(bytes.NewReader(raw))
    token, err := dec.Token()
    if err != nil || token != json.Delim('{') { return nil, errAgentFrame }
    for dec.More() {
        token, err = dec.Token()
        name, ok := token.(string)
        if err != nil || !ok || !allowed[name] { return nil, errAgentFrame }
        if _, duplicated := values[name]; duplicated { return nil, errAgentFrame }
        var value json.RawMessage
        if err = dec.Decode(&value); err != nil { return nil, errAgentFrame }
        values[name] = value
    }
    token, err = dec.Token()
    if err != nil || token != json.Delim('}') { return nil, errAgentFrame }
    if _, err = dec.Token(); !errors.Is(err, io.EOF) { return nil, errAgentFrame }
    return values, nil
}

func relayExactKeys(values map[string]json.RawMessage, fields ...string) bool {
    if len(values) != len(fields) { return false }
    for _, field := range fields {
        if _, exists := values[field]; !exists { return false }
    }
    return true
}

func relayString(raw json.RawMessage, max int) (string, error) {
    if len(raw) < 3 || len(raw) > max+2 { return "", errAgentFrame }
    var result string
    if json.Unmarshal(raw, &result) != nil || result == "" || len(result) > max {
        return "", errAgentFrame
    }
    return result, nil
}

func decodeDeviceConnectProof(raw []byte) (DeviceProof, error) {
    var p DeviceProof
    fields := []string{"device_id", "public_key", "aud", "challenge", "device_lease",
        "timestamp", "nonce", "signature"}
    values, err := relayExactObject(raw, 4096, fields...)
    if err != nil || !relayExactKeys(values, fields...) { return p, errAgentFrame }
    assign := []struct{ key string; max int; target *string }{
        {"device_id", 64, &p.DeviceID}, {"public_key", 64, &p.PublicKey},
        {"aud", 512, &p.Audience}, {"challenge", 64, &p.Challenge},
        {"device_lease", 2048, &p.DeviceLease}, {"nonce", 32, &p.Nonce},
        {"signature", 128, &p.Signature},
    }
    for _, field := range assign {
        *field.target, err = relayString(values[field.key], field.max)
        if err != nil { return DeviceProof{}, errAgentFrame }
    }
    // A strict decimal JSON integer (not a quoted string, exponent or float).
    p.Timestamp, err = strconv.ParseInt(string(values["timestamp"]), 10, 64)
    if err != nil || p.Timestamp <= 0 { return DeviceProof{}, errAgentFrame }
    return p, nil
}

type agentRelayFrame struct {
    Type string
    ProbeID string
    ClientNonce string
    PublicKey string
    Signature string
    Crypto *ProbeCrypto
    Ciphertext string
    DeviceLease string
    RelayID string
}

func decodeAgentRelayFrame(raw []byte) (agentRelayFrame, error) {
    var f agentRelayFrame
    allowed := []string{"type", "probe_id", "client_nonce", "public_key",
        "signature", "crypto", "ciphertext", "device_lease", "relay_id"}
    values, err := relayExactObject(raw, 12288, allowed...)
    if err != nil { return f, errAgentFrame }
    f.Type, err = relayString(values["type"], 40)
    if err != nil { return agentRelayFrame{}, errAgentFrame }
    var required []string
    switch f.Type {
    case "ping":
        required = []string{"type", "device_lease"}
    case "probe_rejected", "login_rejected", "echo_rejected":
        required = []string{"type", "device_lease", "probe_id"}
    case "probe_ack":
        required = []string{"type", "device_lease", "probe_id",
            "client_nonce", "public_key", "signature", "crypto"}
    case "login_ack", "echo_ack":
        required = []string{"type", "device_lease", "probe_id", "ciphertext"}
    case "app_ready", "app_transfer_ready":
        // The signed Agent may only open a bounded opaque ping mailbox;
        // the Relay itself can never assert a TaskDeck login or RBAC role.
        required = []string{"type", "device_lease", "probe_id", "public_key", "signature"}
    case "app_ping_ack":
        required = []string{"type", "device_lease", "probe_id", "relay_id", "ciphertext"}
    case "app_ping_close":
        required = []string{"type", "device_lease", "probe_id"}
    default:
        return agentRelayFrame{}, errAgentFrame
    }
    if !relayExactKeys(values, required...) { return agentRelayFrame{}, errAgentFrame }
    targets := []struct{ key string; max int; target *string }{
        {"device_lease", 2048, &f.DeviceLease},
        {"probe_id", 32, &f.ProbeID},
        {"client_nonce", 32, &f.ClientNonce},
        {"public_key", 64, &f.PublicKey},
        {"signature", 128, &f.Signature},
        {"ciphertext", MaxEncodedAppCiphertext, &f.Ciphertext},
        {"relay_id", 32, &f.RelayID},
    }
    for _, field := range targets {
        if value, exists := values[field.key]; exists {
            *field.target, err = relayString(value, field.max)
            if err != nil { return agentRelayFrame{}, errAgentFrame }
        }
    }
    // Only a previously authorized app_ping_ack may use the large envelope.
    // Never silently expand encrypted login or key-check frame limits.
    if f.Type!="app_ping_ack" && len(f.Ciphertext)>2048 {
        return agentRelayFrame{},errAgentFrame
    }
    if f.Type == "probe_ack" {
        fields := []string{"identity", "ephemeral", "signature", "encrypted_ready"}
        nested, nestedErr := relayExactObject(values["crypto"], 2048, fields...)
        if nestedErr != nil || !relayExactKeys(nested, fields...) {
            return agentRelayFrame{}, errAgentFrame
        }
        f.Crypto = &ProbeCrypto{}
        parts := []struct{ key string; max int; target *string }{
            {"identity", 64, &f.Crypto.Identity}, {"ephemeral", 128, &f.Crypto.Ephemeral},
            {"signature", 128, &f.Crypto.Signature},
            {"encrypted_ready", 512, &f.Crypto.EncryptedReady},
        }
        for _, field := range parts {
            *field.target, err = relayString(nested[field.key], field.max)
            if err != nil { return agentRelayFrame{}, errAgentFrame }
        }
    }
    return f, nil
}
