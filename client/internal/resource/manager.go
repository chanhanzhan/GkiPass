package resource

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// ResourceType 资源类型
type ResourceType int

const (
	ResourceTypeFile ResourceType = iota
	ResourceTypeConnection
	ResourceTypeSocket
	ResourceTypeMemory
	ResourceTypeGoroutine
)

func (rt ResourceType) String() string {
	switch rt {
	case ResourceTypeFile:
		return "file"
	case ResourceTypeConnection:
		return "connection"
	case ResourceTypeSocket:
		return "socket"
	case ResourceTypeMemory:
		return "memory"
	case ResourceTypeGoroutine:
		return "goroutine"
	default:
		return "unknown"
	}
}

// Resource 资源接口
type Resource interface {
	GetID() string
	GetType() ResourceType
	GetCreatedAt() time.Time
	GetLastUsed() time.Time
	UpdateLastUsed()
	IsExpired(ttl time.Duration) bool
	Release() error
	GetSize() int64
}

// BaseResource 基础资源
type BaseResource struct {
	id        string
	resType   ResourceType
	createdAt time.Time
	lastUsed  atomic.Int64 // Unix timestamp
	size      int64
}

func NewBaseResource(id string, resType ResourceType, size int64) *BaseResource {
	now := time.Now()
	br := &BaseResource{
		id:        id,
		resType:   resType,
		createdAt: now,
		size:      size,
	}
	br.lastUsed.Store(now.Unix())
	return br
}

func (br *BaseResource) GetID() string {
	return br.id
}

func (br *BaseResource) GetType() ResourceType {
	return br.resType
}

func (br *BaseResource) GetCreatedAt() time.Time {
	return br.createdAt
}

func (br *BaseResource) GetLastUsed() time.Time {
	return time.Unix(br.lastUsed.Load(), 0)
}

func (br *BaseResource) UpdateLastUsed() {
	br.lastUsed.Store(time.Now().Unix())
}

func (br *BaseResource) IsExpired(ttl time.Duration) bool {
	return time.Since(br.GetLastUsed()) > ttl
}

func (br *BaseResource) GetSize() int64 {
	return br.size
}

// FileResource 文件资源
type FileResource struct {
	*BaseResource
	file     *os.File
	path     string
	readonly bool
}

func NewFileResource(id, path string, file *os.File, readonly bool) *FileResource {
	stat, _ := file.Stat()
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}

	return &FileResource{
		BaseResource: NewBaseResource(id, ResourceTypeFile, size),
		file:         file,
		path:         path,
		readonly:     readonly,
	}
}

func (fr *FileResource) Release() error {
	if fr.file != nil {
		return fr.file.Close()
	}
	return nil
}

func (fr *FileResource) GetPath() string {
	return fr.path
}

func (fr *FileResource) IsReadonly() bool {
	return fr.readonly
}

// ConnectionResource 连接资源
type ConnectionResource struct {
	*BaseResource
	conn       net.Conn
	remoteAddr string
	localAddr  string
	protocol   string
	active     atomic.Bool
}

func NewConnectionResource(id string, conn net.Conn, protocol string) *ConnectionResource {
	cr := &ConnectionResource{
		BaseResource: NewBaseResource(id, ResourceTypeConnection, 0),
		conn:         conn,
		protocol:     protocol,
	}

	if conn != nil {
		if conn.RemoteAddr() != nil {
			cr.remoteAddr = conn.RemoteAddr().String()
		}
		if conn.LocalAddr() != nil {
			cr.localAddr = conn.LocalAddr().String()
		}
	}

	cr.active.Store(true)
	return cr
}

func (cr *ConnectionResource) Release() error {
	cr.active.Store(false)
	if cr.conn != nil {
		return cr.conn.Close()
	}
	return nil
}

func (cr *ConnectionResource) IsActive() bool {
	return cr.active.Load()
}

func (cr *ConnectionResource) GetConnection() net.Conn {
	cr.UpdateLastUsed()
	return cr.conn
}

func (cr *ConnectionResource) GetRemoteAddr() string {
	return cr.remoteAddr
}

func (cr *ConnectionResource) GetLocalAddr() string {
	return cr.localAddr
}

func (cr *ConnectionResource) GetProtocol() string {
	return cr.protocol
}

// ResourceManager 资源管理器
type ResourceManager struct {
	config *ResourceConfig
	logger *zap.Logger

	// 资源存储
	resources   map[string]Resource
	resourceMux sync.RWMutex

	// 分类索引
	fileResources       map[string]*FileResource
	connectionResources map[string]*ConnectionResource

	// 统计信息
	stats struct {
		totalCreated  atomic.Int64
		totalReleased atomic.Int64
		currentCount  atomic.Int64
		totalSize     atomic.Int64
		cleanupRuns   atomic.Int64
		recycledCount atomic.Int64
	}

	// 限制和配额
	limits    *ResourceLimits
	quotas    map[ResourceType]*ResourceQuota
	quotasMux sync.RWMutex

	// 清理和监控
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	cleanupCh chan struct{}
}

// ResourceConfig 资源配置
type ResourceConfig struct {
	// 基础配置
	MaxResources     int           `json:"max_resources"`
	CleanupInterval  time.Duration `json:"cleanup_interval"`
	DefaultTTL       time.Duration `json:"default_ttl"`
	EnableMonitoring bool          `json:"enable_monitoring"`

	// 文件资源配置
	MaxFileHandles  int           `json:"max_file_handles"`
	FileTTL         time.Duration `json:"file_ttl"`
	FileCleanupSize int64         `json:"file_cleanup_size"`

	// 连接资源配置
	MaxConnections    int           `json:"max_connections"`
	ConnectionTTL     time.Duration `json:"connection_ttl"`
	ConnectionTimeout time.Duration `json:"connection_timeout"`

	// 内存管理
	MaxMemoryUsage         int64   `json:"max_memory_usage"`
	MemoryCleanupThreshold float64 `json:"memory_cleanup_threshold"`
}

// DefaultResourceConfig 默认资源配置
func DefaultResourceConfig() *ResourceConfig {
	return &ResourceConfig{
		MaxResources:           10000,
		CleanupInterval:        5 * time.Minute,
		DefaultTTL:             30 * time.Minute,
		EnableMonitoring:       true,
		MaxFileHandles:         1000,
		FileTTL:                1 * time.Hour,
		FileCleanupSize:        100 * 1024 * 1024, // 100MB
		MaxConnections:         5000,
		ConnectionTTL:          10 * time.Minute,
		ConnectionTimeout:      30 * time.Second,
		MaxMemoryUsage:         500 * 1024 * 1024, // 500MB
		MemoryCleanupThreshold: 0.8,
	}
}

// ResourceLimits 资源限制
type ResourceLimits struct {
	MaxFileDescriptors int
	MaxConnections     int
	MaxMemoryUsage     int64
	MaxGoroutines      int
}

// ResourceQuota 资源配额
type ResourceQuota struct {
	Type        ResourceType
	Current     atomic.Int64
	Max         int64
	Reserved    int64
	LastUpdated atomic.Int64
}

// NewResourceManager 创建资源管理器
func NewResourceManager(config *ResourceConfig) *ResourceManager {
	if config == nil {
		config = DefaultResourceConfig()
	}

	rm := &ResourceManager{
		config:              config,
		logger:              zap.L().Named("resource-manager"),
		resources:           make(map[string]Resource),
		fileResources:       make(map[string]*FileResource),
		connectionResources: make(map[string]*ConnectionResource),
		quotas:              make(map[ResourceType]*ResourceQuota),
		cleanupCh:           make(chan struct{}, 1),
	}

	// 初始化系统限制
	rm.initSystemLimits()

	// 初始化资源配额
	rm.initResourceQuotas()

	return rm
}

// Start 启动资源管理器
func (rm *ResourceManager) Start(ctx context.Context) error {
	rm.ctx, rm.cancel = context.WithCancel(ctx)

	// 启动清理协程
	rm.wg.Add(1)
	go rm.cleanupLoop()

	// 启动监控协程
	if rm.config.EnableMonitoring {
		rm.wg.Add(1)
		go rm.monitoringLoop()
	}

	// 启动资源回收协程
	rm.wg.Add(1)
	go rm.recycleLoop()

	rm.logger.Info("🔧 资源管理器启动",
		zap.Int("max_resources", rm.config.MaxResources),
		zap.Int("max_file_handles", rm.config.MaxFileHandles),
		zap.Int("max_connections", rm.config.MaxConnections),
		zap.Duration("cleanup_interval", rm.config.CleanupInterval))

	return nil
}

// Stop 停止资源管理器
func (rm *ResourceManager) Stop() error {
	rm.logger.Info("🛑 正在停止资源管理器...")

	if rm.cancel != nil {
		rm.cancel()
	}

	// 释放所有资源
	rm.releaseAllResources()

	// 等待协程结束
	rm.wg.Wait()

	rm.logger.Info("✅ 资源管理器已停止")
	return nil
}

// AcquireFileResource 获取文件资源
func (rm *ResourceManager) AcquireFileResource(path string, flags int, perm os.FileMode) (*FileResource, error) {
	// 检查配额
	if !rm.checkQuota(ResourceTypeFile) {
		return nil, fmt.Errorf("文件句柄配额不足")
	}

	// 检查是否已存在
	rm.resourceMux.RLock()
	if existing, exists := rm.fileResources[path]; exists {
		existing.UpdateLastUsed()
		rm.resourceMux.RUnlock()
		return existing, nil
	}
	rm.resourceMux.RUnlock()

	// 打开文件
	file, err := os.OpenFile(path, flags, perm)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}

	// 创建文件资源
	id := fmt.Sprintf("file:%s:%d", path, time.Now().UnixNano())
	readonly := (flags&os.O_WRONLY) == 0 && (flags&os.O_RDWR) == 0
	resource := NewFileResource(id, path, file, readonly)

	// 注册资源
	rm.registerResource(resource)

	rm.logger.Debug("获取文件资源",
		zap.String("path", path),
		zap.String("id", id),
		zap.Bool("readonly", readonly))

	return resource, nil
}

// AcquireConnectionResource 获取连接资源
func (rm *ResourceManager) AcquireConnectionResource(network, address string) (*ConnectionResource, error) {
	// 检查配额
	if !rm.checkQuota(ResourceTypeConnection) {
		return nil, fmt.Errorf("连接配额不足")
	}

	// 创建连接
	conn, err := net.DialTimeout(network, address, rm.config.ConnectionTimeout)
	if err != nil {
		return nil, fmt.Errorf("建立连接失败: %w", err)
	}

	// 创建连接资源
	id := fmt.Sprintf("conn:%s:%s:%d", network, address, time.Now().UnixNano())
	resource := NewConnectionResource(id, conn, network)

	// 注册资源
	rm.registerResource(resource)

	rm.logger.Debug("获取连接资源",
		zap.String("network", network),
		zap.String("address", address),
		zap.String("id", id))

	return resource, nil
}

// ReleaseResource 释放资源
func (rm *ResourceManager) ReleaseResource(id string) error {
	rm.resourceMux.Lock()
	defer rm.resourceMux.Unlock()

	resource, exists := rm.resources[id]
	if !exists {
		return fmt.Errorf("资源不存在: %s", id)
	}

	// 释放资源
	if err := resource.Release(); err != nil {
		rm.logger.Error("释放资源失败",
			zap.String("id", id),
			zap.String("type", resource.GetType().String()),
			zap.Error(err))
		return err
	}

	// 从索引中移除
	rm.removeFromIndexes(resource)

	// 从主存储中移除
	delete(rm.resources, id)

	// 更新配额
	rm.updateQuota(resource.GetType(), -1)

	// 更新统计
	rm.stats.totalReleased.Add(1)
	rm.stats.currentCount.Add(-1)
	rm.stats.totalSize.Add(-resource.GetSize())

	rm.logger.Debug("释放资源",
		zap.String("id", id),
		zap.String("type", resource.GetType().String()))

	return nil
}

// registerResource 注册资源
func (rm *ResourceManager) registerResource(resource Resource) {
	rm.resourceMux.Lock()
	defer rm.resourceMux.Unlock()

	id := resource.GetID()
	rm.resources[id] = resource

	// 添加到类型索引
	switch res := resource.(type) {
	case *FileResource:
		rm.fileResources[res.GetPath()] = res
	case *ConnectionResource:
		rm.connectionResources[id] = res
	}

	// 更新配额
	rm.updateQuota(resource.GetType(), 1)

	// 更新统计
	rm.stats.totalCreated.Add(1)
	rm.stats.currentCount.Add(1)
	rm.stats.totalSize.Add(resource.GetSize())
}

// removeFromIndexes 从索引中移除
func (rm *ResourceManager) removeFromIndexes(resource Resource) {
	switch res := resource.(type) {
	case *FileResource:
		delete(rm.fileResources, res.GetPath())
	case *ConnectionResource:
		delete(rm.connectionResources, resource.GetID())
	}
}

// checkQuota 检查配额
func (rm *ResourceManager) checkQuota(resType ResourceType) bool {
	rm.quotasMux.RLock()
	quota, exists := rm.quotas[resType]
	rm.quotasMux.RUnlock()

	if !exists {
		return true
	}

	current := quota.Current.Load()
	return current < quota.Max-quota.Reserved
}

// updateQuota 更新配额
func (rm *ResourceManager) updateQuota(resType ResourceType, delta int64) {
	rm.quotasMux.Lock()
	defer rm.quotasMux.Unlock()

	quota, exists := rm.quotas[resType]
	if !exists {
		return
	}

	quota.Current.Add(delta)
	quota.LastUpdated.Store(time.Now().Unix())
}

// initSystemLimits 初始化系统限制
func (rm *ResourceManager) initSystemLimits() {
	var rlimit syscall.Rlimit

	// 获取文件描述符限制
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err == nil {
		maxFD := int(rlimit.Cur * 80 / 100) // 使用80%的限制
		if maxFD > rm.config.MaxFileHandles {
			maxFD = rm.config.MaxFileHandles
		}

		rm.limits = &ResourceLimits{
			MaxFileDescriptors: maxFD,
			MaxConnections:     rm.config.MaxConnections,
			MaxMemoryUsage:     rm.config.MaxMemoryUsage,
			MaxGoroutines:      1000,
		}

		rm.logger.Info("系统资源限制",
			zap.Int("max_file_descriptors", maxFD),
			zap.Uint64("system_fd_limit", rlimit.Cur))
	} else {
		rm.logger.Warn("获取系统限制失败", zap.Error(err))
		rm.limits = &ResourceLimits{
			MaxFileDescriptors: rm.config.MaxFileHandles,
			MaxConnections:     rm.config.MaxConnections,
			MaxMemoryUsage:     rm.config.MaxMemoryUsage,
			MaxGoroutines:      1000,
		}
	}
}

// initResourceQuotas 初始化资源配额
func (rm *ResourceManager) initResourceQuotas() {
	rm.quotasMux.Lock()
	defer rm.quotasMux.Unlock()

	rm.quotas[ResourceTypeFile] = &ResourceQuota{
		Type:     ResourceTypeFile,
		Max:      int64(rm.limits.MaxFileDescriptors),
		Reserved: int64(rm.limits.MaxFileDescriptors / 10), // 保留10%
	}

	rm.quotas[ResourceTypeConnection] = &ResourceQuota{
		Type:     ResourceTypeConnection,
		Max:      int64(rm.limits.MaxConnections),
		Reserved: int64(rm.limits.MaxConnections / 10), // 保留10%
	}

	rm.quotas[ResourceTypeMemory] = &ResourceQuota{
		Type:     ResourceTypeMemory,
		Max:      rm.limits.MaxMemoryUsage,
		Reserved: rm.limits.MaxMemoryUsage / 10, // 保留10%
	}

	rm.quotas[ResourceTypeGoroutine] = &ResourceQuota{
		Type:     ResourceTypeGoroutine,
		Max:      int64(rm.limits.MaxGoroutines),
		Reserved: int64(rm.limits.MaxGoroutines / 10), // 保留10%
	}
}

// cleanupLoop 清理循环
func (rm *ResourceManager) cleanupLoop() {
	defer rm.wg.Done()

	ticker := time.NewTicker(rm.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.performCleanup()
		case <-rm.cleanupCh:
			rm.performCleanup()
		}
	}
}

// performCleanup 执行清理
func (rm *ResourceManager) performCleanup() {
	rm.logger.Debug("开始资源清理")

	var expired []string
	now := time.Now()

	rm.resourceMux.RLock()
	for id, resource := range rm.resources {
		var ttl time.Duration
		switch resource.GetType() {
		case ResourceTypeFile:
			ttl = rm.config.FileTTL
		case ResourceTypeConnection:
			ttl = rm.config.ConnectionTTL
		default:
			ttl = rm.config.DefaultTTL
		}

		if resource.IsExpired(ttl) {
			expired = append(expired, id)
		}
	}
	rm.resourceMux.RUnlock()

	// 释放过期资源
	for _, id := range expired {
		if err := rm.ReleaseResource(id); err != nil {
			rm.logger.Error("清理过期资源失败",
				zap.String("id", id),
				zap.Error(err))
		}
	}

	rm.stats.cleanupRuns.Add(1)

	if len(expired) > 0 {
		rm.logger.Info("资源清理完成",
			zap.Int("cleaned", len(expired)),
			zap.Duration("duration", time.Since(now)))
	}
}

// monitoringLoop 监控循环
func (rm *ResourceManager) monitoringLoop() {
	defer rm.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.logResourceStats()
		}
	}
}

// recycleLoop 资源回收循环
func (rm *ResourceManager) recycleLoop() {
	defer rm.wg.Done()

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.performRecycling()
		}
	}
}

// performRecycling 执行资源回收
func (rm *ResourceManager) performRecycling() {
	// 检查内存使用情况
	totalSize := rm.stats.totalSize.Load()
	maxSize := rm.config.MaxMemoryUsage

	if float64(totalSize) > float64(maxSize)*rm.config.MemoryCleanupThreshold {
		rm.logger.Warn("内存使用过高，触发资源回收",
			zap.Int64("current_mb", totalSize/1024/1024),
			zap.Int64("max_mb", maxSize/1024/1024))

		// 强制清理
		select {
		case rm.cleanupCh <- struct{}{}:
		default:
		}

		rm.stats.recycledCount.Add(1)
	}
}

// logResourceStats 记录资源统计
func (rm *ResourceManager) logResourceStats() {
	stats := rm.GetStats()

	rm.logger.Info("📊 资源使用统计",
		zap.Int64("current_count", stats["current_count"].(int64)),
		zap.Int64("total_created", stats["total_created"].(int64)),
		zap.Int64("total_released", stats["total_released"].(int64)),
		zap.Int64("total_size_mb", stats["total_size"].(int64)/1024/1024),
		zap.Int64("cleanup_runs", stats["cleanup_runs"].(int64)))
}

// releaseAllResources 释放所有资源
func (rm *ResourceManager) releaseAllResources() {
	rm.resourceMux.Lock()
	defer rm.resourceMux.Unlock()

	for id, resource := range rm.resources {
		if err := resource.Release(); err != nil {
			rm.logger.Error("释放资源失败",
				zap.String("id", id),
				zap.Error(err))
		}
	}

	// 清空所有映射
	rm.resources = make(map[string]Resource)
	rm.fileResources = make(map[string]*FileResource)
	rm.connectionResources = make(map[string]*ConnectionResource)

	// 重置统计
	rm.stats.currentCount.Store(0)
	rm.stats.totalSize.Store(0)
}

// GetStats 获取统计信息
func (rm *ResourceManager) GetStats() map[string]interface{} {
	rm.quotasMux.RLock()
	quotaStats := make(map[string]interface{})
	for resType, quota := range rm.quotas {
		quotaStats[resType.String()] = map[string]interface{}{
			"current":  quota.Current.Load(),
			"max":      quota.Max,
			"reserved": quota.Reserved,
			"usage":    float64(quota.Current.Load()) / float64(quota.Max),
		}
	}
	rm.quotasMux.RUnlock()

	return map[string]interface{}{
		"total_created":  rm.stats.totalCreated.Load(),
		"total_released": rm.stats.totalReleased.Load(),
		"current_count":  rm.stats.currentCount.Load(),
		"total_size":     rm.stats.totalSize.Load(),
		"cleanup_runs":   rm.stats.cleanupRuns.Load(),
		"recycled_count": rm.stats.recycledCount.Load(),
		"quotas":         quotaStats,
		"limits":         rm.limits,
		"config":         rm.config,
	}
}

// TriggerCleanup 触发清理
func (rm *ResourceManager) TriggerCleanup() {
	select {
	case rm.cleanupCh <- struct{}{}:
	default:
	}
}
