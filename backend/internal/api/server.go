// Package api provides the HTTP interface layer.
package api

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/acme"
	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/auth"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ddns"
	"ddns-by-dnspod/internal/dnspod"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// clientCacheTTL is how long a DNSPod client is reused before it is rebuilt,
// which also bounds how long a rotated credential can stay in memory
const clientCacheTTL = 10 * time.Minute

type cachedClient struct {
	client  *dnspod.Client
	expires time.Time
}

// clientCache reuses DNSPod clients per account, so their read caches survive
// across requests instead of being dropped with a per-request client
type clientCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	clients map[uint]cachedClient
}

func newClientCache(ttl time.Duration) *clientCache {
	return &clientCache{ttl: ttl, clients: make(map[uint]cachedClient)}
}

func (cc *clientCache) removeExpiredLocked(now time.Time) {
	for id, c := range cc.clients {
		if now.After(c.expires) {
			delete(cc.clients, id)
		}
	}
}

// getOrBuild returns the cached client, building and storing it when the entry
// is missing or expired.
//
// The build runs under the lock so that an invalidation cannot be undone: the
// handlers that change an account drop its entry after committing, and without
// this a request that read the row before that commit landed would put the
// client it built from the old credentials back, where it stays valid for a
// whole TTL. Expired entries are swept here as well, because the only other
// time they are dropped is a lookup of that same account.
func (cc *clientCache) getOrBuild(id uint, build func() (*dnspod.Client, error)) (*dnspod.Client, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	now := time.Now()
	cc.removeExpiredLocked(now)
	if c, ok := cc.clients[id]; ok && now.Before(c.expires) {
		return c.client, nil
	}

	cli, err := build()
	if err != nil {
		return nil, err
	}
	cc.clients[id] = cachedClient{client: cli, expires: now.Add(cc.ttl)}
	return cli, nil
}

// remove drops a cached client, used when an account changes or is deleted
func (cc *clientCache) remove(id uint) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.clients, id)
}

// Server holds the dependencies required by the interface layer
type Server struct {
	cfg       *config.Config
	store     *store.Store
	scheduler *ddns.Scheduler
	detector  *ipdetect.Detector
	sender    *notifier.Sender
	verifier  *auth.Verifier
	acme      *acme.Manager
	clients   *clientCache
}

// NewServer builds the API server
func NewServer(cfg *config.Config, st *store.Store, sch *ddns.Scheduler, det *ipdetect.Detector, snd *notifier.Sender, v *auth.Verifier, certs *acme.Manager) *Server {
	return &Server{
		cfg:       cfg,
		store:     st,
		scheduler: sch,
		detector:  det,
		sender:    snd,
		verifier:  v,
		acme:      certs,
		clients:   newClientCache(clientCacheTTL),
	}
}

// clientFor builds a DNSPod client for an account; accountID 0 means the default account.
// Clients are reused per account so their read caches actually take effect.
func (s *Server) clientFor(accountID uint) (*dnspod.Client, error) {
	// Resolved to an id only: the row itself is read again inside the critical
	// section below. A credential rotation that commits while this request is
	// running drops the cached client, and a client built from a row read before
	// that commit would put the old credentials back for a whole TTL.
	if accountID == 0 {
		account, err := s.resolveAccount(0)
		if err != nil {
			return nil, err
		}
		accountID = account.ID
	}
	return s.clients.getOrBuild(accountID, func() (*dnspod.Client, error) {
		account, err := s.resolveAccount(accountID)
		if err != nil {
			return nil, err
		}
		secretKey, err := s.store.Encryptor().Decrypt(account.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt account secret key: %w", err)
		}
		ttl := dnspod.CacheTTL{}
		if s.cfg.Cache.Enabled {
			ttl = dnspod.CacheTTL{
				Domains: time.Duration(s.cfg.Cache.DomainsSec) * time.Second,
				Records: time.Duration(s.cfg.Cache.RecordsSec) * time.Second,
				Lines:   time.Duration(s.cfg.Cache.LinesSec) * time.Second,
			}
		}
		return dnspod.NewClientWithCache(account.SecretID, secretKey, s.cfg.DNSPod.Endpoint,
			s.cfg.DNSPod.Timeout, ttl)
	})
}

// resolveAccount resolves an account: explicit ID first, then the default account, then a single row
func (s *Server) resolveAccount(accountID uint) (*model.Account, error) {
	var account model.Account

	if accountID > 0 {
		if err := s.store.DB().First(&account, accountID).Error; err != nil {
			return nil, &accountError{msg: fmt.Sprintf("account not found (id=%d)", accountID)}
		}
		return &account, nil
	}

	if err := s.store.DB().Where("is_default = ?", true).First(&account).Error; err == nil {
		return &account, nil
	}
	if err := s.store.DB().First(&account).Error; err != nil {
		return nil, &accountError{
			msg:  "no Tencent Cloud account is configured yet, add one first",
			code: CodeNoAccount,
		}
	}
	return &account, nil
}

// parseID parses the id path parameter. On failure the response is already written and 0 is returned, so callers must return at once.
func parseID(c *gin.Context) uint {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		badRequest(c, "invalid resource id")
		return 0
	}
	return uint(id)
}

// parseAccountID reads account_id from the query; 0 means the default account.
//
// An API key call may not choose an account: the key is bound to one, and
// letting the request pick would be a way around that binding.
//
// The boolean reports whether the value was usable. Unlike parseID it cannot use
// 0 as the failure marker, because 0 is the valid "default account" answer. When
// it is false the response is already written and the caller must return at once,
// otherwise the handler writes a second response on top of the 400.
func parseAccountID(c *gin.Context) (uint, bool) {
	if key, ok := middleware.APIKeyFrom(c); ok {
		return key.AccountID, true
	}

	raw := c.Query("account_id")
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		badRequest(c, "invalid account_id")
		return 0, false
	}
	return uint(id), true
}

// parseGroupID reads the group_id query parameter; 0 means no group filter.
//
// An unparsable value is ignored rather than rejected: a group is only ever a
// filter here, so falling back to "all records" keeps a stale link useful
// instead of turning it into an error.
func parseGroupID(c *gin.Context) uint64 {
	raw := strings.TrimSpace(c.Query("group_id"))
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// parseRecordID reads the DNSPod record ID from the path parameter
func parseRecordID(c *gin.Context) uint64 {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		badRequest(c, "invalid record id")
		return 0
	}
	return id
}
