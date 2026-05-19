package agui

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/conversation"
)

// handleThreadUpdate implements PATCH /v1/agents/run/threads/:threadId —
// renames a thread. CopilotKit v2 sends {"name":"...","agentId":"..."}.
func (g *Gateway) handleThreadUpdate(c *gin.Context) {
	threadID := c.Param("threadId")
	if threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "threadId required"})
		return
	}
	cs := g.deps.ConversationStore
	if cs == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "no conversation store"})
		return
	}

	var body struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		AgentID string `json:"agentId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	name := body.Name
	if name == "" {
		name = body.Title
	}

	if err := cs.UpdateThreadTitle(c.Request.Context(), threadID, name); err != nil {
		if errors.Is(err, conversation.ErrThreadNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "thread not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	t, err := cs.GetThread(c.Request.Context(), threadID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"id": threadID, "name": name})
		return
	}
	c.JSON(http.StatusOK, formatThreadResponse(t))
}

// handleThreadArchive implements POST /v1/agents/run/threads/:threadId/archive —
// archives a thread. CopilotKit v2 sends {"agentId":"..."}.
// Treated as a soft-delete since the conversation store has no separate archive flag.
func (g *Gateway) handleThreadArchive(c *gin.Context) {
	threadID := c.Param("threadId")
	if threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "threadId required"})
		return
	}
	cs := g.deps.ConversationStore
	if cs == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "no conversation store"})
		return
	}

	if err := cs.SoftDeleteThread(c.Request.Context(), threadID); err != nil {
		if errors.Is(err, conversation.ErrThreadNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "thread not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "archive failed"})
		return
	}
	c.Status(http.StatusOK)
}

// handleThreadDelete implements DELETE /v1/agents/run/threads/:threadId —
// soft-deletes a thread.
func (g *Gateway) handleThreadDelete(c *gin.Context) {
	threadID := c.Param("threadId")
	if threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "threadId required"})
		return
	}
	cs := g.deps.ConversationStore
	if cs == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "no conversation store"})
		return
	}

	if err := cs.SoftDeleteThread(c.Request.Context(), threadID); err != nil {
		if errors.Is(err, conversation.ErrThreadNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "thread not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": threadID, "status": "deleted"})
}
