package tls

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/ioutil"
)

// PinVerifier 证书固定验证器
type PinVerifier struct {
	pins map[string]bool
}

// NewPinVerifier 创建证书固定验证器
func NewPinVerifier(pins []string) *PinVerifier {
	pinMap := make(map[string]bool)
	for _, pin := range pins {
		pinMap[pin] = true
	}
	return &PinVerifier{
		pins: pinMap,
	}
}

// AddPin 添加证书固定
func (v *PinVerifier) AddPin(pin string) {
	v.pins[pin] = true
}

// VerifyCertificate 验证证书
func (v *PinVerifier) VerifyCertificate(cert *x509.Certificate) (bool, string, error) {
	// 计算SPKI指纹
	spkiFingerprint := calculateSPKIFingerprint(cert)

	// 检查是否匹配
	if v.pins[spkiFingerprint] {
		return true, spkiFingerprint, nil
	}

	return false, spkiFingerprint, nil
}

// VerifyCertificateFromFile 从文件验证证书
func (v *PinVerifier) VerifyCertificateFromFile(certFile string) (bool, string, error) {
	// 读取证书文件
	certData, err := ioutil.ReadFile(certFile)
	if err != nil {
		return false, "", fmt.Errorf("读取证书文件失败: %w", err)
	}

	// 解码PEM
	block, _ := pem.Decode(certData)
	if block == nil {
		return false, "", fmt.Errorf("解析PEM证书失败")
	}

	// 解析证书
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, "", fmt.Errorf("解析X509证书失败: %w", err)
	}

	return v.VerifyCertificate(cert)
}

// calculateSPKIFingerprint 计算SPKI指纹
func calculateSPKIFingerprint(cert *x509.Certificate) string {
	// 获取SPKI
	spki := cert.RawSubjectPublicKeyInfo

	// 计算SHA-256哈希
	hash := sha256.Sum256(spki)

	// 转换为Base64
	return base64.StdEncoding.EncodeToString(hash[:])
}
