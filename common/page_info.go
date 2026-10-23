package common

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type PageInfo struct {
	Page     int `json:"page"`      // page num 页码
	PageSize int `json:"page_size"` // page size 页大小

	Total int `json:"total"` // 总条数，后设置
	Items any `json:"items"` // 数据，后设置
}

func (p *PageInfo) GetStartIdx() int {
	// 负数页/页大小会让 OFFSET 变负，PostgreSQL 与 MySQL 直接报错
	// （/api/log/、/api/log/self 返回 500）。这里兜底为 0。
	if p.Page < 1 || p.PageSize < 1 {
		return 0
	}
	return (p.Page - 1) * p.PageSize
}

func (p *PageInfo) GetEndIdx() int {
	return p.Page * p.PageSize
}

func (p *PageInfo) GetPageSize() int {
	return p.PageSize
}

func (p *PageInfo) GetPage() int {
	return p.Page
}

func (p *PageInfo) SetTotal(total int) {
	p.Total = total
}

func (p *PageInfo) SetItems(items any) {
	p.Items = items
}

func GetPageQuery(c *gin.Context) *PageInfo {
	pageInfo := &PageInfo{}
	// 手动获取并处理每个参数
	if page, err := strconv.Atoi(c.Query("p")); err == nil {
		pageInfo.Page = page
	}
	if pageSize, err := strconv.Atoi(c.Query("page_size")); err == nil {
		pageInfo.PageSize = pageSize
	}
	if pageInfo.Page < 1 {
		// page=0 与负数一律归到第 1 页。旧实现在这里重新读取 p 并原样赋值，
		// 负数会被放行 → GetStartIdx() 得负数 → PostgreSQL/MySQL 报
		// "OFFSET must not be negative" → 接口 500。
		pageInfo.Page = 1
	}

	if pageInfo.PageSize == 0 {
		// 兼容
		pageSize, _ := strconv.Atoi(c.Query("ps"))
		if pageSize != 0 {
			pageInfo.PageSize = pageSize
		}
		if pageInfo.PageSize == 0 {
			pageSize, _ = strconv.Atoi(c.Query("size")) // token page
			if pageSize != 0 {
				pageInfo.PageSize = pageSize
			}
		}
		if pageInfo.PageSize == 0 {
			pageInfo.PageSize = ItemsPerPage
		}
	}

	if pageInfo.PageSize < 1 {
		// 负数页大小同上：LIMIT 为负在 PostgreSQL 直接报错。
		pageInfo.PageSize = ItemsPerPage
	}
	if pageInfo.PageSize > 100 {
		pageInfo.PageSize = 100
	}

	return pageInfo
}
