package server

import "testing"

func TestAgentRunManagerBuildsOwnerBoundProviderRouterByDefault(t *testing.T) {
	s, _, _ := prepareRunAPIServer(t, false)
	s.AgentRunExecutor = nil
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()
}
