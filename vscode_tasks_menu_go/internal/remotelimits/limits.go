// Package remotelimits is the single source of truth for the maximum
// number of independently authenticated, bounded, read-only AEAD application
// exchanges permitted within one short-lived TaskDeck E2EE session.
//
// Raising this must preserve: a 5-minute session, one in-flight exchange,
// a 512-byte plaintext AEAD limit, host-local approved client identity,
// fresh per-action TaskDeck password/RBAC and live Portal grant checks.
// It does NOT enable Git mutation, task execution, shell, PTY or file APIs.
package remotelimits

const MaxReadOnlyAppRounds = 16
