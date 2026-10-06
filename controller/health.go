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

// Readyz 就绪探针（readiness）：检查主库、日志库与 Redis 连通性；关键依赖不可达返回 503。
// 供编排器判断「是否可接收流量」——依赖未就绪时不接流量，避免把请求打到半启动实例。
//
// Redis 语义（B3-2）：仅在启用（REDIS_CONN_STRING 已配置）时检查；未启用则视为
// 「未依赖 Redis，就绪」。可通过 READYZ_CHECK_REDIS=false 改为 fail-soft（只报告
// Redis 状态不影响就绪码），用于 Redis 仅作加速缓存、宕机不阻断主流程的部署。
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

	// B3-2 Redis：启用时探测可达性。fail-soft 模式只报告不改就绪码。
	if common.RedisEnabled && common.RDB != nil {
		redisSoft := !common.GetEnvOrDefaultBool("READYZ_CHECK_REDIS", true)
		if err := common.RDB.Ping(ctx).Err(); err != nil {
			checks["redis"] = "down: " + err.Error()
			if !redisSoft {
				ok = false
			}
		} else {
			checks["redis"] = "up"
		}
	} else {
		checks["redis"] = "disabled"
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
