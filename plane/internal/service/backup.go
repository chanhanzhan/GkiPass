package service

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// BackupService SQLite备份服务
type BackupService struct {
	dbPath     string
	backupDir  string
	maxBackups int
	interval   time.Duration
	stopChan   chan struct{}
}

// NewBackupService 创建备份服务
func NewBackupService(dbPath, backupDir string, maxBackups int, interval time.Duration) *BackupService {
	return &BackupService{
		dbPath:     dbPath,
		backupDir:  backupDir,
		maxBackups: maxBackups,
		interval:   interval,
		stopChan:   make(chan struct{}),
	}
}

// Start 启动备份服务
func (bs *BackupService) Start() {
	go bs.backupLoop()
	logger.Info("数据库备份服务已启动",
		zap.String("backup_dir", bs.backupDir),
		zap.Duration("interval", bs.interval))
}

// Stop 停止备份服务
func (bs *BackupService) Stop() {
	close(bs.stopChan)
	logger.Info("数据库备份服务已停止")
}

// backupLoop 备份循环
func (bs *BackupService) backupLoop() {
	ticker := time.NewTicker(bs.interval)
	defer ticker.Stop()

	// 启动时立即执行一次备份
	bs.Backup()

	for {
		select {
		case <-ticker.C:
			bs.Backup()
		case <-bs.stopChan:
			return
		}
	}
}

// Backup 执行备份
func (bs *BackupService) Backup() error {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(bs.backupDir, fmt.Sprintf("backup_%s.db", timestamp))

	// 确保备份目录存在
	if err := os.MkdirAll(bs.backupDir, 0755); err != nil {
		logger.Error("创建备份目录失败", zap.Error(err))
		return err
	}

	// 复制数据库文件
	if err := bs.copyFile(bs.dbPath, backupPath); err != nil {
		logger.Error("备份数据库失败", zap.Error(err))
		return err
	}

	logger.Info("数据库备份成功", zap.String("backup_path", backupPath))

	// 清理旧备份
	bs.cleanupOldBackups()

	return nil
}

// copyFile 复制文件
func (bs *BackupService) copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// cleanupOldBackups 清理旧备份
func (bs *BackupService) cleanupOldBackups() {
	files, err := filepath.Glob(filepath.Join(bs.backupDir, "backup_*.db"))
	if err != nil {
		return
	}

	if len(files) > bs.maxBackups {
		// 删除最旧的备份
		for i := 0; i < len(files)-bs.maxBackups; i++ {
			os.Remove(files[i])
			logger.Info("删除旧备份", zap.String("file", files[i]))
		}
	}
}

// Restore 恢复备份
func (bs *BackupService) Restore(backupPath string) error {
	if err := bs.copyFile(backupPath, bs.dbPath); err != nil {
		logger.Error("恢复数据库失败", zap.Error(err))
		return err
	}

	logger.Info("数据库恢复成功", zap.String("backup_path", backupPath))
	return nil
}

