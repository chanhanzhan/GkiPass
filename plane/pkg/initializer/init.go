package initializer

import (
	"fmt"
	"os"
	"path/filepath"

	"gkipass/plane/internal/config"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// IsFirstRun 检查是否首次运行
func IsFirstRun(configPath string) bool {
	_, err := os.Stat(configPath)
	return os.IsNotExist(err)
}

// InitConfig 初始化配置文件
func InitConfig(configPath string) error {
	//	logger.Info("首次启动，初始化配置文件...", zap.String("path", configPath))

	// 确保目录存在
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 生成默认配置
	cfg := config.DefaultConfig()

	// 生成随机 JWT Secret
	cfg.Auth.JWTSecret = generateRandomSecret()

	// 保存配置文件
	if err := config.SaveConfig(cfg, configPath); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	logger.Info("✓ 配置文件已生成", zap.String("path", configPath))
	return nil
}

// InitDirectories 初始化必要的目录
func InitDirectories() error {
	dirs := []string{
		"./data",
		"./logs",
		"./certs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
		//	logger.Info("✓ 目录已创建", zap.String("path", dir))
	}

	return nil
}

// generateRandomSecret 生成随机密钥
func generateRandomSecret() string {
	return fmt.Sprintf("gkipass-secret-%d", os.Getpid())
}

// PrintWelcome 打印欢迎信息
func PrintWelcome() {
	welcome := `
╔═══════════════════════════════════════════════════════╗
║                                                       ║
║   ██████╗ ██╗  ██╗██╗    ██████╗  █████╗ ███████╗███╗
║  ██╔════╝ ██║ ██╔╝██║    ██╔══██╗██╔══██╗██╔════╝████║
║  ██║  ███╗█████╔╝ ██║    ██████╔╝███████║███████╗╚═██║
║  ██║   ██║██╔═██╗ ██║    ██╔═══╝ ██╔══██║╚════██║  ██║
║  ╚██████╔╝██║  ██╗██║    ██║     ██║  ██║███████║  ██║
║   ╚═════╝ ╚═╝  ╚═╝╚═╝    ╚═╝     ╚═╝  ╚═╝╚══════╝  ╚═╝
║                                                       ║
║           Bidirectional Tunnel Control Plane         ║
║                      v2.0.0                           ║
║                                                       ║
╚═══════════════════════════════════════════════════════╝
`
	fmt.Println(welcome)
}



