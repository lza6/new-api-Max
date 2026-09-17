// placeholder
package service

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

const (
	webProtectionContextKey  = "web_protection_begin"
	webProtectionFlushEvery  = 30 * time.Second
	webProtectionIdleTTL     = 10 * time.Minute
	webProtectionRetryAfter  = 60
	webProtectionBanCacheTTL = 5 * time.Second
)

// webTokenBucket 每-IP 令牌桶：容量=burst，速率=perSec/s。
type webTokenBucket struct {
	tokens   float64
	last     time.Time
	rate     float64
	capacity float64
}

func (b *webTokenBucket) take(now time.Time) bool {
	if b.last.IsZero() {
		b.tokens = b.capacity
		b.last = now
	} else if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(b.capacity, b.tokens+elapsed*b.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// webPathAgg 单条路径的聚合明细。
type webPathAgg struct {
	method        string
	status        int
	count         int
	bytesSent     int64
	bytesReceived int64
	lastAt        int64
}

// webWindowAgg 一个聚合窗口（默认 60s）内的计数。
type webWindowAgg struct {
	windowStart   int64
	count         int
	rejected      int
	bytesSent     int64
	bytesReceived int64
	lastAt        int64
	lastUA        string
	paths         map[string]*webPathAgg
	banAttempted  bool
}

func newWebWindowAgg(windowStart int64) *webWindowAgg {
	return &webWindowAgg{windowStart: windowStart, paths: map[string]*webPathAgg{}}
}

type webIPState struct {
	bucket   *webTokenBucket
	window   *webWindowAgg
	lastSeen time.Time
}

type webBanCacheEntry struct {
	banned    bool
	expiresAt int64
	checkedAt time.Time
}

// webProtectionTracker 维护每-IP 令牌桶、窗口聚合、封禁缓存与待落库批次。
type webProtectionTracker struct {
	mu          sync.Mutex
	ipState     map[string]*webIPState
	lastFlush   time.Time
	banCache    map[string]webBanCacheEntry
	pendingRows []model.WebRequestLog
}

var webProtectionTrackerInstance = &webProtectionTracker{
	ipState:  map[string]*webIPState{},
	banCache: map[string]webBanCacheEntry{},
}

// InitWebProtectionTracker 惰性初始化（中间件首次挂载时调用）。
func InitWebProtectionTracker() {
	// 实例为包级单例，无需额外初始化；此函数保留作为显式契约。
}

func webProtectionWindowStart(now time.Time, windowSec int64) int64 {
	if windowSec <= 1 {
		return now.Unix()
	}
	return now.Unix() - now.Unix()%windowSec
}

func (t *webProtectionTracker) isIPBannedCached(ip string, now time.Time) bool {
	if entry, ok := t.banCache[ip]; ok && now.Sub(entry.checkedAt) < webProtectionBanCacheTTL {
		if entry.banned {
			if entry.expiresAt == 0 || entry.expiresAt > now.Unix() {
				return true
			}
		} else {
			return false
		}
	}
	expiresAt, banned := model.IsIPBanned(ip)
	t.banCache[ip] = webBanCacheEntry{banned: banned, expiresAt: expiresAt, checkedAt: now}
	return banned
}

// TrackWebRequestBegin 在请求进入时判定：/v1 放行、封禁拒绝、超限拒绝、放行并计数。
// 返回 false 表示请求已被 429 终止。
func TrackWebRequestBegin(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return true
	}
	if !operation_setting.IsWebProtectionEnabled() {
		return true
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1") {
		return true
	}
	now := time.Now()
	ip := c.ClientIP()
	perSec, burst, windowSec := operation_setting.GetWebProtectionLimit()

	t := webProtectionTrackerInstance
	t.mu.Lock()
	if t.isIPBannedCached(ip, now) {
		t.mu.Unlock()
		writeWebProtectionReject(c, webProtectionRetryAfter, "ip_banned", "ip banned")
		return false
	}
	st := t.ipState[ip]
	windowStart := webProtectionWindowStart(now, windowSec)
	if st == nil {
		st = &webIPState{
			bucket: &webTokenBucket{rate: float64(perSec), capacity: float64(burst)},
			window: newWebWindowAgg(windowStart),
		}
		t.ipState[ip] = st
	} else if st.window.windowStart != windowStart {
		t.closeWindowLocked(ip, st, windowSec)
		st.window = newWebWindowAgg(windowStart)
	}
	st.lastSeen = now
	if !st.bucket.take(now) {
		st.window.rejected++
		threshold := operation_setting.GetAutoBanThreshold()
		shouldBan := operation_setting.IsAutoBanEnabled() && !st.window.banAttempted && st.window.rejected >= threshold
		if shouldBan {
			st.window.banAttempted = true
		}
		t.mu.Unlock()
		if shouldBan {
			minutes := operation_setting.GetAutoBanMinutes()
			if err := model.BanIP(ip, "auto:web_rate_limit", "auto", minutes); err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("auto ban ip %s failed: %v", ip, err))
			}
		}
		writeWebProtectionReject(c, webProtectionRetryAfter, "rate_limited", "rate limited, please retry later")
		return false
	}
	st.window.count++
	if c.Request.ContentLength > 0 {
		st.window.bytesReceived += c.Request.ContentLength
	}
	st.window.lastAt = now.Unix()
	c.Set(webProtectionContextKey, ip)
	t.mu.Unlock()
	return true
}

// TrackWebRequestEnd 在请求结束后聚合计数/字节并周期落库。
func TrackWebRequestEnd(c *gin.Context, status int) {
	if c == nil || !operation_setting.IsWebProtectionEnabled() {
		return
	}
	ipVal, ok := c.Get(webProtectionContextKey)
	ip, _ := ipVal.(string)
	if !ok || ip == "" || c.Request == nil || c.Request.URL == nil {
		return
	}
	now := time.Now()
	path := c.Request.URL.Path
	method := c.Request.Method
	userAgent := c.Request.UserAgent()
	bytesSent := int64(c.Writer.Size())
	windowSec := int64(60)
	if _, _, ws := operation_setting.GetWebProtectionLimit(); ws > 0 {
		windowSec = ws
	}
	t := webProtectionTrackerInstance
	t.mu.Lock()
	st := t.ipState[ip]
	if st == nil {
		t.mu.Unlock()
		return
	}
	st.lastSeen = now
	agg := st.window
	if agg.windowStart != webProtectionWindowStart(now, windowSec) {
		t.closeWindowLocked(ip, st, windowSec)
		agg = st.window
	}
	agg.bytesSent += bytesSent
	agg.lastAt = now.Unix()
	if agg.lastUA == "" && len(userAgent) > 0 {
		agg.lastUA = userAgent
	}
	p := agg.paths[path]
	if p == nil {
		p = &webPathAgg{method: method, status: status}
		agg.paths[path] = p
	}
	p.count++
	p.status = status
	p.bytesSent += bytesSent
	p.lastAt = now.Unix()
	t.mu.Unlock()
	t.flushIfDue(now)
}

// closeWindowLocked 关闭一个过期窗口并生成待落库日志行（每路径一行，
// 接收字节只记在该窗口第一条路径行，IP 聚合按 SUM 恰好精确）。
func (t *webProtectionTracker) closeWindowLocked(ip string, st *webIPState, windowSec int64) {
	agg := st.window
	if agg == nil || agg.count <= 0 {
		return
	}
	rate := float64(agg.count) / float64(windowSec)
	ua := agg.lastUA
	if len(ua) > 512 {
		ua = ua[:512]
	}
	idx := 0
	for path, p := range agg.paths {
		received := int64(0)
		if idx == 0 {
			received = agg.bytesReceived
		}
		idx++
		t.pendingRows = append(t.pendingRows, model.WebRequestLog{
			IP:            ip,
			Path:          path,
			Method:        p.method,
			Status:        p.status,
			BytesSent:     p.bytesSent,
			BytesReceived: received,
			UserAgent:     ua,
			RequestCount:  p.count,
			RatePerSecond: rate,
			WindowStart:   agg.windowStart,
		})
	}
	if len(t.pendingRows) >= 500 {
		t.flushRowsLocked()
	}
}

// flushIfDue 每 30s 落库一次并清理闲置 IP。
func (t *webProtectionTracker) flushIfDue(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now.Sub(t.lastFlush) < webProtectionFlushEvery && len(t.pendingRows) == 0 {
		return
	}
	for ip, st := range t.ipState {
		if now.Sub(st.lastSeen) > webProtectionIdleTTL {
			t.closeWindowLocked(ip, st, 60)
			delete(t.ipState, ip)
		}
	}
	t.flushRowsLocked()
	t.lastFlush = now
}

// flushRowsLocked 把待落库批次写入 DB（异步，失败仅告警）。
func (t *webProtectionTracker) flushRowsLocked() {
	if len(t.pendingRows) == 0 {
		return
	}
	if !operation_setting.IsWebLogEnabled() {
		t.pendingRows = nil
		return
	}
	rows := t.pendingRows
	t.pendingRows = nil
	go func() {
		if err := model.RecordWebRequestLogs(rows); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("record web request logs failed: %v", err))
		}
	}()
}

// FlushWebRequestLogs 强制落库（测试/优雅退出用）。
func FlushWebRequestLogs() error {
	t := webProtectionTrackerInstance
	t.mu.Lock()
	if len(t.pendingRows) == 0 {
		t.mu.Unlock()
		return nil
	}
	rows := t.pendingRows
	t.pendingRows = nil
	t.mu.Unlock()
	return model.RecordWebRequestLogs(rows)
}

func writeWebProtectionReject(c *gin.Context, retryAfter int64, code, message string) {
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message": message,
			"type":    code,
			"code":    code,
		},
	})
}
