package initializer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"


	
)

// InitCertificates 初始化证书
func InitCertificates(certDir string) error {
	// 确保目录存在
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	// 生成 CA 证书
	caKey, caCert, err := generateCA()
	if err != nil {
		return fmt.Errorf("生成CA证书失败: %w", err)
	}

	// 保存 CA 证书
	caKeyPath := filepath.Join(certDir, "ca-key.pem")
	caCertPath := filepath.Join(certDir, "ca-cert.pem")

	if err := savePrivateKey(caKeyPath, caKey); err != nil {
		return err
	}
	if err := saveCertificate(caCertPath, caCert); err != nil {
		return err
	}
	// 生成服务器证书
	serverKey, serverCert, err := generateServerCert(caKey, caCert)
	if err != nil {
		return fmt.Errorf("生成服务器证书失败: %w", err)
	}

	// 保存服务器证书
	serverKeyPath := filepath.Join(certDir, "server-key.pem")
	serverCertPath := filepath.Join(certDir, "server-cert.pem")

	if err := savePrivateKey(serverKeyPath, serverKey); err != nil {
		return err
	}
	if err := saveCertificate(serverCertPath, serverCert); err != nil {
		return err
	}
	return nil
}

// generateCA 生成 CA 证书
func generateCA() (*rsa.PrivateKey, *x509.Certificate, error) {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// 创建证书模板
	notBefore := time.Now()
	notAfter := notBefore.AddDate(10, 0, 0) // 10年有效期

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "GKIPass Root CA",
			Organization: []string{"GKIPass"},
			Country:      []string{"CN"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	// 自签名
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, cert, nil
}

// generateServerCert 生成服务器证书
func generateServerCert(caKey *rsa.PrivateKey, caCert *x509.Certificate) (*rsa.PrivateKey, *x509.Certificate, error) {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// 创建证书模板
	notBefore := time.Now()
	notAfter := notBefore.AddDate(0, 0, 90) // 90天有效期（短周期）

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "GKIPass Server",
			Organization: []string{"GKIPass"},
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    []string{"localhost", "*.gkipass.local"},
	}

	// 用 CA 签名
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, cert, nil
}

// savePrivateKey 保存私钥
func savePrivateKey(filename string, key *rsa.PrivateKey) error {
	keyFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer keyFile.Close()

	keyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return pem.Encode(keyFile, keyPEM)
}

// saveCertificate 保存证书
func saveCertificate(filename string, cert *x509.Certificate) error {
	certFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer certFile.Close()

	certPEM := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}

	return pem.Encode(certFile, certPEM)
}

