package dbadapter

import (
	"fmt"
	"strings"
)

type CapabilitySet struct {
	Connect        bool `json:"connect"`
	Ping           bool `json:"ping"`
	ListCatalogs   bool `json:"list_catalogs"`
	ListObjects    bool `json:"list_objects"`
	DescribeObject bool `json:"describe_object"`
	BrowseRows     bool `json:"browse_rows"`
	MutateRows     bool `json:"mutate_rows"`
	ObjectActions  bool `json:"object_actions"`
	Execute        bool `json:"execute"`
	Cancel         bool `json:"cancel"`
	Transactions   bool `json:"transactions"`
}

type Manifest struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Kind            string        `json:"kind"`
	ProtocolVersion int           `json:"protocol_version"`
	Command         string        `json:"-"`
	Args            []string      `json:"-"`
	Capabilities    CapabilitySet `json:"capabilities"`
}

func (m Manifest) Validate() error {
	if err := validateToken("adapter id", strings.TrimSpace(m.ID), 128, true); err != nil {
		return err
	}
	if err := validateText("adapter name", strings.TrimSpace(m.Name), 128, true); err != nil {
		return err
	}
	if err := validateToken("adapter kind", strings.TrimSpace(m.Kind), 64, true); err != nil {
		return err
	}
	if m.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("adapter %q protocol version %d is unsupported", m.ID, m.ProtocolVersion)
	}
	if strings.TrimSpace(m.Command) == "" {
		return fmt.Errorf("adapter %q command is required", m.ID)
	}
	if len(m.Args) > 64 {
		return fmt.Errorf("adapter %q exceeds 64 process arguments", m.ID)
	}
	for i, arg := range m.Args {
		if err := validateText("adapter argument", arg, 4096, false); err != nil {
			return fmt.Errorf("adapter %q argument %d: %w", m.ID, i+1, err)
		}
	}
	if !m.Capabilities.Connect {
		return fmt.Errorf("adapter %q must support connect", m.ID)
	}
	if !m.Capabilities.Ping {
		return fmt.Errorf("adapter %q must support ping", m.ID)
	}
	return nil
}

func (c CapabilitySet) Supports(operation Operation) bool {
	switch operation {
	case OpHello, OpCapabilities, OpDisconnect:
		return true
	case OpConnect:
		return c.Connect
	case OpPing:
		return c.Ping
	case OpListCatalogs:
		return c.ListCatalogs
	case OpListObjects:
		return c.ListObjects
	case OpDescribeObject:
		return c.DescribeObject
	case OpBrowseRows:
		return c.BrowseRows
	case OpMutateRows:
		return c.MutateRows
	case OpObjectAction:
		return c.ObjectActions
	case OpExecute:
		return c.Execute
	case OpCancel:
		return c.Cancel
	case OpBegin, OpCommit, OpRollback:
		return c.Transactions
	default:
		return false
	}
}
