package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/model"
)

// listHistory returns the change history, filterable by task, domain and status, with pagination
func (s *Server) listHistory(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	pageSize := atoiDefault(c.Query("page_size"), 20)
	if pageSize > 200 {
		pageSize = 200
	}
	// Capped like parsePageParams does, so the offset below stays inside int
	if page > maxPage {
		page = maxPage
	}

	q := s.store.DB().Model(&model.DDNSHistory{})

	if v := c.Query("task_id"); v != "" {
		// Validate before it reaches the query builder: the value is numeric by contract
		taskID, err := strconv.ParseUint(v, 10, 64)
		if err != nil || taskID == 0 {
			badRequest(c, "task_id must be a positive integer")
			return
		}
		q = q.Where("task_id = ?", taskID)
	}
	if v := c.Query("domain"); v != "" {
		q = q.Where("domain = ?", v)
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}

	var items []model.DDNSHistory
	err := q.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	if err != nil {
		serverError(c, err)
		return
	}

	ok(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ipChangeStats returns recent public IP changes of this host, for charting.
//
// Only real changes are returned: the first row of a family has no previous
// address to compare against, so it carries no duration and would plot a
// misleading zero-height bar. It still has to be stored, because it is the
// baseline the next change measures its lifetime against.
func (s *Server) ipChangeStats(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 50)
	if limit > 200 {
		limit = 200
	}

	q := s.store.DB().Where("old_ip <> ''")
	if v := strings.TrimSpace(c.Query("family")); v != "" {
		q = q.Where("family = ?", v)
	}

	var items []model.PublicIPChange
	err := q.Order("id DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		serverError(c, err)
		return
	}

	// The chart reads better in chronological order
	points := make([]gin.H, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		points = append(points, gin.H{
			"time":         it.CreatedAt,
			"family":       it.Family,
			"old_ip":       it.OldIP,
			"new_ip":       it.NewIP,
			"duration_sec": it.DurationSec,
		})
	}

	ok(c, points)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
