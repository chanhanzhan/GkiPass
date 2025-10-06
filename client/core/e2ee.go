package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// E2EEManager 端到端加密管理器
type E2EEManager struct {
	algorithm string // aes-gcm, chacha20-poly1305
	key       []byte
}

// NewE2EEManager 创建E2EE管理器
func NewE2EEManager(algorithm string, passphrase string) (*E2EEManager, error) {
	key := sha256.Sum256([]byte(passphrase))
	return &E2EEManager{
		algorithm: algorithm,
		key:       key[:],
	}, nil
}

// Encrypt 加密数据
func (e *E2EEManager) Encrypt(plaintext []byte) ([]byte, error) {
	switch e.algorithm {
	case "aes-gcm":
		return e.encryptAESGCM(plaintext)
	case "chacha20-poly1305":
		return e.encryptChaCha20(plaintext)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", e.algorithm)
	}
}

// Decrypt 解密数据
func (e *E2EEManager) Decrypt(ciphertext []byte) ([]byte, error) {
	switch e.algorithm {
	case "aes-gcm":
		return e.decryptAESGCM(ciphertext)
	case "chacha20-poly1305":
		return e.decryptChaCha20(ciphertext)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", e.algorithm)
	}
}

func (e *E2EEManager) encryptAESGCM(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (e *E2EEManager) decryptAESGCM(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func (e *E2EEManager) encryptChaCha20(plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(e.key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (e *E2EEManager) decryptChaCha20(ciphertext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(e.key)
	if err != nil {
		return nil, err
	}

	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

