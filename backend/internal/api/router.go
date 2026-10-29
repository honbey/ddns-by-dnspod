package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/metrics"
)

// RegisterRoutes registers all routes.
//
// Note: batch deletion uses /records/batch-delete instead of DELETE /records/batch,
// which would be ambiguous with the :id parameter of DELETE /records/:id.
func (s *Server) RegisterRoutes(r *gin.Engine, cfg config.Config) {
	// Both probes skip auth: a liveness check that needs a token cannot be used
	// by an orchestrator. Live is "the process is up"; ready is "it can serve".
	r.GET("/api/health", s.health)
	r.GET("/api/ready", s.ready)

	// The scrape endpoint. Public like every /metrics - the numbers are
	// operational detail, so keep the port behind the proxy; switch it off
	// entirely with metrics.enabled=false when nothing scrapes it.
	if cfg.Metrics.Enabled {
		r.GET("/metrics", gin.WrapH(metrics.Handler()))
	} else {
		// Answered here rather than left to the SPA fallback: an unregistered
		// path reaches it and comes back as index.html with a 200, which a
		// scraper cannot tell from a healthy endpoint with an empty answer.
		r.GET("/metrics", func(c *gin.Context) {
			fail(c, http.StatusNotFound, "metrics are disabled")
		})
	}

	// Runtime configuration of the SPA. Public for the same reason the bundle
	// is: the browser needs it before it can hold a token, and it carries
	// nothing that is not already in the page it serves.
	r.GET("/config.json", s.frontendConfig)

	// Public REST API for scripts. It deliberately bypasses the browser sign in:
	// the caller presents an API key instead of an access token, so the bearer
	// middleware must not run here, it would reject the missing header.
	if cfg.API.Enabled {
		v1 := r.Group("/api/v1")
		v1.Use(middleware.APIKeyAuth(s.store.DB()))
		{
			records := v1.Group("/domains/:domain/records")
			records.Use(middleware.APIKeyDomain())
			records.GET("", s.listRecords)
			records.POST("", s.createRecord)
			records.PUT("/:id", s.updateRecord)
			records.DELETE("/:id", s.deleteRecord)
		}
	} else {
		// Say why the calls fail, a bare 404 would look like a wrong URL
		r.Any("/api/v1/*path", apiKeyDisabled)
	}

	api := r.Group("/api")
	api.Use(middleware.Bearer(s.verifier))
	{
		// System
		api.GET("/ip", s.currentIP)

		// Tencent Cloud accounts
		api.GET("/accounts", s.listAccounts)
		api.POST("/accounts", s.createAccount)
		api.PUT("/accounts/:id", s.updateAccount)
		api.DELETE("/accounts/:id", s.deleteAccount)
		api.POST("/accounts/:id/verify", s.verifyAccount)

		// DNS records
		api.GET("/domains", s.listDomains)
		api.GET("/domains/:domain/records", s.listRecords)
		api.POST("/domains/:domain/records", s.createRecord)
		api.POST("/domains/:domain/records/batch", s.batchCreateRecords)
		api.POST("/domains/:domain/records/batch-delete", s.batchDeleteRecords)
		api.GET("/domains/:domain/lines", s.listRecordLines)
		api.GET("/domains/:domain/groups", s.listRecordGroups)
		api.PUT("/domains/:domain/records/:id", s.updateRecord)
		api.DELETE("/domains/:domain/records/:id", s.deleteRecord)
		api.PATCH("/domains/:domain/records/:id/status", s.setRecordStatus)

		// DDNS tasks
		api.GET("/ddns/tasks", s.listTasks)
		api.POST("/ddns/tasks", s.createTask)
		api.GET("/ddns/tasks/:id", s.getTask)
		api.PUT("/ddns/tasks/:id", s.updateTask)
		api.DELETE("/ddns/tasks/:id", s.deleteTask)
		api.POST("/ddns/tasks/:id/run", s.runTask)
		api.POST("/ddns/tasks/import-group/:domain", s.importTasksFromGroup)
		api.POST("/ddns/reload", s.reloadScheduler)

		// Change history: per task (the DNS record changes) and per host (the
		// public address itself, tracked independently of any domain)
		api.GET("/history", s.listHistory)
		api.GET("/stats/ip-changes", s.ipChangeStats)

		// Notification channels
		api.GET("/notify/channels", s.listChannels)
		api.POST("/notify/channels", s.createChannel)
		api.PUT("/notify/channels/:id", s.updateChannel)
		api.DELETE("/notify/channels/:id", s.deleteChannel)
		api.POST("/notify/channels/:id/test", s.testChannel)

		// Certificates: an ACME order per certificate, with the files landing in
		// a directory other TLS applications read.
		//
		// Issuance answers 202 and the run continues in the background, so the
		// caller watches the row rather than the request.
		api.GET("/certificates", s.listCertificates)
		api.POST("/certificates", s.createCertificate)
		api.PUT("/certificates/:id", s.updateCertificate)
		api.DELETE("/certificates/:id", s.deleteCertificate)
		api.POST("/certificates/:id/issue", s.issueCertificate)

		// API keys for the public API under /api/v1
		api.GET("/keys", s.listAPIKeys)
		api.POST("/keys", s.createAPIKey)
		api.PUT("/keys/:id", s.updateAPIKey)
		api.DELETE("/keys/:id", s.deleteAPIKey)
	}
}
