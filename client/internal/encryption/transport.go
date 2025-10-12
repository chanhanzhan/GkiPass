package encryption

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// TransportEncryptor HTTP传输加密器
type TransportEncryptor struct {
	encryptor DataEncryptor
	logger    *zap.Logger
}

// NewTransportEncryptor 创建HTTP传输加密器
func NewTransportEncryptor(encryptor DataEncryptor, logger *zap.Logger) *TransportEncryptor {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &TransportEncryptor{
		encryptor: encryptor,
		logger:    logger,
	}
}

// EncryptedTransport 加密传输中间件
// 实现http.RoundTripper接口
type EncryptedTransport struct {
	encryptor     *TransportEncryptor
	baseTransport http.RoundTripper
	logger        *zap.Logger

	// 配置选项
	encryptPaths     []string // 需要加密的路径前缀
	encryptBodies    bool     // 是否加密请求体
	encryptHeaders   bool     // 是否加密敏感请求头
	sensitiveHeaders []string // 敏感请求头列表
}

// NewEncryptedTransport 创建加密传输中间件
func NewEncryptedTransport(encryptor *TransportEncryptor, baseTransport http.RoundTripper, logger *zap.Logger) *EncryptedTransport {
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &EncryptedTransport{
		encryptor:      encryptor,
		baseTransport:  baseTransport,
		logger:         logger,
		encryptPaths:   []string{"/api/v1/auth", "/api/v1/users", "/api/v1/private"},
		encryptBodies:  true,
		encryptHeaders: true,
		sensitiveHeaders: []string{
			"Authorization",
			"X-API-Key",
			"Cookie",
			"Set-Cookie",
			"X-Auth-Token",
		},
	}
}

// RoundTrip 实现http.RoundTripper接口
func (t *EncryptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 检查请求是否需要加密
	if !t.shouldEncrypt(req.URL.Path) {
		// 不需要加密，直接使用基础传输
		return t.baseTransport.RoundTrip(req)
	}

	// 克隆请求（因为req是只读的）
	reqCopy := t.cloneRequest(req)

	// 加密请求
	if err := t.encryptRequest(reqCopy); err != nil {
		t.logger.Error("Failed to encrypt request",
			zap.String("url", req.URL.String()),
			zap.Error(err))
		return nil, fmt.Errorf("encrypt request: %w", err)
	}

	// 发送加密请求
	resp, err := t.baseTransport.RoundTrip(reqCopy)
	if err != nil {
		return nil, err
	}

	// 解密响应
	if err := t.decryptResponse(resp); err != nil {
		t.logger.Error("Failed to decrypt response",
			zap.String("url", req.URL.String()),
			zap.Error(err))
		return nil, fmt.Errorf("decrypt response: %w", err)
	}

	return resp, nil
}

// shouldEncrypt 检查是否应该加密请求
func (t *EncryptedTransport) shouldEncrypt(path string) bool {
	for _, prefix := range t.encryptPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// cloneRequest 克隆HTTP请求
func (t *EncryptedTransport) cloneRequest(req *http.Request) *http.Request {
	reqCopy := new(http.Request)
	*reqCopy = *req

	// 克隆请求头
	reqCopy.Header = make(http.Header)
	for k, v := range req.Header {
		reqCopy.Header[k] = v
	}

	// 如果有请求体，读取并克隆
	if req.Body != nil {
		// 读取原始请求体
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body.Close()

		// 重新创建原始请求的请求体
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 为克隆的请求创建请求体
		reqCopy.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		reqCopy.ContentLength = int64(len(bodyBytes))
	}

	return reqCopy
}

// encryptRequest 加密HTTP请求
func (t *EncryptedTransport) encryptRequest(req *http.Request) error {
	// 加密敏感请求头
	if t.encryptHeaders {
		for _, header := range t.sensitiveHeaders {
			if values, ok := req.Header[header]; ok && len(values) > 0 {
				// 读取原始值
				originalValue := values[0]

				// 加密值
				encryptedValue, err := t.encryptValue(originalValue)
				if err != nil {
					return fmt.Errorf("encrypt header %s: %w", header, err)
				}

				// 替换原始值
				req.Header.Set(header, encryptedValue)

				// 标记为已加密
				req.Header.Set("X-Encrypted-Headers", header)
			}
		}
	}

	// 如果需要加密请求体
	if t.encryptBodies && req.Body != nil {
		// 读取原始请求体
		bodyBytes, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return fmt.Errorf("read request body: %w", err)
		}

		// 加密请求体
		encryptedBody, err := t.encryptor.encryptor.Encrypt(bodyBytes)
		if err != nil {
			return fmt.Errorf("encrypt request body: %w", err)
		}

		// 替换请求体
		req.Body = io.NopCloser(bytes.NewBuffer(encryptedBody))
		req.ContentLength = int64(len(encryptedBody))

		// 添加标记，表示请求体已加密
		req.Header.Set("X-Encrypted-Body", "true")
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	return nil
}

// decryptResponse 解密HTTP响应
func (t *EncryptedTransport) decryptResponse(resp *http.Response) error {
	// 检查响应体是否加密
	isBodyEncrypted := resp.Header.Get("X-Encrypted-Body") == "true"

	// 解密响应体
	if isBodyEncrypted && resp.Body != nil {
		// 读取加密的响应体
		encryptedBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read encrypted response body: %w", err)
		}

		// 解密响应体
		decryptedBody, err := t.encryptor.encryptor.Decrypt(encryptedBody)
		if err != nil {
			return fmt.Errorf("decrypt response body: %w", err)
		}

		// 替换响应体
		resp.Body = io.NopCloser(bytes.NewBuffer(decryptedBody))
		resp.ContentLength = int64(len(decryptedBody))

		// 更新内容类型（假设是JSON）
		resp.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp.Header.Del("X-Encrypted-Body")
	}

	// 检查是否有加密的响应头
	encryptedHeaders := resp.Header.Get("X-Encrypted-Headers")
	if encryptedHeaders != "" && t.encryptHeaders {
		// 解析加密的头部列表
		headerNames := strings.Split(encryptedHeaders, ",")

		for _, headerName := range headerNames {
			headerName = strings.TrimSpace(headerName)
			if values, ok := resp.Header[headerName]; ok && len(values) > 0 {
				// 读取加密值
				encryptedValue := values[0]

				// 解密值
				decryptedValue, err := t.decryptValue(encryptedValue)
				if err != nil {
					return fmt.Errorf("decrypt header %s: %w", headerName, err)
				}

				// 替换加密值
				resp.Header.Set(headerName, decryptedValue)
			}
		}

		// 删除标记
		resp.Header.Del("X-Encrypted-Headers")
	}

	return nil
}

// encryptValue 加密值（字符串）
func (t *EncryptedTransport) encryptValue(value string) (string, error) {
	encryptedBytes, err := t.encryptor.encryptor.Encrypt([]byte(value))
	if err != nil {
		return "", err
	}

	// 使用Base64编码加密后的字节
	encoder := NewBase64Encoder(nil)
	encodedBytes, err := encoder.Encrypt(encryptedBytes)
	if err != nil {
		return "", err
	}

	return string(encodedBytes), nil
}

// decryptValue 解密值（字符串）
func (t *EncryptedTransport) decryptValue(encryptedValue string) (string, error) {
	// 解析Base64编码
	encoder := NewBase64Encoder(nil)
	encryptedBytes, err := encoder.Decrypt([]byte(encryptedValue))
	if err != nil {
		return "", err
	}

	// 解密字节
	decryptedBytes, err := t.encryptor.encryptor.Decrypt(encryptedBytes)
	if err != nil {
		return "", err
	}

	return string(decryptedBytes), nil
}

// EncryptJSON 加密JSON数据
func (t *TransportEncryptor) EncryptJSON(data interface{}) ([]byte, error) {
	// 序列化为JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	// 加密JSON
	encryptedData, err := t.encryptor.Encrypt(jsonData)
	if err != nil {
		return nil, fmt.Errorf("encrypt json: %w", err)
	}

	// 封装在特殊格式中
	wrapper := map[string]interface{}{
		"encrypted": true,
		"data":      encryptedData,
	}

	// 序列化封装数据
	return json.Marshal(wrapper)
}

// DecryptJSON 解密JSON数据
func (t *TransportEncryptor) DecryptJSON(encryptedData []byte, result interface{}) error {
	// 解析封装数据
	var wrapper struct {
		Encrypted bool            `json:"encrypted"`
		Data      json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(encryptedData, &wrapper); err != nil {
		// 可能不是加密数据，尝试直接解析
		return json.Unmarshal(encryptedData, result)
	}

	// 检查是否加密
	if !wrapper.Encrypted {
		// 直接解析原始数据
		return json.Unmarshal(wrapper.Data, result)
	}

	// 解密数据
	decryptedData, err := t.encryptor.Decrypt(wrapper.Data)
	if err != nil {
		return fmt.Errorf("decrypt data: %w", err)
	}

	// 解析解密后的JSON
	return json.Unmarshal(decryptedData, result)
}
