package controller

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/config"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// GetWebProtectionSettings 返回 Web 防刷配置（管理员）。
func GetWebProtectionSettings(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetWebProtectionSetting())
}

// UpdateWebProtectionSettings 更新 Web 防刷配置并热生效+持久化。
func UpdateWebProtectionSettings(c *gin.Context) {
	var input map[string]any
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	configMap := make(map[string]string)
	for key, value := range input {
		switch v := value.(type) {
		case bool:
			configMap[key] = strconv.FormatBool(v)
		case float64:
			configMap[key] = strconv.FormatInt(int64(v), 10)
		case string:
			configMap[key] = v
		case []any:
			// 策略维度数组（allowed_paths/blocked_paths/ua_allowlist）：JSON 编码存字符串，
			// UpdateConfigFromMap 对 slice 字段反序列化回数组。
			encoded, err := common.Marshal(v)
			if err != nil {
				common.ApiErrorI18n(c, i18n.MsgInvalidParams)
				return
			}
			configMap[key] = string(encoded)
		default:
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}
	target := config.GlobalConfig.Get("web_protection")
	if target == nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := config.UpdateConfigFromMap(target, configMap); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Validate.Struct(target); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidInput)
		return
	}
	if err := saveWebProtectionConfig(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, operation_setting.GetWebProtectionSetting())
}

func saveWebProtectionConfig() error {
	exported := config.GlobalConfig.ExportAllConfigs()
	for key, value := range exported {
		if !strings.HasPrefix(key, "web_protection.") {
			continue
		}
		if err := model.UpdateOption(key, value); err != nil {
			return err
		}
	}
	return nil
}

// GetServerStats 返回管理员端服务器实时状态：网络出入口 MB/s（近 1s 采样平均）与本节点规格/负载/磁盘（来自 system_instances 最近上报）。
func GetServerStats(c *gin.Context) {
	inMBps, outMBps := service.GetNetworkThroughput()
	data := gin.H{
		"network_in_mbps":  roundMBps(inMBps),
		"network_out_mbps": roundMBps(outMBps),
	}
	// B1-3：当前生效封禁数 + 今日请求量/带宽（Web 防刷可见性，供状态页展示）。
	data["in_flight"] = service.GetWebProtectionInFlight()
	// Recent bans (latest 10, active only) for the real-time status page + manual unban.
	if items, _, err := model.ListBannedIPs(1, 10); err == nil {
		recent := make([]model.BannedIP, 0, len(items))
		nowUnix := time.Now().Unix()
		for _, it := range items {
			if it.ExpiresAt == 0 || it.ExpiresAt > nowUnix {
				recent = append(recent, it)
			}
		}
		data["recent_bans"] = recent
	} else {
		data["recent_bans"] = []model.BannedIP{}
	}
	if banned, err := model.CountActiveBannedIPs(0); err == nil {
		data["banned_count"] = banned
	} else {
		data["banned_count"] = 0
	}
	if today := startOfTodayUnix(); today > 0 {
		if stat, err := model.TodayWebRequestStats(today); err == nil {
			data["today_request_count"] = stat.RequestCount
			data["today_bytes_sent"] = stat.BytesSent
			data["today_bytes_received"] = stat.BytesReceived
		}
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		if inst, err := model.GetSystemInstanceByNode(host); err == nil {
			var info any
			if common.UnmarshalJsonStr(inst.Info, &info) == nil {
				data["instance"] = info
			}
			data["started_at"] = inst.StartedAt
			data["last_seen_at"] = inst.LastSeenAt
			data["uptime_seconds"] = common.GetTimestamp() - inst.StartedAt
		}
	}
	common.ApiSuccess(c, data)
}

// startOfTodayUnix 返回当天 0 点（本地时区）的 unix 秒。
func startOfTodayUnix() int64 {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return today.Unix()
}

// RunWebProtectionMaintenanceController 手动触发维护（flush 日志 + 清理过期封禁/超期日志）。
func RunWebProtectionMaintenanceController(c *gin.Context) {
	if err := service.RunWebProtectionMaintenance(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func roundMBps(v float64) float64 {
	return math.Round(v*100) / 100
}

// WebRequestLogItem 日志聚合行（按 IP 汇总）。
type WebRequestLogItem struct {
	IP            string  `json:"ip"`
	RequestCount  int64   `json:"request_count"`
	RatePerSecond float64 `json:"rate_per_second"`
	BytesTotal    int64   `json:"bytes_total"`
	LastRequestAt int64   `json:"last_request_at"`
	Banned        bool    `json:"banned"`
	BanExpiresAt  int64   `json:"ban_expires_at"`
}

// ListWebRequestLogs 按 IP 聚合返回 Web 请求日志（管理员）。
// 可选 ip 过滤；sort=rate|count|bytes，order=asc|desc。
func ListWebRequestLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	ip := strings.TrimSpace(c.Query("ip"))
	sort := c.DefaultQuery("sort", "rate")
	order := strings.ToLower(c.DefaultQuery("order", "desc"))

	rows, total, err := model.AggregateWebRequestLogs(ip, sort, order, page, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]WebRequestLogItem, 0, len(rows))
	for _, r := range rows {
		banExpiresAt, banned := model.IsIPBanned(r.IP)
		items = append(items, WebRequestLogItem{
			IP:            r.IP,
			RequestCount:  r.RequestCount,
			RatePerSecond: r.RatePerSecond,
			BytesTotal:    r.BytesTotal,
			LastRequestAt: r.LastRequestAt,
			Banned:        banned,
			BanExpiresAt:  banExpiresAt,
		})
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total})
}

// ListWebRequestLogDetail 返回指定 IP 的路径明细（管理员）。
func ListWebRequestLogDetail(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	rows, total, err := model.WebRequestLogDetailByIP(ip, page, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows, "total": total})
}

// ListBannedIPsController 分页返回封禁列表（管理员）。
func ListBannedIPsController(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	items, total, err := model.ListBannedIPs(page, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total})
}

// BanIPController 一键封禁 IP（管理员）。body: {ip,minutes,reason}
func BanIPController(c *gin.Context) {
	var input struct {
		IP      string `json:"ip"`
		Minutes int64  `json:"minutes"`
		Reason  string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if input.IP == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if input.Reason == "" {
		input.Reason = "manual:admin_ban"
	}
	if err := model.BanIP(input.IP, input.Reason, getAdminUsername(c), input.Minutes); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// UnbanIPController 解封 IP（管理员）。body: {ip}
func UnbanIPController(c *gin.Context) {
	var input struct {
		IP string `json:"ip"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if input.IP == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.UnbanIP(input.IP); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// UnbanIPsController 批量解封 IP（管理员，B1-3）。body: {ips:[...]}
func UnbanIPsController(c *gin.Context) {
	var input struct {
		IPs []string `json:"ips"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(input.IPs) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.UnbanIPs(input.IPs); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// DeleteBannedIPController 按 ID 删除封禁记录（管理员）。
func DeleteBannedIPController(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteBannedIP(uint(id)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func getAdminUsername(c *gin.Context) string {
	if name := c.GetString("username"); name != "" {
		return name
	}
	// 管理员认证中间件已保证登录；取不到时回落为 admin。
	return "admin"
}
