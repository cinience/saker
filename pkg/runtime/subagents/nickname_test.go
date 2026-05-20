package subagents

import (
	"strings"
	"testing"
)

func TestGenerateNicknameUnique(t *testing.T) {
	t.Parallel()
	nick := GenerateNickname(nil, "fallback")
	if nick == "" {
		t.Fatal("expected non-empty nickname")
	}
	found := false
	for _, n := range defaultNicknames {
		if n == nick {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("nickname %q not in default list", nick)
	}
}

func TestGenerateNicknameAvoidsExisting(t *testing.T) {
	t.Parallel()
	// Use all but one nickname.
	existing := make([]string, len(defaultNicknames)-1)
	copy(existing, defaultNicknames[:len(defaultNicknames)-1])
	nick := GenerateNickname(existing, "fallback-id")
	// Should pick the remaining one.
	if nick != defaultNicknames[len(defaultNicknames)-1] {
		t.Errorf("expected %q, got %q", defaultNicknames[len(defaultNicknames)-1], nick)
	}
}

func TestGenerateNicknameFallback(t *testing.T) {
	t.Parallel()
	// Use all nicknames.
	nick := GenerateNickname(defaultNicknames, "my-fallback")
	if nick != "my-fallback" {
		t.Errorf("expected fallback, got %q", nick)
	}
}

func TestResolveAgentIDByID(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	_ = store.Create(Instance{ID: "subagent-1", Metadata: map[string]any{"nickname": "scout"}})

	id, err := ResolveAgentID(store, "subagent-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "subagent-1" {
		t.Errorf("got %q, want subagent-1", id)
	}
}

func TestResolveAgentIDByNickname(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	_ = store.Create(Instance{ID: "subagent-1", Metadata: map[string]any{"nickname": "scout"}})

	id, err := ResolveAgentID(store, "scout")
	if err != nil {
		t.Fatal(err)
	}
	if id != "subagent-1" {
		t.Errorf("got %q, want subagent-1", id)
	}
}

func TestResolveAgentIDCaseInsensitive(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	_ = store.Create(Instance{ID: "subagent-1", Metadata: map[string]any{"nickname": "Scout"}})

	id, err := ResolveAgentID(store, "SCOUT")
	if err != nil {
		t.Fatal(err)
	}
	if id != "subagent-1" {
		t.Errorf("got %q, want subagent-1", id)
	}
}

func TestResolveAgentIDNotFound(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	_, err := ResolveAgentID(store, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown instance") {
		t.Errorf("unexpected error: %v", err)
	}
}
