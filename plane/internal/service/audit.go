package service

import (
	"encoding/json"
	"fmt"
	"time"

	"gkipass/plane/db"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// AuditLog 审计日志
type AuditLog struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Username  string                 `json:"username"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Details   map[string]interface{} `json:"details"`
	IP        string                 `json:"ip"`
	UserAgent string                 `json:"user_agent"`
	Status    string                 `json:"status"`
	CreatedAt time.Time              `json:"created_at"`
}

// AuditService 审计服务
type AuditService struct {
	db *db.Manager
}

// NewAuditService 创建审计服务
func NewAuditService(dbManager *db.Manager) *AuditService {
	return &AuditService{db: dbManager}
}

// Log 记录审计日志
func (s *AuditService) Log(log *AuditLog) {
	log.CreatedAt = time.Now()

	// 记录到结构化日志
	logger.Info("审计日志",
		zap.String("user_id", log.UserID),
		zap.String("action", log.Action),
		zap.String("resource", log.Resource),
		zap.String("status", log.Status))

	// 存储到Redis（可选）
	if s.db.HasCache() {
		key := fmt.Sprintf("audit:%s", log.ID)
		data, _ := json.Marshal(log)
		_ = s.db.Cache.Redis.Set(key, string(data), 7*24*time.Hour)
	}
}
