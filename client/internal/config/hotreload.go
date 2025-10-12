package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// ReloadEvent 重载事件类型
type ReloadEvent int

const (
	ReloadEventConfigChanged ReloadEvent = iota
	ReloadEventReloadStarted
	ReloadEventReloadCompleted
	ReloadEventReloadFailed
)

// String 返回重载事件名称
func (re ReloadEvent) String() string {
	switch re {
	case ReloadEventConfigChanged:
		return "CONFIG_CHANGED"
	case ReloadEventReloadStarted:
		return "RELOAD_STARTED"
	case ReloadEventReloadCompleted:
		return "RELOAD_COMPLETED"
	case ReloadEventReloadFailed:
		return "RELOAD_FAILED"
	default:
		return "UNKNOWN"
	}
}

// ReloadNotification 重载通知
type ReloadNotification struct {
	Event      ReloadEvent `json:"event"`
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
	ConfigPath string      `json:"config_path,omitempty"`
}

// ReloadCallback 重载回调函数
type ReloadCallback func(oldConfig, newConfig *Config) error

// LocalHotReloadConfig 本地热重载配置
type LocalHotReloadConfig struct {
	Enabled        bool          `json:"enabled"`
	WatchInterval  time.Duration `json:"watch_interval"`
	ReloadDelay    time.Duration `json:"reload_delay"`    // 延迟重载时间
	BackupEnabled  bool          `json:"backup_enabled"`  // 是否备份配置
	BackupDir      string        `json:"backup_dir"`      // 备份目录
	ValidateConfig bool          `json:"validate_config"` // 是否验证配置
}

// DefaultHotReloadConfig 默认热重载配置
func DefaultHotReloadConfig() *LocalHotReloadConfig {
	return &LocalHotReloadConfig{
		Enabled:        true,
		WatchInterval:  1 * time.Second,
		ReloadDelay:    3 * time.Second,
		BackupEnabled:  true,
		BackupDir:      "./backups",
		ValidateConfig: true,
	}
}

// HotReloader 热重载器
type HotReloader struct {
	config        *LocalHotReloadConfig
	configPath    string
	currentConfig *Config

	// 文件监控
	watcher *fsnotify.Watcher

	// 回调函数
	callbacks     []ReloadCallback
	callbackMutex sync.RWMutex

	// 通知
	notifications chan ReloadNotification

	// 状态
	reloading   atomic.Bool
	reloadCount atomic.Int64
	lastReload  atomic.Int64 // Unix timestamp

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewHotReloader 创建热重载器
func NewHotReloader(configPath string, initialConfig *Config, hotConfig *HotReloadConfig) (*HotReloader, error) {
	var localConfig *LocalHotReloadConfig

	if hotConfig == nil {
		localConfig = DefaultHotReloadConfig()
	} else {
		// 转换为本地配置
		localConfig = &LocalHotReloadConfig{
			Enabled:        hotConfig.Enabled,
			WatchInterval:  hotConfig.WatchInterval,
			ReloadDelay:    hotConfig.ReloadDelay,
			BackupEnabled:  hotConfig.BackupEnabled,
			BackupDir:      hotConfig.BackupDir,
			ValidateConfig: hotConfig.ValidateConfig,
		}
	}

	if !localConfig.Enabled {
		return nil, fmt.Errorf("热重载未启用")
	}

	ctx, cancel := context.WithCancel(context.Background())

	hr := &HotReloader{
		config:        localConfig,
		configPath:    configPath,
		currentConfig: initialConfig,
		callbacks:     make([]ReloadCallback, 0),
		notifications: make(chan ReloadNotification, 100),
		ctx:           ctx,
		cancel:        cancel,
		logger:        zap.L().Named("hot-reloader"),
	}

	// 创建文件监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建文件监控器失败: %w", err)
	}
	hr.watcher = watcher

	return hr, nil
}

// Start 启动热重载器
func (hr *HotReloader) Start() error {
	if !hr.config.Enabled {
		return fmt.Errorf("热重载未启用")
	}

	hr.logger.Info("启动热重载器",
		zap.String("config_path", hr.configPath),
		zap.Duration("watch_interval", hr.config.WatchInterval),
		zap.Duration("reload_delay", hr.config.ReloadDelay))

	// 创建备份目录
	if hr.config.BackupEnabled {
		if err := os.MkdirAll(hr.config.BackupDir, 0755); err != nil {
			return fmt.Errorf("创建备份目录失败: %w", err)
		}
	}

	// 添加文件监控
	configDir := filepath.Dir(hr.configPath)
	if err := hr.watcher.Add(configDir); err != nil {
		return fmt.Errorf("添加文件监控失败: %w", err)
	}

	// 启动监控协程
	hr.wg.Add(1)
	go func() {
		defer hr.wg.Done()
		hr.watchLoop()
	}()

	return nil
}

// Stop 停止热重载器
func (hr *HotReloader) Stop() error {
	hr.logger.Info("停止热重载器")

	// 取消上下文
	if hr.cancel != nil {
		hr.cancel()
	}

	// 关闭文件监控器
	if hr.watcher != nil {
		hr.watcher.Close()
	}

	// 等待协程结束
	hr.wg.Wait()

	// 关闭通知通道
	close(hr.notifications)

	hr.logger.Info("热重载器已停止")
	return nil
}

// AddCallback 添加重载回调函数
func (hr *HotReloader) AddCallback(callback ReloadCallback) {
	hr.callbackMutex.Lock()
	defer hr.callbackMutex.Unlock()

	hr.callbacks = append(hr.callbacks, callback)
	hr.logger.Debug("添加重载回调函数", zap.Int("total_callbacks", len(hr.callbacks)))
}

// GetNotifications 获取通知通道
func (hr *HotReloader) GetNotifications() <-chan ReloadNotification {
	return hr.notifications
}

// TriggerReload 手动触发重载
func (hr *HotReloader) TriggerReload() error {
	if hr.reloading.Load() {
		return fmt.Errorf("重载正在进行中")
	}

	hr.logger.Info("手动触发配置重载")
	return hr.performReload()
}

// watchLoop 监控循环
func (hr *HotReloader) watchLoop() {
	hr.logger.Debug("启动配置文件监控循环")

	reloadTimer := time.NewTimer(0)
	reloadTimer.Stop()

	for {
		select {
		case <-hr.ctx.Done():
			hr.logger.Debug("停止配置文件监控循环")
			reloadTimer.Stop()
			return

		case event, ok := <-hr.watcher.Events:
			if !ok {
				return
			}

			// 检查是否是配置文件变化
			if hr.isConfigFileEvent(event) {
				hr.logger.Debug("检测到配置文件变化",
					zap.String("file", event.Name),
					zap.String("op", event.Op.String()))

				// 发送变更通知
				hr.sendNotification(ReloadEventConfigChanged, "检测到配置文件变化", "", event.Name)

				// 重置延迟定时器
				reloadTimer.Stop()
				reloadTimer.Reset(hr.config.ReloadDelay)
			}

		case err, ok := <-hr.watcher.Errors:
			if !ok {
				return
			}
			hr.logger.Error("文件监控错误", zap.Error(err))

		case <-reloadTimer.C:
			// 延迟时间到，执行重载
			if err := hr.performReload(); err != nil {
				hr.logger.Error("配置重载失败", zap.Error(err))
			}
		}
	}
}

// isConfigFileEvent 检查是否是配置文件事件
func (hr *HotReloader) isConfigFileEvent(event fsnotify.Event) bool {
	// 检查文件名是否匹配
	eventFile := filepath.Base(event.Name)
	configFile := filepath.Base(hr.configPath)

	if eventFile != configFile {
		return false
	}

	// 检查事件类型
	return event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Create == fsnotify.Create
}

// performReload 执行重载
func (hr *HotReloader) performReload() error {
	if !hr.reloading.CompareAndSwap(false, true) {
		return fmt.Errorf("重载正在进行中")
	}
	defer hr.reloading.Store(false)

	hr.logger.Info("开始配置重载")
	hr.sendNotification(ReloadEventReloadStarted, "开始配置重载", "", "")

	// 备份当前配置
	if hr.config.BackupEnabled {
		if err := hr.backupConfig(); err != nil {
			hr.logger.Error("备份配置失败", zap.Error(err))
		}
	}

	// 加载新配置
	newConfig, err := hr.loadNewConfig()
	if err != nil {
		hr.sendNotification(ReloadEventReloadFailed, "加载新配置失败", err.Error(), "")
		return fmt.Errorf("加载新配置失败: %w", err)
	}

	// 验证新配置
	if hr.config.ValidateConfig {
		if err := hr.validateConfig(newConfig); err != nil {
			hr.sendNotification(ReloadEventReloadFailed, "配置验证失败", err.Error(), "")
			return fmt.Errorf("配置验证失败: %w", err)
		}
	}

	// 执行回调函数
	oldConfig := hr.currentConfig
	if err := hr.executeCallbacks(oldConfig, newConfig); err != nil {
		hr.sendNotification(ReloadEventReloadFailed, "执行回调函数失败", err.Error(), "")
		return fmt.Errorf("执行回调函数失败: %w", err)
	}

	// 更新当前配置
	hr.currentConfig = newConfig
	hr.reloadCount.Add(1)
	hr.lastReload.Store(time.Now().Unix())

	hr.logger.Info("配置重载完成",
		zap.Int64("reload_count", hr.reloadCount.Load()))
	hr.sendNotification(ReloadEventReloadCompleted, "配置重载完成", "", "")

	return nil
}

// loadNewConfig 加载新配置
func (hr *HotReloader) loadNewConfig() (*Config, error) {
	hr.logger.Debug("加载新配置文件", zap.String("path", hr.configPath))

	// 读取配置文件
	data, err := os.ReadFile(hr.configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析配置
	var newConfig Config
	if err := json.Unmarshal(data, &newConfig); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &newConfig, nil
}

// validateConfig 验证配置
func (hr *HotReloader) validateConfig(config *Config) error {
	hr.logger.Debug("验证新配置")

	// 基本验证
	if config.NodeID == "" {
		return fmt.Errorf("节点ID不能为空")
	}

	if len(config.PlaneURLs) == 0 {
		return fmt.Errorf("Plane地址不能为空")
	}

	// 可以添加更多的验证逻辑

	return nil
}

// executeCallbacks 执行回调函数
func (hr *HotReloader) executeCallbacks(oldConfig, newConfig *Config) error {
	hr.callbackMutex.RLock()
	callbacks := make([]ReloadCallback, len(hr.callbacks))
	copy(callbacks, hr.callbacks)
	hr.callbackMutex.RUnlock()

	hr.logger.Debug("执行重载回调函数", zap.Int("callback_count", len(callbacks)))

	for i, callback := range callbacks {
		if err := callback(oldConfig, newConfig); err != nil {
			return fmt.Errorf("回调函数 %d 执行失败: %w", i, err)
		}
	}

	return nil
}

// backupConfig 备份配置
func (hr *HotReloader) backupConfig() error {
	timestamp := time.Now().Format("20060102_150405")
	backupFileName := fmt.Sprintf("config_%s.json", timestamp)
	backupPath := filepath.Join(hr.config.BackupDir, backupFileName)

	hr.logger.Debug("备份当前配置", zap.String("backup_path", backupPath))

	// 读取当前配置文件
	data, err := os.ReadFile(hr.configPath)
	if err != nil {
		return fmt.Errorf("读取当前配置失败: %w", err)
	}

	// 写入备份文件
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}

	// 清理旧备份（保留最近10个）
	hr.cleanupOldBackups()

	return nil
}

// cleanupOldBackups 清理旧备份
func (hr *HotReloader) cleanupOldBackups() {
	files, err := filepath.Glob(filepath.Join(hr.config.BackupDir, "config_*.json"))
	if err != nil {
		hr.logger.Error("查找备份文件失败", zap.Error(err))
		return
	}

	if len(files) <= 10 {
		return
	}

	// 按修改时间排序，删除最旧的文件
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var fileInfos []fileInfo
	for _, file := range files {
		if stat, err := os.Stat(file); err == nil {
			fileInfos = append(fileInfos, fileInfo{
				path:    file,
				modTime: stat.ModTime(),
			})
		}
	}

	// 简单排序（按修改时间）
	for i := 0; i < len(fileInfos)-1; i++ {
		for j := i + 1; j < len(fileInfos); j++ {
			if fileInfos[i].modTime.After(fileInfos[j].modTime) {
				fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
			}
		}
	}

	// 删除多余的文件
	for i := 0; i < len(fileInfos)-10; i++ {
		if err := os.Remove(fileInfos[i].path); err != nil {
			hr.logger.Error("删除旧备份文件失败",
				zap.String("file", fileInfos[i].path),
				zap.Error(err))
		} else {
			hr.logger.Debug("删除旧备份文件", zap.String("file", fileInfos[i].path))
		}
	}
}

// sendNotification 发送通知
func (hr *HotReloader) sendNotification(event ReloadEvent, message, errorMsg, configPath string) {
	notification := ReloadNotification{
		Event:      event,
		Message:    message,
		Error:      errorMsg,
		Timestamp:  time.Now(),
		ConfigPath: configPath,
	}

	select {
	case hr.notifications <- notification:
	default:
		hr.logger.Warn("通知通道已满，丢弃通知",
			zap.String("event", event.String()))
	}
}

// GetCurrentConfig 获取当前配置
func (hr *HotReloader) GetCurrentConfig() *Config {
	return hr.currentConfig
}

// GetStats 获取热重载器统计信息
func (hr *HotReloader) GetStats() map[string]interface{} {
	hr.callbackMutex.RLock()
	callbackCount := len(hr.callbacks)
	hr.callbackMutex.RUnlock()

	return map[string]interface{}{
		"enabled":        hr.config.Enabled,
		"config_path":    hr.configPath,
		"reload_count":   hr.reloadCount.Load(),
		"last_reload":    time.Unix(hr.lastReload.Load(), 0),
		"reloading":      hr.reloading.Load(),
		"callback_count": callbackCount,
		"config": map[string]interface{}{
			"watch_interval":  hr.config.WatchInterval.String(),
			"reload_delay":    hr.config.ReloadDelay.String(),
			"backup_enabled":  hr.config.BackupEnabled,
			"backup_dir":      hr.config.BackupDir,
			"validate_config": hr.config.ValidateConfig,
		},
	}
}
