package web

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// These budgets are shared by the SDK proxy and server-side location lookups.
// Bursts accommodate parallel map tiles while keeping sustained owner-key use
// bounded. Each process has its own budgets; replicas need a shared edge quota.
const (
	mapUserBurst        = 60
	mapIPBurst          = 120
	mapUserRate         = 2.0  // 120 requests/minute after the initial burst.
	mapIPRate           = 10.0 // 600 requests/minute shared by users on the same IP.
	mapUserConcurrent   = 8
	mapIPConcurrent     = 16
	mapGlobalConcurrent = 32
	mapMaxIdentities    = 10000
	mapIdleExpiry       = 5 * time.Minute
)

type mapBudget struct {
	tokens  float64
	updated time.Time
	active  int
}

type mapRequestLimiter struct {
	mu        sync.Mutex
	users     map[int64]*mapBudget
	ips       map[string]*mapBudget
	active    int
	lastSweep time.Time
}

func refillMapBudget(b *mapBudget, now time.Time, rate, burst float64) {
	b.tokens += now.Sub(b.updated).Seconds() * rate
	if b.tokens > burst {
		b.tokens = burst
	}
	b.updated = now
}

func (l *mapRequestLimiter) acquire(user int64, ip string, now time.Time) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.users == nil {
		l.users = make(map[int64]*mapBudget)
		l.ips = make(map[string]*mapBudget)
	}
	if now.Sub(l.lastSweep) >= time.Minute {
		for key, b := range l.users {
			if b.active == 0 && now.Sub(b.updated) > mapIdleExpiry {
				delete(l.users, key)
			}
		}
		for key, b := range l.ips {
			if b.active == 0 && now.Sub(b.updated) > mapIdleExpiry {
				delete(l.ips, key)
			}
		}
		l.lastSweep = now
	}
	u, p := l.users[user], l.ips[ip]
	// Fail closed rather than allow identity churn to grow memory without bound.
	if u == nil && len(l.users) >= mapMaxIdentities || p == nil && len(l.ips) >= mapMaxIdentities {
		return nil, false
	}
	if u == nil {
		u = &mapBudget{tokens: mapUserBurst, updated: now}
		l.users[user] = u
	}
	if p == nil {
		p = &mapBudget{tokens: mapIPBurst, updated: now}
		l.ips[ip] = p
	}
	refillMapBudget(u, now, mapUserRate, mapUserBurst)
	refillMapBudget(p, now, mapIPRate, mapIPBurst)
	if u.tokens < 1 || p.tokens < 1 || u.active >= mapUserConcurrent || p.active >= mapIPConcurrent || l.active >= mapGlobalConcurrent {
		return nil, false
	}
	u.tokens--
	p.tokens--
	u.active++
	p.active++
	l.active++
	var once sync.Once
	return func() { once.Do(func() { l.mu.Lock(); u.active--; p.active--; l.active--; l.mu.Unlock() }) }, true
}

func (h *locationHandler) acquireMapRequest(c *gin.Context) (func(), bool) {
	user := currentPlatformUser(c)
	if user.ID <= 0 {
		c.AbortWithStatusJSON(401, gin.H{"error": "Authentication required", "code": "AUTH_REQUIRED"})
		return nil, false
	}
	h.limitsOnce.Do(func() { h.limits = &mapRequestLimiter{} })
	release, ok := h.limits.acquire(user.ID, requestClientIP(c), time.Now())
	if !ok {
		c.Header("Retry-After", "1")
		c.AbortWithStatusJSON(429, gin.H{"error": "Map request limit reached; retry shortly"})
		return nil, false
	}
	return release, true
}

func (h *locationHandler) withMapLimits(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := h.acquireMapRequest(c)
		if !ok {
			return
		}
		defer release()
		next(c)
	}
}
