package agui

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func newAGUIRateLimiter(rps float64, burst int, logger *slog.Logger) (gin.HandlerFunc, func()) {
	type visitor struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu       sync.Mutex
		visitors = make(map[string]*visitor)
		stopCh   = make(chan struct{})
	)

	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				mu.Lock()
				for key, v := range visitors {
					if time.Since(v.lastSeen) > 10*time.Minute {
						delete(visitors, key)
					}
				}
				mu.Unlock()
			}
		}
	}()

	cleanup := func() { close(stopCh) }

	middleware := func(c *gin.Context) {
		identity := identityFromContext(c.Request.Context())
		key := identity.APIKeyID
		if key == "" {
			key = identity.Username
		}
		if key == "" {
			key = c.ClientIP()
		}

		mu.Lock()
		v, exists := visitors[key]
		if !exists {
			v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
			visitors[key] = v
		}
		v.lastSeen = time.Now()
		mu.Unlock()

		if !v.limiter.Allow() {
			logger.Warn("agui rate limit exceeded",
				"key", key,
				"client_ip", c.ClientIP(),
			)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": gin.H{
				"message": "rate limit exceeded",
				"type":    "rate_limit_error",
			}})
			return
		}
		c.Next()
	}

	return middleware, cleanup
}
