package tls

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
)

// PinVerifier Pin校验器
type PinVerifier struct {
	pins map[string]bool // SHA256 hash → true
}

// NewPinVerifier 创建Pin校验器
func NewPinVerifier(pins []string) *PinVerifier {
	pv := &PinVerifier{
		pins: make(map[string]bool),
	}

	for _, pin := range pins {
		pv.pins[pin] = true
	}

	return pv
}

// Verify 校验证书Pin
func (pv *PinVerifier) Verify(certs []*x509.Certificate) error {
	if len(pv.pins) == 0 {
		// 没有配置Pin，跳过校验
		return nil
	}

	for _, cert := range certs {
		pin := CalculatePin(cert)
		if pv.pins[pin] {
			// 找到匹配的Pin
			return nil
		}
	}

	return fmt.Errorf("证书Pin校验失败")
}

// AddPin 添加Pin
func (pv *PinVerifier) AddPin(pin string) {
	pv.pins[pin] = true
}

// RemovePin 移除Pin
func (pv *PinVerifier) RemovePin(pin string) {
	delete(pv.pins, pin)
}

// HasPin 检查是否包含Pin
func (pv *PinVerifier) HasPin(pin string) bool {
	return pv.pins[pin]
}

// CalculatePin 计算证书Pin (SPKI SHA256)
func CalculatePin(cert *x509.Certificate) string {
	// 计算Subject Public Key Info的SHA256哈希
	hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	// Base64编码
	return base64.StdEncoding.EncodeToString(hash[:])
}






