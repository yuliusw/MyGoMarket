package utils

import (
	"log"
	"time"

	"github.com/casbin/casbin/v3"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type CasbinManager struct {
	cache     *lru.Cache[string, cachedEnforcer]
	sf        singleflight.Group
	modelPath string
	db        *gorm.DB
	ttl       time.Duration
}

type cachedEnforcer struct {
	enforcer *casbin.Enforcer
	loadedAt time.Time
}

var EnforcerPool *CasbinManager

// InitCasbinPool 初始化缓存池 (maxDomains 建议设为 1000~5000，视内存而定)
func InitCasbinPool(db *gorm.DB, modelPath string, maxDomains int) {
	InitCasbinPoolWithTTL(db, modelPath, maxDomains, 10*time.Minute)
}

func InitCasbinPoolWithTTL(db *gorm.DB, modelPath string, maxDomains int, ttl time.Duration) {
	cache, err := lru.New[string, cachedEnforcer](maxDomains)
	if err != nil {
		log.Fatalf("Failed to create Casbin LRU cache: %v", err)
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	EnforcerPool = &CasbinManager{
		cache:     cache,
		modelPath: modelPath,
		db:        db,
		ttl:       ttl,
	}
}

// GetEnforcer 线程安全地获取或懒加载专属某个 Domain 的轻量 Enforcer
func (m *CasbinManager) GetEnforcer(domainID string) (*casbin.Enforcer, error) {
	if e, ok := m.cache.Get(domainID); ok {
		if time.Since(e.loadedAt) < m.ttl {
			return e.enforcer, nil
		}
		m.cache.Remove(domainID)
	}

	// 依靠 singleflight 机制合并并发请求，防止数据库击穿
	val, err, _ := m.sf.Do(domainID, func() (interface{}, error) {
		if e, ok := m.cache.Get(domainID); ok {
			if time.Since(e.loadedAt) < m.ttl {
				return e.enforcer, nil
			}
			m.cache.Remove(domainID)
		}

		adapter := NewCustomDBAdapter(m.db)
		e, err := casbin.NewEnforcer(m.modelPath, adapter)
		if err != nil {
			return nil, err
		}

		// 仅仅拉取该域的数据
		err = e.LoadFilteredPolicy(&DomainFilter{DomainID: domainID})
		if err != nil {
			return nil, err
		}

		m.cache.Add(domainID, cachedEnforcer{enforcer: e, loadedAt: time.Now()})
		log.Printf("[Casbin Memory Cache] Lazy loaded successfully for Domain: %s", domainID)
		return e, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*casbin.Enforcer), nil
}

// InvalidateDomain 驱逐单个域的内存缓存（由 RocketMQ 监听到变更时调用）
func (m *CasbinManager) InvalidateDomain(domainID string) {
	m.cache.Remove(domainID)
	log.Printf("[Casbin Memory Cache] Evicted cached domain due to sync event: %s", domainID)
}

// PurgeAll 驱逐所有域的内存缓存（当全局权限 P 规则变化时调用）
func (m *CasbinManager) PurgeAll() {
	m.cache.Purge()
	log.Println("[Casbin Memory Cache] Purged all cached domains due to global policy update")
}
