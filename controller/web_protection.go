package controller

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
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

// WebRequestLogItem 日志聚合行（按 IP 汇总）。
type WebRequestLogItem struct {
	IP             string  `json:"ip"`
	RequestCount   int64   `json:"request_count"`
	RatePerSecond  float64 `json:"rate_per_second"`
	BytesTotal     int64   `json:"bytes_total"`
	LastRequestAt  int64   `json:"last_request_at"`
	Banned         bool    `json:"banned"`
	BanExpiresAt   int64   `json:"ban_expires_at"`
}

// ListWebRequestLogs 按 IP 聚合返回 Web 请求日志（管理员）。
// 可选 ip 过滤；sort=rate|count|bytes，order=asc|desc。
func ListWebRequestLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page <= 0 { page = 1 }
	if size <= 0 || size > 100 { size = 20 }
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
	if page <= 0 { page = 1 }
	if size <= 0 || size > 100 { size = 20 }
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
	var input struct { IP string `json:"ip"` }
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
