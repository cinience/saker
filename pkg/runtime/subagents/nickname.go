package subagents

import (
	"math/rand"
	"strings"
)

var defaultNicknames = []string{
	"atlas", "scout", "beacon", "forge", "pulse",
	"nova", "arrow", "orbit", "pixel", "cipher",
	"spark", "drift", "prism", "vortex", "flint",
	"onyx", "ridge", "quill", "lumen", "nexus",
	"ember", "frost", "helix", "zenith", "delta",
	"coral", "swift", "raven", "blaze", "crest",
}

// GenerateNickname picks a random nickname not already in use.
// Falls back to the instance ID suffix if all are taken.
func GenerateNickname(existing []string, fallback string) string {
	used := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		used[strings.ToLower(n)] = struct{}{}
	}
	available := make([]string, 0, len(defaultNicknames))
	for _, name := range defaultNicknames {
		if _, taken := used[name]; !taken {
			available = append(available, name)
		}
	}
	if len(available) == 0 {
		if fallback != "" {
			return fallback
		}
		return defaultNicknames[rand.Intn(len(defaultNicknames))]
	}
	return available[rand.Intn(len(available))]
}

// ResolveAgentID looks up an agent by ID or nickname in the store.
// Returns the canonical ID or an error if not found.
func ResolveAgentID(store Store, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrUnknownInstance
	}
	// Try direct ID match first.
	if _, ok := store.Get(input); ok {
		return input, nil
	}
	// Try nickname match.
	if ms, ok := store.(*MemoryStore); ok {
		ms.mu.RLock()
		defer ms.mu.RUnlock()
		lower := strings.ToLower(input)
		for _, inst := range ms.items {
			if nick, ok := inst.Metadata["nickname"].(string); ok {
				if strings.ToLower(nick) == lower {
					return inst.ID, nil
				}
			}
		}
	}
	return "", ErrUnknownInstance
}
