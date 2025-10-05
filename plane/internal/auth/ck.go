package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// CKManager Connection Key 管理器
type CKManager struct {
}

// NewCKManager 创建 CK 管理器
func NewCKManager() *CKManager {
	return &CKManager{}
}

func GenerateCK() string {
	bytes := make([]byte, 32) // 32字节 = 64位hex
	if _, err := rand.Read(bytes); err != nil {
		logger.Error("生成CK失败", zap.Error(err))
		return ""
	}

	return "gkp_" + hex.EncodeToString(bytes)
}

// ValidateCK 验证 CK 格式
func ValidateCK(ck string) error {
	if len(ck) != 68 { // gkp_ + 64位hex
		return fmt.Errorf("CK 长度错误")
	}

	if ck[:4] != "gkp_" {
		return fmt.Errorf("CK 前缀错误")
	}

	// 验证是否为有效的hex
	_, err := hex.DecodeString(ck[4:])
	if err != nil {
		return fmt.Errorf("CK 格式错误: %w", err)
	}

	return nil
}

// CreateNodeCK 为节点创建 CK
func CreateNodeCK(nodeID string, expiresIn time.Duration) *dbinit.ConnectionKey {
	ck := GenerateCK()

	return &dbinit.ConnectionKey{
		ID:        GenerateID(),
		Key:       ck,
		NodeID:    nodeID,
		Type:      "node",
		ExpiresAt: time.Now().Add(expiresIn),
		CreatedAt: time.Now(),
	}
}

// CreateUserCK 为用户创建 CK
func CreateUserCK(userID string, expiresIn time.Duration) *dbinit.ConnectionKey {
	ck := GenerateCK()

	return &dbinit.ConnectionKey{
		ID:        GenerateID(),
		Key:       ck,
		NodeID:    userID, // 对于用户，复用 NodeID 字段存储 UserID
		Type:      "user",
		ExpiresAt: time.Now().Add(expiresIn),
		CreatedAt: time.Now(),
	}
}

// GenerateID 生成唯一ID
func GenerateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// IsExpired 检查 CK 是否过期
func IsExpired(ck *dbinit.ConnectionKey) bool {
	return time.Now().After(ck.ExpiresAt)
}
