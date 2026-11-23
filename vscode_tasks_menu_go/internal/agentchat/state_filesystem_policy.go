package agentchat

import (
 "errors"
 "fmt"
)

// ErrUnsupportedAgentStateFilesystem is an admission failure. TaskDeck's
// local process + flock locking and rename/directory-fsync protocol cannot
// safely promise CAS or crash durability on network/distributed mounts.
// Operators must move the PRIVATE Agent state directory to a supported
// locally mounted filesystem; retrying the provider or native tool does not
// repair this condition.
var ErrUnsupportedAgentStateFilesystem=errors.New("unsupported agent state filesystem")

func unsupportedAgentStateFilesystem(kind string)error{
 return fmt.Errorf("%w: %s; use a private locally mounted filesystem, not a shared network mount",
  ErrUnsupportedAgentStateFilesystem,kind)
}
