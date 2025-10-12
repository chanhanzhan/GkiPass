package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// CertificateGenerator 证书生成器
type CertificateGenerator struct {
	logger *zap.Logger
}

// NewCertificateGenerator 创建证书生成器
func NewCertificateGenerator() *CertificateGenerator {
	return &CertificateGenerator{
		logger: zap.L().Named("cert-generator"),
	}
}

// GenerateSelfSignedCert 生成自签名证书
func (g *CertificateGenerator) GenerateSelfSignedCert(commonName string, ips []net.IP, dnsNames []string, validFor time.Duration, certPath, keyPath string) error {
	// 创建私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %w", err)
	}

	// 序列号
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("生成序列号失败: %w", err)
	}

	// 有效期
	notBefore := time.Now()
	notAfter := notBefore.Add(validFor)

	// 证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
		DNSNames:              dnsNames,
	}

	// 创建证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("创建证书失败: %w", err)
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return fmt.Errorf("创建密钥目录失败: %w", err)
	}

	// 写入证书文件
	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("创建证书文件失败: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		certOut.Close()
		return fmt.Errorf("写入证书文件失败: %w", err)
	}
	certOut.Close()

	// 写入密钥文件
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("创建密钥文件失败: %w", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		keyOut.Close()
		return fmt.Errorf("序列化私钥失败: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		keyOut.Close()
		return fmt.Errorf("写入密钥文件失败: %w", err)
	}
	keyOut.Close()

	g.logger.Info("自签名证书生成成功",
		zap.String("common_name", commonName),
		zap.String("cert_path", certPath),
		zap.String("key_path", keyPath),
		zap.Time("not_before", notBefore),
		zap.Time("not_after", notAfter))

	return nil
}
