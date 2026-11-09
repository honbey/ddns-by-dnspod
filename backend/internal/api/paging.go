package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultPageSize = 10
	maxPageSize     = 200
	// maxPage keeps the offset arithmetic inside int. It is computed as a
	// product, so an unbounded page number overflows and hands DNSPod an
	// enormous offset.
	maxPage = 1_000_000
)

// pageParams holds the pagination query parameters of a list endpoint
type pageParams struct {
	Page     int
	PageSize int
}

// parsePageParams reads page and page_size from the query, falling back to sane
// defaults whenever a value is missing or invalid.
func parsePageParams(c *gin.Context) pageParams {
	return parsePageParamsWith(c, defaultPageSize)
}

// parsePageParamsWith is parsePageParams for an endpoint whose natural page is
// a different size.
func parsePageParamsWith(c *gin.Context, defaultSize int) pageParams {
	page := atoiDefault(c.Query("page"), 1)
	size := atoiDefault(c.Query("page_size"), defaultSize)
	if size > maxPageSize {
		size = maxPageSize
	}
	if page > maxPage {
		page = maxPage
	}
	return pageParams{Page: page, PageSize: size}
}

// offset is the database or API offset of the requested page
func (p pageParams) offset() int {
	if p.Page <= 0 {
		return 0
	}
	return (p.Page - 1) * p.PageSize
}

// paged writes one page of items using the same envelope as the history endpoint
func paged(c *gin.Context, items interface{}, total int64, p pageParams) {
	ok(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      p.Page,
		"page_size": p.PageSize,
	})
}

// pagesFromTotal is the number of pages needed to show total items
func pagesFromTotal(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		return 1
	}
	return pages
}

// pageRows reads one page of rows in id order.
//
// Every list of stored rows goes through here, so the small tables cannot
// disagree with the record and task lists about what page 2 means or how big a
// page may get. It answers the request itself when the read fails, in which
// case the caller is done.
//
// The caller may still shape the rows - mask a password, resolve an id into a
// name - before handing them to paged.
func pageRows[T any](c *gin.Context, db *gorm.DB, items *[]T) (pageParams, int64, bool) {
	params := parsePageParams(c)

	var total int64
	if err := db.Model(new(T)).Count(&total).Error; err != nil {
		serverError(c, err)
		return params, 0, false
	}
	if err := db.Order("id").Limit(params.PageSize).Offset(params.offset()).Find(items).Error; err != nil {
		serverError(c, err)
		return params, 0, false
	}
	return params, total, true
}
