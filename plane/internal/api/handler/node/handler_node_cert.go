package node

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NodeCertHandler 节点证书处理器
type NodeCertHandler struct {
	app *types.App
}

// NewNodeCertHandler 创建节点证书处理器
func NewNodeCertHandler(app *types.App) *NodeCertHandler {
	return &NodeCertHandler{app: app}
}

// GenerateCert 生成节点证书
func (h *NodeCertHandler) GenerateCert(c *gin.Context) {
	nodeID := c.Param("id")

	// 获取节点信息
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 初始化证书管理器
	certManager, err := service.NewNodeCertManager(h.app.DB, "./certs")
	if err != nil {
		logger.Error("初始化证书管理器失败", zap.Error(err))
		response.InternalError(c, "Failed to initialize cert manager")
		return
	}

	// 生成证书
	cert, err := certManager.GenerateNodeCert(node.ID, node.Name)
	if err != nil {
		logger.Error("生成节点证书失败", zap.Error(err))
		response.InternalError(c, "Failed to generate certificate: "+err.Error())
		return
	}

	// 更新节点记录
	node.CertID = cert.ID
	if err := h.app.DB.DB.SQLite.UpdateNode(node); err != nil {
		logger.Warn("更新节点证书ID失败", zap.Error(err))
	}

	response.SuccessWithMessage(c, "Certificate generated successfully", gin.H{
		"cert_id":     cert.ID,
		"expires_at":  cert.NotAfter,
		"common_name": cert.CommonName,
	})
}

// DownloadCert 下载节点证书
func (h *NodeCertHandler) DownloadCert(c *gin.Context) {
	nodeID := c.Param("id")

	// 获取节点信息
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 检查证书是否存在
	if node.CertID == "" {
		response.BadRequest(c, "Node certificate not generated")
		return
	}

	// 初始化证书管理器
	certManager, err := service.NewNodeCertManager(h.app.DB, "./certs")
	if err != nil {
		response.InternalError(c, "Failed to initialize cert manager")
		return
	}

	// 获取证书路径
	certPath, keyPath, caPath := certManager.GetNodeCertPath(node.ID)

	// 检查文件是否存在
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		response.NotFound(c, "Certificate files not found")
		return
	}

	// 创建 ZIP 文件
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// 添加证书文件
	files := map[string]string{
		"cert.pem":    certPath,
		"key.pem":     keyPath,
		"ca-cert.pem": caPath,
	}

	for name, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Error("读取证书文件失败", zap.String("path", path), zap.Error(err))
			continue
		}

		f, err := zipWriter.Create(name)
		if err != nil {
			logger.Error("创建ZIP文件失败", zap.Error(err))
			continue
		}

		if _, err := f.Write(data); err != nil {
			logger.Error("写入ZIP文件失败", zap.Error(err))
			continue
		}
	}

	// 获取证书信息
	nodeCert, _ := h.app.DB.DB.SQLite.GetCertificate(node.CertID)
	genTime := "Unknown"
	if nodeCert != nil {
		genTime = nodeCert.NotBefore.Format("2006-01-02 15:04:05")
	}

	// 添加 README
	readme := fmt.Sprintf(`GKI Pass 节点证书包

节点ID: %s
节点名称: %s
生成时间: %s

文件说明:
- cert.pem: 节点证书
- key.pem: 节点私钥
- ca-cert.pem: CA 根证书

使用方法:
1. 将证书文件放置到节点的 certs/ 目录
2. 启动节点: ./client --token <your-connection-key> --cert-dir ./certs

注意事项:
- 请妥善保管私钥文件（key.pem）
- 证书有效期: 1年
- 证书到期前请及时续期
`, node.ID, node.Name, genTime)

	f, _ := zipWriter.Create("README.txt")
	f.Write([]byte(readme))

	if err := zipWriter.Close(); err != nil {
		response.InternalError(c, "Failed to create ZIP file")
		return
	}

	// 发送 ZIP 文件
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=node-%s-certs.zip", node.ID))
	c.Data(200, "application/zip", buf.Bytes())

	logger.Info("节点证书已下载", zap.String("nodeID", node.ID))
}

// RenewCert 续期节点证书
func (h *NodeCertHandler) RenewCert(c *gin.Context) {
	nodeID := c.Param("id")

	// 获取节点信息
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 初始化证书管理器
	certManager, err := service.NewNodeCertManager(h.app.DB, "./certs")
	if err != nil {
		response.InternalError(c, "Failed to initialize cert manager")
		return
	}

	// 续期证书
	newCert, err := certManager.RenewNodeCert(node.ID, node.Name, node.CertID)
	if err != nil {
		logger.Error("续期节点证书失败", zap.Error(err))
		response.InternalError(c, "Failed to renew certificate: "+err.Error())
		return
	}

	// 更新节点记录
	node.CertID = newCert.ID
	if err := h.app.DB.DB.SQLite.UpdateNode(node); err != nil {
		logger.Warn("更新节点证书ID失败", zap.Error(err))
	}

	response.SuccessWithMessage(c, "Certificate renewed successfully", gin.H{
		"cert_id":     newCert.ID,
		"expires_at":  newCert.NotAfter,
		"common_name": newCert.CommonName,
	})
}

// GetCertInfo 获取节点证书信息
func (h *NodeCertHandler) GetCertInfo(c *gin.Context) {
	nodeID := c.Param("id")

	// 获取节点信息
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 检查证书是否存在
	if node.CertID == "" {
		response.Success(c, gin.H{
			"has_cert": false,
			"message":  "Certificate not generated",
		})
		return
	}

	// 获取证书信息
	cert, err := h.app.DB.DB.SQLite.GetCertificate(node.CertID)
	if err != nil || cert == nil {
		response.NotFound(c, "Certificate not found")
		return
	}

	// 初始化证书管理器
	certManager, err := service.NewNodeCertManager(h.app.DB, "./certs")
	if err != nil {
		response.InternalError(c, "Failed to initialize cert manager")
		return
	}

	// 检查证书文件是否存在
	certPath, _, _ := certManager.GetNodeCertPath(node.ID)
	fileExists := false
	if _, err := os.Stat(certPath); err == nil {
		fileExists = true
	}

	response.Success(c, gin.H{
		"has_cert":     true,
		"file_exists":  fileExists,
		"cert_id":      cert.ID,
		"common_name":  cert.CommonName,
		"not_before":   cert.NotBefore,
		"not_after":    cert.NotAfter,
		"revoked":      cert.Revoked,
		"expires_soon": certManager.CheckCertExpiry(cert),
		"cert_path":    filepath.Join("./certs/nodes", node.ID),
	})
}
