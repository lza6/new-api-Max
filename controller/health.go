package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// Healthz 存活探针（liveness）：进程可响应即 200，不做依赖检查。
// 供负载均衡/容器编排（K8s livenessProbe、Caddy active health check）判断「进程是否活着」。
// 与 /api/status 的区别：不读 console/option 设置、不查 DB，极轻量，可高频探测。
func Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": common.Version,
		"uptime":  time.Now().Unix() - common.StartTime,
	})
}

// Readyz 就绪探针（readiness）：检查主库与日志库连通性；任一不可达返回 503。
// 供编排器判断「是否可接收流量」——依赖未就绪时不接流量，避免把请求打到半启动实例。
func Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{}
	ok := true

	if model.DB != nil {
		if sqlDB, err := model.DB.DB(); err == nil {
			if err := sqlDB.PingContext(ctx); err != nil {
				checks["database"] = "down: " + err.Error()
				ok = false
			} else {
				checks["database"] = "up"
			}
		} else {
			checks["database"] = "down: " + err.Error()
			ok = false
		}
	} else {
		checks["database"] = "down: not initialized"
		ok = false
	}

	// 日志库：与主库同实例时跳过重复 ping（否则重复检测无意义）。
	if model.LOG_DB != nil && model.LOG_DB != model.DB {
		if sqlDB, err := model.LOG_DB.DB(); err == nil {
			if err := sqlDB.PingContext(ctx); err != nil {
				checks["log_database"] = "down: " + err.Error()
				ok = false
			} else {
				checks["log_database"] = "up"
			}
		} else {
			checks["log_database"] = "down: " + err.Error()
			ok = false
		}
	}

	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"checks": checks,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": checks,
	})
}
