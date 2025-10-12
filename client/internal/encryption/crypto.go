package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
)

// DataEncryptor 数据加密器接口
type DataEncryptor interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
}

// AESEncryptor AES加密器
type AESEncryptor struct {
	key []byte
}

// NewAESEncryptor 创建AES加密器
// key长度必须是16, 24或32字节, 对应AES-128, AES-192或AES-256
func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
	keyLen := len(key)
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		return nil, fmt.Errorf("AES key must be 16, 24, or 32 bytes, got %d", keyLen)
	}

	return &AESEncryptor{key: key}, nil
}

// Encrypt AES加密
func (e *AESEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 创建随机nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// 加密
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt AES解密
func (e *AESEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	// 提取nonce
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	// 解密
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// RSAEncryptor RSA加密器
type RSAEncryptor struct {
	publicKey  *rsa.PublicKey
	privateKey *rsa.PrivateKey
}

// NewRSAEncryptorFromKeys 从密钥对创建RSA加密器
func NewRSAEncryptorFromKeys(publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) *RSAEncryptor {
	return &RSAEncryptor{
		publicKey:  publicKey,
		privateKey: privateKey,
	}
}

// NewRSAEncryptorFromFiles 从密钥文件创建RSA加密器
func NewRSAEncryptorFromFiles(publicKeyFile, privateKeyFile string) (*RSAEncryptor, error) {
	publicKey, err := loadPublicKeyFromFile(publicKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load public key: %w", err)
	}

	privateKey, err := loadPrivateKeyFromFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load private key: %w", err)
	}

	return &RSAEncryptor{
		publicKey:  publicKey,
		privateKey: privateKey,
	}, nil
}

// NewRSAEncryptorFromPEM 从PEM字符串创建RSA加密器
func NewRSAEncryptorFromPEM(publicKeyPEM, privateKeyPEM string) (*RSAEncryptor, error) {
	publicKey, err := parsePublicKeyPEM([]byte(publicKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	privateKey, err := parsePrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return &RSAEncryptor{
		publicKey:  publicKey,
		privateKey: privateKey,
	}, nil
}

// Encrypt RSA加密
// 注意：RSA加密的数据大小有限制，通常不超过密钥大小减去一些填充开销
// 对于2048位密钥，最大大约是245字节
func (e *RSAEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if e.publicKey == nil {
		return nil, errors.New("public key is not set")
	}

	// 使用OAEP填充
	label := []byte("")
	hash := sha256.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, e.publicKey, plaintext, label)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

// Decrypt RSA解密
func (e *RSAEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if e.privateKey == nil {
		return nil, errors.New("private key is not set")
	}

	// 使用OAEP填充
	label := []byte("")
	hash := sha256.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, e.privateKey, ciphertext, label)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// HybridEncryptor 混合加密器 (RSA+AES)
type HybridEncryptor struct {
	rsaEncryptor *RSAEncryptor
}

// NewHybridEncryptor 创建混合加密器
func NewHybridEncryptor(rsaEncryptor *RSAEncryptor) *HybridEncryptor {
	return &HybridEncryptor{
		rsaEncryptor: rsaEncryptor,
	}
}

// Encrypt 混合加密
// 使用AES加密数据，然后使用RSA加密AES密钥
func (e *HybridEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	// 生成随机AES密钥
	aesKey := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(rand.Reader, aesKey); err != nil {
		return nil, err
	}

	// 使用AES加密数据
	aesEncryptor, err := NewAESEncryptor(aesKey)
	if err != nil {
		return nil, err
	}

	ciphertext, err := aesEncryptor.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}

	// 使用RSA加密AES密钥
	encryptedKey, err := e.rsaEncryptor.Encrypt(aesKey)
	if err != nil {
		return nil, err
	}

	// 组合加密的密钥和数据
	// 格式: [encryptedKeyLength(4字节)][encryptedKey][ciphertext]
	result := make([]byte, 4+len(encryptedKey)+len(ciphertext))

	// 写入加密密钥的长度
	keyLength := uint32(len(encryptedKey))
	result[0] = byte(keyLength >> 24)
	result[1] = byte(keyLength >> 16)
	result[2] = byte(keyLength >> 8)
	result[3] = byte(keyLength)

	// 写入加密的密钥和数据
	copy(result[4:], encryptedKey)
	copy(result[4+len(encryptedKey):], ciphertext)

	return result, nil
}

// Decrypt 混合解密
// 使用RSA解密AES密钥，然后使用AES解密数据
func (e *HybridEncryptor) Decrypt(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid hybrid encrypted data: too short")
	}

	// 读取加密密钥的长度
	keyLength := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])

	if len(data) < 4+int(keyLength) {
		return nil, errors.New("invalid hybrid encrypted data: missing key data")
	}

	// 提取加密的密钥和数据
	encryptedKey := data[4 : 4+keyLength]
	ciphertext := data[4+keyLength:]

	// 使用RSA解密AES密钥
	aesKey, err := e.rsaEncryptor.Decrypt(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// 使用AES解密数据
	aesEncryptor, err := NewAESEncryptor(aesKey)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesEncryptor.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// Base64Encoder Base64编码包装器
type Base64Encoder struct {
	encryptor DataEncryptor
}

// NewBase64Encoder 创建Base64编码包装器
func NewBase64Encoder(encryptor DataEncryptor) *Base64Encoder {
	return &Base64Encoder{
		encryptor: encryptor,
	}
}

// Encrypt 加密并Base64编码
func (e *Base64Encoder) Encrypt(plaintext []byte) ([]byte, error) {
	ciphertext, err := e.encryptor.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}

	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(ciphertext)))
	base64.StdEncoding.Encode(encoded, ciphertext)
	return encoded, nil
}

// Decrypt Base64解码并解密
func (e *Base64Encoder) Decrypt(encoded []byte) ([]byte, error) {
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Decode(decoded, encoded)
	if err != nil {
		return nil, err
	}

	return e.encryptor.Decrypt(decoded[:n])
}

// 辅助函数

// GenerateAESKey 生成AES密钥
func GenerateAESKey(bits int) ([]byte, error) {
	if bits != 128 && bits != 192 && bits != 256 {
		return nil, fmt.Errorf("AES key size must be 128, 192, or 256 bits, got %d", bits)
	}

	key := make([]byte, bits/8)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	return key, nil
}

// GenerateRSAKeyPair 生成RSA密钥对
func GenerateRSAKeyPair(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, &privateKey.PublicKey, nil
}

// SaveRSAKeyPair 保存RSA密钥对到文件
func SaveRSAKeyPair(privateKey *rsa.PrivateKey, privateKeyFile, publicKeyFile string) error {
	// 保存私钥
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	privateKeyPEM := pem.EncodeToMemory(privateKeyBlock)
	if err := os.WriteFile(privateKeyFile, privateKeyPEM, 0600); err != nil {
		return fmt.Errorf("save private key: %w", err)
	}

	// 保存公钥
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}

	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	publicKeyPEM := pem.EncodeToMemory(publicKeyBlock)
	if err := os.WriteFile(publicKeyFile, publicKeyPEM, 0644); err != nil {
		return fmt.Errorf("save public key: %w", err)
	}

	return nil
}

// loadPrivateKeyFromFile 从文件加载RSA私钥
func loadPrivateKeyFromFile(filename string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return parsePrivateKeyPEM(data)
}

// loadPublicKeyFromFile 从文件加载RSA公钥
func loadPublicKeyFromFile(filename string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return parsePublicKeyPEM(data)
}

// parsePrivateKeyPEM 解析PEM格式的RSA私钥
func parsePrivateKeyPEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// parsePublicKeyPEM 解析PEM格式的RSA公钥
func parsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	publicKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("parsed key is not an RSA public key")
	}

	return publicKey, nil
}
