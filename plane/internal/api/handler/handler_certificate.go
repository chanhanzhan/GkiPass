package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CertificateHandler 证书处理器
type CertificateHandler struct {
	app *types.App
}

// NewCertificateHandler 创建证书处理器
func NewCertificateHandler(app *types.App) *CertificateHandler {
	return &CertificateHandler{app: app}
}

// GenerateCARequest 生成CA证书请求
type GenerateCARequest struct {
	Name        string `json:"name" binding:"required"`
	CommonName  string `json:"common_name" binding:"required"`
	ValidYears  int    `json:"valid_years"`
	Description string `json:"description"`
}

// GenerateCA 生成CA证书
func (h *CertificateHandler) GenerateCA(c *gin.Context) {
	var req GenerateCARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.ValidYears == 0 {
		req.ValidYears = 10 // 默认10年
	}

	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		response.InternalError(c, "Failed to generate private key")
		return
	}

	// 创建证书模板
	notBefore := time.Now()
	notAfter := notBefore.AddDate(req.ValidYears, 0, 0)

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   req.CommonName,
			Organization: []string{"GKIPass"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	// 自签名CA证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		response.InternalError(c, "Failed to create certificate")
		return
	}

	// 编码为PEM格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	// 计算SPKI Pin
	pin := calculateSPKIPin(&privateKey.PublicKey)

	// 保存到数据库
	cert := &dbinit.Certificate{
		ID:          uuid.New().String(),
		Type:        "ca",
		Name:        req.Name,
		CommonName:  req.CommonName,
		PublicKey:   string(certPEM),
		PrivateKey:  string(keyPEM), // 在生产环境应该加密存储
		Pin:         pin,
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		Description: req.Description,
	}

	if err := h.app.DB.DB.SQLite.CreateCertificate(cert); err != nil {
		response.InternalError(c, "Failed to save certificate: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "CA certificate generated successfully", cert)
}

// GenerateLeafRequest 生成叶子证书请求
type GenerateLeafRequest struct {
	Name        string `json:"name" binding:"required"`
	CommonName  string `json:"common_name" binding:"required"`
	ParentID    string `json:"parent_id" binding:"required"`
	ValidDays   int    `json:"valid_days"`
	Description string `json:"description"`
}

// GenerateLeaf 生成叶子证书
func (h *CertificateHandler) GenerateLeaf(c *gin.Context) {
	var req GenerateLeafRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.ValidDays == 0 {
		req.ValidDays = 90 // 默认90天（短周期）
	}

	// 获取父证书(CA)
	parentCert, err := h.app.DB.DB.SQLite.GetCertificate(req.ParentID)
	if err != nil || parentCert == nil {
		response.GinBadRequest(c, "Parent CA certificate not found")
		return
	}

	// 解析父证书
	parentBlock, _ := pem.Decode([]byte(parentCert.PublicKey))
	parentX509, _ := x509.ParseCertificate(parentBlock.Bytes)

	parentKeyBlock, _ := pem.Decode([]byte(parentCert.PrivateKey))
	parentKey, _ := x509.ParsePKCS1PrivateKey(parentKeyBlock.Bytes)

	// 生成新的私钥
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	// 创建证书模板
	notBefore := time.Now()
	notAfter := notBefore.AddDate(0, 0, req.ValidDays)

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   req.CommonName,
			Organization: []string{"GKIPass Node"},
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	// 用父证书签名
	derBytes, _ := x509.CreateCertificate(rand.Reader, &template, parentX509, &privateKey.PublicKey, parentKey)

	// 编码为PEM格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	// 计算SPKI Pin
	pin := calculateSPKIPin(&privateKey.PublicKey)

	// 保存到数据库
	cert := &dbinit.Certificate{
		ID:          uuid.New().String(),
		Type:        "leaf",
		Name:        req.Name,
		CommonName:  req.CommonName,
		PublicKey:   string(certPEM),
		PrivateKey:  string(keyPEM),
		Pin:         pin,
		ParentID:    req.ParentID,
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		Description: req.Description,
	}

	if err := h.app.DB.DB.SQLite.CreateCertificate(cert); err != nil {
		response.InternalError(c, "Failed to save certificate: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "Leaf certificate generated successfully", cert)
}

// List 列出证书
func (h *CertificateHandler) List(c *gin.Context) {
	certType := c.Query("type")
	revokedStr := c.Query("revoked")

	var revoked *bool
	if revokedStr != "" {
		val := revokedStr == "true"
		revoked = &val
	}

	certs, err := h.app.DB.DB.SQLite.ListCertificates(certType, revoked)
	if err != nil {
		response.InternalError(c, "Failed to list certificates: "+err.Error())
		return
	}

	// 不返回私钥
	for _, cert := range certs {
		cert.PrivateKey = "[HIDDEN]"
	}

	response.GinSuccess(c, gin.H{
		"certificates": certs,
		"total":        len(certs),
	})
}

// Get 获取证书详情
func (h *CertificateHandler) Get(c *gin.Context) {
	id := c.Param("id")
	showPrivate := c.Query("show_private") == "true"

	cert, err := h.app.DB.DB.SQLite.GetCertificate(id)
	if err != nil {
		response.InternalError(c, "Failed to get certificate: "+err.Error())
		return
	}

	if cert == nil {
		response.GinNotFound(c, "Certificate not found")
		return
	}

	// 默认隐藏私钥
	if !showPrivate {
		cert.PrivateKey = "[HIDDEN]"
	}

	response.GinSuccess(c, cert)
}

// Revoke 吊销证书
func (h *CertificateHandler) Revoke(c *gin.Context) {
	id := c.Param("id")

	if err := h.app.DB.DB.SQLite.RevokeCertificate(id); err != nil {
		response.InternalError(c, "Failed to revoke certificate: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "Certificate revoked successfully", nil)
}

// Download 下载证书
func (h *CertificateHandler) Download(c *gin.Context) {
	id := c.Param("id")
	includeKey := c.Query("include_key") == "true"

	cert, err := h.app.DB.DB.SQLite.GetCertificate(id)
	if err != nil || cert == nil {
		response.GinNotFound(c, "Certificate not found")
		return
	}

	if includeKey {
		// 下载证书+私钥（打包）
		content := "# Certificate\n" + cert.PublicKey + "\n# Private Key\n" + cert.PrivateKey
		c.Header("Content-Disposition", "attachment; filename="+cert.Name+".pem")
		c.Data(200, "application/x-pem-file", []byte(content))
	} else {
		// 仅下载证书
		c.Header("Content-Disposition", "attachment; filename="+cert.Name+".crt")
		c.Data(200, "application/x-pem-file", []byte(cert.PublicKey))
	}
}

// calculateSPKIPin 计算SPKI Pin (用于证书固定)
func calculateSPKIPin(pubKey *rsa.PublicKey) string {
	spki, _ := x509.MarshalPKIXPublicKey(pubKey)
	hash := sha256.Sum256(spki)
	return base64.StdEncoding.EncodeToString(hash[:])
}
