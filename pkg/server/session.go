package server

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/saker-ai/saker/pkg/conversation"
	"github.com/saker-ai/saker/pkg/message"
	"github.com/google/uuid"
)


// SessionStore manages thread state as an in-memory cache backed by
// conversation.Store. Startup state is populated via LoadFromConversation;
// persistence is owned by the Runtime layer.
type SessionStore struct {
	mu        sync.RWMutex
	threads   []Thread
	threadIdx map[string]int         // threadID → index in threads slice
	items     map[string][]ThreadItem // threadID → items
}

// NewSessionStore creates an empty in-memory store. Call LoadFromConversation
// after construction to populate startup state from conversation.Store.
func NewSessionStore() (*SessionStore, error) {
	return &SessionStore{
		threads:   make([]Thread, 0),
		threadIdx: make(map[string]int),
		items:     make(map[string][]ThreadItem),
	}, nil
}

// LoadFromConversation populates in-memory threads and items from the
// conversation.Store. No-op when store is nil. Called once after construction.
func (s *SessionStore) LoadFromConversation(store *conversation.Store, projectID string) error {
	if s == nil || store == nil {
		return nil
	}
	ctx := context.Background()
	threads, err := store.ListThreads(ctx, projectID, conversation.ListThreadsOpts{
		Limit: conversation.MaxListLimit,
	})
	if err != nil {
		return fmt.Errorf("load threads: %w", err)
	}
	threadIDs := make([]string, len(threads))
	for i := range threads {
		threadIDs[i] = threads[i].ID
	}
	allMsgs, err := store.GetMessagesByThreadIDs(ctx, threadIDs, conversation.GetMessagesOpts{
		Limit: conversation.MaxListLimit,
	})
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ct := range threads {
		t := Thread{
			ID:        ct.ID,
			Title:     ct.Title,
			CreatedAt: ct.CreatedAt,
			UpdatedAt: ct.UpdatedAt,
		}
		s.threadIdx[t.ID] = len(s.threads)
		s.threads = append(s.threads, t)
		msgs := allMsgs[ct.ID]
		items := make([]ThreadItem, 0, len(msgs))
		for _, m := range msgs {
			items = append(items, ThreadItem{
				ID:        strconv.FormatInt(m.ID, 10),
				ThreadID:  m.ThreadID,
				TurnID:    m.TurnID,
				Role:      m.Role,
				Content:   m.Content,
				CreatedAt: m.CreatedAt,
			})
		}
		s.items[ct.ID] = items
	}
	return nil
}

// CreateThread creates a new conversation thread.
func (s *SessionStore) CreateThread(title string) Thread {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	t := Thread{
		ID:        uuid.New().String(),
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.threadIdx[t.ID] = len(s.threads)
	s.threads = append(s.threads, t)
	s.items[t.ID] = make([]ThreadItem, 0)
	return t
}

// ListThreads returns all threads ordered by creation time.
func (s *SessionStore) ListThreads() []Thread {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Thread, len(s.threads))
	copy(out, s.threads)
	return out
}

// UpdateThreadTitle updates the title of an existing thread.
func (s *SessionStore) UpdateThreadTitle(threadID, title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.threadIdx[threadID]
	if !ok {
		return false
	}
	s.threads[idx].Title = title
	s.threads[idx].UpdatedAt = time.Now()
	return true
}

// DeleteThread removes a thread and its items from the in-memory cache.
func (s *SessionStore) DeleteThread(threadID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.threadIdx[threadID]
	if !ok {
		return false
	}
	last := len(s.threads) - 1
	if idx != last {
		s.threads[idx] = s.threads[last]
		s.threadIdx[s.threads[idx].ID] = idx
	}
	s.threads = s.threads[:last]
	delete(s.threadIdx, threadID)
	delete(s.items, threadID)
	return true
}

// GetThread returns a single thread by ID.
func (s *SessionStore) GetThread(threadID string) (Thread, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.threadIdx[threadID]; ok {
		return s.threads[idx], true
	}
	return Thread{}, false
}

// AppendItem adds a message to a thread and returns the created item.
func (s *SessionStore) AppendItem(threadID, role, content, turnID string) ThreadItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := ThreadItem{
		ID:        uuid.New().String(),
		ThreadID:  threadID,
		TurnID:    turnID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}
	s.items[threadID] = append(s.items[threadID], item)
	if idx, ok := s.threadIdx[threadID]; ok {
		s.threads[idx].UpdatedAt = item.CreatedAt
	}
	return item
}

// AppendItemWithArtifacts adds a message with media artifacts to a thread.
func (s *SessionStore) AppendItemWithArtifacts(threadID, role, content, turnID string, artifacts []Artifact) ThreadItem {
	return s.appendItemFull(threadID, role, "", content, turnID, artifacts)
}

// AppendToolItem adds a tool result item with an explicit tool name.
func (s *SessionStore) AppendToolItem(threadID, toolName, content, turnID string, artifacts []Artifact) ThreadItem {
	return s.appendItemFull(threadID, "tool", toolName, content, turnID, artifacts)
}

func (s *SessionStore) appendItemFull(threadID, role, toolName, content, turnID string, artifacts []Artifact) ThreadItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := ThreadItem{
		ID:        uuid.New().String(),
		ThreadID:  threadID,
		TurnID:    turnID,
		Role:      role,
		ToolName:  toolName,
		Content:   content,
		Artifacts: artifacts,
		CreatedAt: time.Now(),
	}
	s.items[threadID] = append(s.items[threadID], item)
	if idx, ok := s.threadIdx[threadID]; ok {
		s.threads[idx].UpdatedAt = item.CreatedAt
	}
	return item
}

// GetItem returns a single item by ID across all threads.
func (s *SessionStore) GetItem(itemID string) (ThreadItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, items := range s.items {
		for _, item := range items {
			if item.ID == itemID {
				return item, true
			}
		}
	}
	return ThreadItem{}, false
}

// UpdateItemArtifact replaces an artifact URL within an item. Returns true if updated.
func (s *SessionStore) UpdateItemArtifact(itemID, oldURL, newURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tid, items := range s.items {
		for i, item := range items {
			if item.ID != itemID {
				continue
			}
			for j, a := range item.Artifacts {
				if a.URL == oldURL {
					s.items[tid][i].Artifacts[j].URL = newURL
					return true
				}
			}
			return false
		}
	}
	return false
}

// IngestFromRuntime receives messages persisted by the Runtime and appends
// them to the in-memory cache. De-duplicates by turnID+role+content so items
// already written by handler_turn (web UI path) are not doubled.
// If the thread is not in the cache, the messages are silently dropped (the
// thread will be loaded from DB on next access).
func (s *SessionStore) IngestFromRuntime(threadID string, msgs []message.Message) {
	if s == nil || len(msgs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.threadIdx[threadID]; !ok {
		return
	}
	existing := s.items[threadID]
	for _, m := range msgs {
		if isDuplicate(existing, m) {
			continue
		}
		item := ThreadItem{
			ID:        uuid.New().String(),
			ThreadID:  threadID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: time.Now(),
		}
		existing = append(existing, item)
	}
	s.items[threadID] = existing
	if idx, ok := s.threadIdx[threadID]; ok {
		s.threads[idx].UpdatedAt = time.Now()
	}
}

func isDuplicate(items []ThreadItem, m message.Message) bool {
	for i := len(items) - 1; i >= 0 && i >= len(items)-5; i-- {
		if items[i].Role == m.Role && items[i].Content == m.Content {
			return true
		}
	}
	return false
}

// GetItems returns all items for a thread.
func (s *SessionStore) GetItems(threadID string) []ThreadItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.items[threadID]
	out := make([]ThreadItem, len(items))
	copy(out, items)
	return out
}

