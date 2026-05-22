package client

import "testing"

func TestDetectAgents_ReturnsSlice(t *testing.T) {
	agents := DetectAgents()
	// In CI/test environments, agents may or may not be installed.
	// Just verify it returns a non-nil-typed slice without panicking.
	if agents == nil {
		// nil is acceptable (no agents found), but verify the type.
		var _ []DetectedAgent = agents
	}
	for _, a := range agents {
		if a.Name == "" {
			t.Errorf("detected agent with empty Name at path %q", a.Path)
		}
		if a.Path == "" {
			t.Errorf("detected agent %q with empty Path", a.Name)
		}
	}
}

func TestKnownAgents_CatalogEntries(t *testing.T) {
	if len(knownAgents) == 0 {
		t.Fatal("knownAgents catalog is empty")
	}
	for i, ka := range knownAgents {
		if ka.Name == "" {
			t.Errorf("knownAgents[%d] has empty Name", i)
		}
		if ka.Binary == "" {
			t.Errorf("knownAgents[%d] (%s) has empty Binary", i, ka.Name)
		}
		if ka.Desc == "" {
			t.Errorf("knownAgents[%d] (%s) has empty Desc", i, ka.Name)
		}
	}
}
