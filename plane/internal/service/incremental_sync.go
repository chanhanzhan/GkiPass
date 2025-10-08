package service

import (
	"sync"
	"time"
)

// IncrementalSyncService 增量同步服务
type IncrementalSyncService struct {
	lastSync  map[string]time.Time
	mu        sync.RWMutex
	changeLog []ChangeRecord
	changesMu sync.Mutex
}

// ChangeRecord 变更记录
type ChangeRecord struct {
	EntityType string // node, tunnel, settings
	EntityID   string
	Action     string // create, update, delete
	Data       interface{}
	Timestamp  time.Time
}

// NewIncrementalSyncService 创建增量同步服务
func NewIncrementalSyncService() *IncrementalSyncService {
	return &IncrementalSyncService{
		lastSync:  make(map[string]time.Time),
		changeLog: make([]ChangeRecord, 0),
	}
}

// RecordChange 记录变更
func (iss *IncrementalSyncService) RecordChange(record ChangeRecord) {
	iss.changesMu.Lock()
	defer iss.changesMu.Unlock()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	iss.changeLog = append(iss.changeLog, record)
}

// GetChanges 获取变更（自上次同步以来）
func (iss *IncrementalSyncService) GetChanges(nodeID string) []ChangeRecord {
	iss.mu.RLock()
	lastSync := iss.lastSync[nodeID]
	iss.mu.RUnlock()

	iss.changesMu.Lock()
	defer iss.changesMu.Unlock()

	changes := make([]ChangeRecord, 0)
	for _, change := range iss.changeLog {
		if change.Timestamp.After(lastSync) {
			changes = append(changes, change)
		}
	}

	// 更新最后同步时间
	iss.mu.Lock()
	iss.lastSync[nodeID] = time.Now()
	iss.mu.Unlock()

	return changes
}

// CleanupOldChanges 清理旧变更记录
func (iss *IncrementalSyncService) CleanupOldChanges(retention time.Duration) {
	iss.changesMu.Lock()
	defer iss.changesMu.Unlock()

	cutoff := time.Now().Add(-retention)
	newLog := make([]ChangeRecord, 0)

	for _, change := range iss.changeLog {
		if change.Timestamp.After(cutoff) {
			newLog = append(newLog, change)
		}
	}

	iss.changeLog = newLog
}





