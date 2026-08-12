package middleware

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"fund-management-api/models"
	"fund-management-api/services"

	"github.com/gin-gonic/gin"
)

// Gin context keys set by APIKeyAuthMiddleware and consumed by RequireScope / ExtRequestLog.
const (
	ctxAPIClientID = "apiClientID" // uint64
	ctxAPIKeyID    = "apiKeyID"    // uint64
	ctxAPIScopes   = "apiScopes"   // []string
)

// APIKeyAuthMiddleware authenticates external clients via `Authorization: Bearer <key>`.
// On success it stores the client id, key id, and granted scopes in the gin context.
// Every failure returns 401 with the standard error envelope and does not distinguish
// between an unknown, revoked, or expired key.
func APIKeyAuthMiddleware() gin.HandlerFunc {
	svc := services.NewAPIClientService(nil)
	return func(c *gin.Context) {
		raw := extractBearerToken(c)
		if raw == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Missing API key. Send it as 'Authorization: Bearer <key>'.",
				"code":    "MISSING_API_KEY",
			})
			c.Abort()
			return
		}

		verified, err := svc.VerifyAPIKey(raw)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired API key.",
				"code":    "INVALID_API_KEY",
			})
			c.Abort()
			return
		}

		c.Set(ctxAPIClientID, verified.Client.ID)
		c.Set(ctxAPIKeyID, verified.Key.ID)
		c.Set(ctxAPIScopes, verified.Scopes)
		c.Next()
	}
}

// RequireScope allows the request through when the authenticated client holds ANY one of the
// listed scope codes (OR semantics, mirroring RequirePermission). Otherwise it returns 403.
func RequireScope(scopeCodes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(scopeCodes) == 0 {
			c.Next()
			return
		}

		scopesVal, ok := c.Get(ctxAPIScopes)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "API authorization context not found.",
				"code":    "AUTH_CONTEXT_MISSING",
			})
			c.Abort()
			return
		}
		granted, _ := scopesVal.([]string)

		for _, required := range scopeCodes {
			for _, have := range granted {
				if strings.EqualFold(strings.TrimSpace(have), strings.TrimSpace(required)) {
					c.Next()
					return
				}
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "This API key is not authorized for the requested resource.",
			"code":    "INSUFFICIENT_SCOPE",
			"detail":  strings.Join(scopeCodes, ", "),
		})
		c.Abort()
	}
}

// --- Rate limiting (per-client, in-memory token bucket) ---
//
// NOTE: this limiter is in-process only — its counters reset on restart and are NOT shared
// across multiple instances. That is fine for the current single-process deployment; if the
// API is ever scaled horizontally, move this to a shared store (e.g. Redis).

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

type clientRateLimiter struct {
	mu       sync.Mutex
	buckets  map[uint64]*tokenBucket
	capacity float64
	perSec   float64 // refill rate (tokens per second)
}

var (
	extRateLimiterOnce sync.Once
	extRateLimiter     *clientRateLimiter
)

func getExtRateLimiter() *clientRateLimiter {
	extRateLimiterOnce.Do(func() {
		perMin := envIntDefault("EXT_API_RATE_LIMIT_PER_MIN", 100)
		if perMin <= 0 {
			perMin = 100
		}
		extRateLimiter = &clientRateLimiter{
			buckets:  make(map[uint64]*tokenBucket),
			capacity: float64(perMin),
			perSec:   float64(perMin) / 60.0,
		}
	})
	return extRateLimiter
}

// allow consumes one token for the client, returning (allowed, retryAfterSeconds).
func (l *clientRateLimiter) allow(clientID uint64) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[clientID]
	if !ok {
		b = &tokenBucket{tokens: l.capacity, lastRefill: now}
		l.buckets[clientID] = b
	}

	// Refill based on elapsed time, capped at capacity.
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.perSec
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Seconds until the next whole token is available.
	retry := int((1.0-b.tokens)/l.perSec) + 1
	return false, retry
}

// ExtRateLimit enforces the per-client request budget. It must run AFTER APIKeyAuthMiddleware
// so the client id is available.
func ExtRateLimit() gin.HandlerFunc {
	limiter := getExtRateLimiter()
	return func(c *gin.Context) {
		clientID, ok := clientIDFromContext(c)
		if !ok {
			// No authenticated client (should not happen after auth) — let it through; the
			// downstream handler / auth will reject appropriately.
			c.Next()
			return
		}
		allowed, retryAfter := limiter.allow(clientID)
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "Rate limit exceeded. Slow down and retry later.",
				"code":    "RATE_LIMIT_EXCEEDED",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// --- Audit logging ---

// ExtRequestLog records one row per external API request AFTER it completes. It captures the
// client, endpoint, requested filters and outcome — never the API key itself. Writes happen
// on a background goroutine so logging never adds latency to the response.
func ExtRequestLog() gin.HandlerFunc {
	svc := services.NewAPIClientService(nil)
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		latency := uint32(time.Since(start).Milliseconds())
		entry := models.APIRequestLog{
			Method:     c.Request.Method,
			Endpoint:   c.Request.URL.Path,
			HTTPStatus: uint16(c.Writer.Status()),
			LatencyMs:  &latency,
		}
		if ip := c.ClientIP(); ip != "" {
			entry.IP = &ip
		}
		if clientID, ok := clientIDFromContext(c); ok {
			entry.ClientID = &clientID
		}
		if v, ok := c.Get(ctxAPIKeyID); ok {
			if keyID, ok2 := v.(uint64); ok2 {
				entry.APIKeyID = &keyID
			}
		}
		if uids := strings.TrimSpace(c.Query("user_ids")); uids != "" {
			entry.RequestedUserIDs = &uids
		}
		if yf := parseYearQuery(c.Query("year_from")); yf != nil {
			entry.YearFrom = yf
		}
		if yt := parseYearQuery(c.Query("year_to")); yt != nil {
			entry.YearTo = yt
		}

		go func(e models.APIRequestLog) {
			_ = svc.WriteRequestLog(&e)
		}(entry)
	}
}

// RequireHTTPS rejects plaintext requests when EXT_API_REQUIRE_HTTPS=true, trusting the
// reverse proxy's X-Forwarded-Proto header. Off by default — TLS is normally terminated at
// the proxy, which is the primary enforcement point.
func RequireHTTPS() gin.HandlerFunc {
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("EXT_API_REQUIRE_HTTPS")), "true")
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		proto := c.GetHeader("X-Forwarded-Proto")
		if proto == "" && c.Request.TLS != nil {
			proto = "https"
		}
		if !strings.EqualFold(proto, "https") {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "HTTPS is required for this API.",
				"code":    "HTTPS_REQUIRED",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// --- helpers ---

func extractBearerToken(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func clientIDFromContext(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(ctxAPIClientID)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint64)
	return id, ok
}

func parseYearQuery(v string) *uint16 {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 65535 {
		return nil
	}
	y := uint16(n)
	return &y
}

func envIntDefault(key string, def int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return def
}
