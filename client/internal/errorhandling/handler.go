package errorhandling

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ErrorType 错误类型
type ErrorType string

const (
	// 网络类错误
	ErrorTypeNetwork     ErrorType = "network"
	ErrorTypeConnRefused ErrorType = "connection_refused"
	ErrorTypeConnTimeout ErrorType = "connection_timeout"
	ErrorTypeConnClosed  ErrorType = "connection_closed"
	ErrorTypeDNSError    ErrorType = "dns_error"

	// API类错误
	ErrorTypeAPI          ErrorType = "api"
	ErrorTypeAPITimeout   ErrorType = "api_timeout"
	ErrorTypeAPIRateLimit ErrorType = "api_rate_limit"
	ErrorTypeAPINotFound  ErrorType = "api_not_found"
	ErrorTypeAPIAuth      ErrorType = "api_auth"
	ErrorTypeAPIServer    ErrorType = "api_server"

	// WebSocket类错误
	ErrorTypeWebSocket     ErrorType = "websocket"
	ErrorTypeWSClosed      ErrorType = "ws_closed"
	ErrorTypeWSConnFailed  ErrorType = "ws_conn_failed"
	ErrorTypeWSReadFailed  ErrorType = "ws_read_failed"
	ErrorTypeWSWriteFailed ErrorType = "ws_write_failed"

	// 认证类错误
	ErrorTypeAuth        ErrorType = "auth"
	ErrorTypeAuthExpired ErrorType = "auth_expired"
	ErrorTypeAuthInvalid ErrorType = "auth_invalid"
	ErrorTypeAuthRefresh ErrorType = "auth_refresh_failed"

	// 系统类错误
	ErrorTypeSystem  ErrorType = "system"
	ErrorTypeMemory  ErrorType = "memory"
	ErrorTypeIO      ErrorType = "io"
	ErrorTypeRuntime ErrorType = "runtime"

	// 应用类错误
	ErrorTypeApp       ErrorType = "app"
	ErrorTypeConfig    ErrorType = "config"
	ErrorTypeRule      ErrorType = "rule"
	ErrorTypeOperation ErrorType = "operation"

	// 未知错误
	ErrorTypeUnknown ErrorType = "unknown"
)

// ErrorSeverity 错误严重程度
type ErrorSeverity string

const (
	SeverityDebug    ErrorSeverity = "debug"
	SeverityInfo     ErrorSeverity = "info"
	SeverityWarning  ErrorSeverity = "warning"
	SeverityError    ErrorSeverity = "error"
	SeverityCritical ErrorSeverity = "critical"
	SeverityFatal    ErrorSeverity = "fatal"
)

// ErrorInfo 错误信息
type ErrorInfo struct {
	Type        ErrorType              `json:"type"`
	Code        string                 `json:"code"`
	Message     string                 `json:"message"`
	Severity    ErrorSeverity          `json:"severity"`
	Timestamp   time.Time              `json:"timestamp"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Retryable   bool                   `json:"retryable"`
	Source      string                 `json:"source"`
	OriginalErr string                 `json:"original_err,omitempty"`
}

// Error 实现error接口
func (e *ErrorInfo) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Type, e.Message)
}

// ErrorHandler 错误处理器
type ErrorHandler struct {
	logger         *zap.Logger
	isReporting    bool
	reportEndpoint string
	reportInterval time.Duration
	maxErrors      int
	errorHistory   []*ErrorInfo
	errorCallback  func(*ErrorInfo)
	httpClient     *http.Client
}

// ErrorHandlerOptions 错误处理器选项
type ErrorHandlerOptions struct {
	Logger         *zap.Logger
	IsReporting    bool
	ReportEndpoint string
	ReportInterval time.Duration
	MaxErrors      int
	ErrorCallback  func(*ErrorInfo)
}

// DefaultErrorHandlerOptions 默认错误处理器选项
func DefaultErrorHandlerOptions() *ErrorHandlerOptions {
	return &ErrorHandlerOptions{
		Logger:         zap.L().Named("error-handler"),
		IsReporting:    true,
		ReportEndpoint: "",
		ReportInterval: 5 * time.Minute,
		MaxErrors:      100,
	}
}

// NewErrorHandler 创建错误处理器
func NewErrorHandler(options *ErrorHandlerOptions) *ErrorHandler {
	if options == nil {
		options = DefaultErrorHandlerOptions()
	}

	return &ErrorHandler{
		logger:         options.Logger,
		isReporting:    options.IsReporting,
		reportEndpoint: options.ReportEndpoint,
		reportInterval: options.ReportInterval,
		maxErrors:      options.MaxErrors,
		errorHistory:   make([]*ErrorInfo, 0, options.MaxErrors),
		errorCallback:  options.ErrorCallback,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Start 启动错误处理器
func (h *ErrorHandler) Start() {
	if h.isReporting && h.reportEndpoint != "" {
		go h.startReportLoop()
	}
}

// startReportLoop 启动错误上报循环
func (h *ErrorHandler) startReportLoop() {
	ticker := time.NewTicker(h.reportInterval)
	defer ticker.Stop()

	for range ticker.C {
		h.reportErrors()
	}
}

// reportErrors 上报错误
func (h *ErrorHandler) reportErrors() {
	if len(h.errorHistory) == 0 {
		return
	}

	// 准备上报数据
	data := map[string]interface{}{
		"errors":    h.errorHistory,
		"count":     len(h.errorHistory),
		"timestamp": time.Now(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		h.logger.Error("序列化错误数据失败", zap.Error(err))
		return
	}

	// 发送错误报告
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", h.reportEndpoint, strings.NewReader(string(jsonData)))
	if err != nil {
		h.logger.Error("创建错误上报请求失败", zap.Error(err))
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.Error("发送错误上报失败", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.Error("错误上报响应异常",
			zap.Int("status_code", resp.StatusCode))
		return
	}

	// 清空已上报的错误
	h.errorHistory = h.errorHistory[:0]
}

// HandleError 处理错误
func (h *ErrorHandler) HandleError(err error) *ErrorInfo {
	if err == nil {
		return nil
	}

	// 如果错误已经是 ErrorInfo，直接返回
	if errorInfo, ok := err.(*ErrorInfo); ok {
		return h.processError(errorInfo)
	}

	// 转换为 ErrorInfo
	errorInfo := h.classifyError(err)
	return h.processError(errorInfo)
}

// processError 处理错误信息
func (h *ErrorHandler) processError(errorInfo *ErrorInfo) *ErrorInfo {
	// 记录日志
	h.logError(errorInfo)

	// 调用回调函数
	if h.errorCallback != nil {
		h.errorCallback(errorInfo)
	}

	// 添加到错误历史
	h.addErrorToHistory(errorInfo)

	return errorInfo
}

// classifyError 分类错误
func (h *ErrorHandler) classifyError(err error) *ErrorInfo {
	errStr := err.Error()
	errType := ErrorTypeUnknown
	severity := SeverityError
	retryable := false
	code := "E-UNKNOWN"
	details := make(map[string]interface{})

	// 检查网络错误
	if isNetworkError(err) {
		errType = ErrorTypeNetwork
		retryable = true

		// 细分网络错误类型
		if strings.Contains(errStr, "connection refused") {
			errType = ErrorTypeConnRefused
			code = "E-CONN-REFUSED"
		} else if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
			errType = ErrorTypeConnTimeout
			code = "E-CONN-TIMEOUT"
		} else if strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "broken pipe") {
			errType = ErrorTypeConnClosed
			code = "E-CONN-CLOSED"
		} else if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "lookup") {
			errType = ErrorTypeDNSError
			code = "E-DNS"
		} else {
			code = "E-NET-OTHER"
		}

		severity = SeverityWarning
	} else if isAPIError(err) {
		// API 错误
		errType = ErrorTypeAPI
		code = "E-API"

		// 解析状态码
		statusCode := extractStatusCode(errStr)
		if statusCode > 0 {
			details["status_code"] = statusCode
			code = fmt.Sprintf("E-API-%d", statusCode)

			// 根据状态码细分错误类型
			switch {
			case statusCode == 401 || statusCode == 403:
				errType = ErrorTypeAPIAuth
				severity = SeverityCritical
				retryable = false
			case statusCode == 404:
				errType = ErrorTypeAPINotFound
				severity = SeverityWarning
				retryable = false
			case statusCode == 429:
				errType = ErrorTypeAPIRateLimit
				severity = SeverityWarning
				retryable = true
			case statusCode >= 500:
				errType = ErrorTypeAPIServer
				severity = SeverityError
				retryable = true
			}
		}
	} else if isWebSocketError(err) {
		// WebSocket 错误
		errType = ErrorTypeWebSocket
		code = "E-WS"
		retryable = true

		if strings.Contains(errStr, "close") {
			errType = ErrorTypeWSClosed
			code = "E-WS-CLOSED"
		} else if strings.Contains(errStr, "connect") {
			errType = ErrorTypeWSConnFailed
			code = "E-WS-CONNECT"
		} else if strings.Contains(errStr, "read") {
			errType = ErrorTypeWSReadFailed
			code = "E-WS-READ"
		} else if strings.Contains(errStr, "write") {
			errType = ErrorTypeWSWriteFailed
			code = "E-WS-WRITE"
		}
	} else if isAuthError(err) {
		// 认证错误
		errType = ErrorTypeAuth
		code = "E-AUTH"
		severity = SeverityCritical

		if strings.Contains(errStr, "expired") {
			errType = ErrorTypeAuthExpired
			code = "E-AUTH-EXPIRED"
			retryable = true
		} else if strings.Contains(errStr, "invalid") {
			errType = ErrorTypeAuthInvalid
			code = "E-AUTH-INVALID"
		} else if strings.Contains(errStr, "refresh") {
			errType = ErrorTypeAuthRefresh
			code = "E-AUTH-REFRESH"
			retryable = true
		}
	} else if isSystemError(err) {
		// 系统错误
		errType = ErrorTypeSystem
		code = "E-SYS"

		if strings.Contains(errStr, "memory") {
			errType = ErrorTypeMemory
			code = "E-SYS-MEM"
			severity = SeverityCritical
		} else if strings.Contains(errStr, "file") || strings.Contains(errStr, "directory") || strings.Contains(errStr, "permission") {
			errType = ErrorTypeIO
			code = "E-SYS-IO"
			severity = SeverityError
		}
	}

	// 构建错误信息
	return &ErrorInfo{
		Type:        errType,
		Code:        code,
		Message:     err.Error(),
		Severity:    severity,
		Timestamp:   time.Now(),
		Details:     details,
		Retryable:   retryable,
		Source:      getCallerInfo(),
		OriginalErr: err.Error(),
	}
}

// logError 记录错误日志
func (h *ErrorHandler) logError(errorInfo *ErrorInfo) {
	// 根据严重程度使用不同的日志级别
	fields := []zap.Field{
		zap.String("code", errorInfo.Code),
		zap.String("type", string(errorInfo.Type)),
		zap.String("source", errorInfo.Source),
	}

	// 添加详情字段
	for k, v := range errorInfo.Details {
		fields = append(fields, zap.Any(k, v))
	}

	switch errorInfo.Severity {
	case SeverityCritical, SeverityFatal:
		h.logger.Error(errorInfo.Message, fields...)
	case SeverityError:
		h.logger.Error(errorInfo.Message, fields...)
	case SeverityWarning:
		h.logger.Warn(errorInfo.Message, fields...)
	case SeverityInfo:
		h.logger.Info(errorInfo.Message, fields...)
	case SeverityDebug:
		h.logger.Debug(errorInfo.Message, fields...)
	}
}

// addErrorToHistory 添加错误到历史记录
func (h *ErrorHandler) addErrorToHistory(errorInfo *ErrorInfo) {
	// 保持错误历史在最大容量范围内
	if len(h.errorHistory) >= h.maxErrors {
		// 移除最早的错误
		h.errorHistory = h.errorHistory[1:]
	}

	// 添加新错误
	h.errorHistory = append(h.errorHistory, errorInfo)
}

// GetErrorHistory 获取错误历史
func (h *ErrorHandler) GetErrorHistory() []*ErrorInfo {
	// 返回副本
	history := make([]*ErrorInfo, len(h.errorHistory))
	copy(history, h.errorHistory)
	return history
}

// ClearErrorHistory 清空错误历史
func (h *ErrorHandler) ClearErrorHistory() {
	h.errorHistory = h.errorHistory[:0]
}

// WrapError 包装错误
func (h *ErrorHandler) WrapError(err error, message string, errType ErrorType, code string) *ErrorInfo {
	if err == nil {
		return nil
	}

	// 如果错误已经是 ErrorInfo，更新信息
	if errorInfo, ok := err.(*ErrorInfo); ok {
		newErrorInfo := *errorInfo
		if message != "" {
			newErrorInfo.Message = message + ": " + errorInfo.Message
		}
		if errType != "" {
			newErrorInfo.Type = errType
		}
		if code != "" {
			newErrorInfo.Code = code
		}
		return h.processError(&newErrorInfo)
	}

	// 创建新的 ErrorInfo
	errorInfo := &ErrorInfo{
		Type:        errType,
		Code:        code,
		Message:     message + ": " + err.Error(),
		Severity:    SeverityError,
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Retryable:   false,
		Source:      getCallerInfo(),
		OriginalErr: err.Error(),
	}

	return h.processError(errorInfo)
}

// CreateError 创建新错误
func (h *ErrorHandler) CreateError(message string, errType ErrorType, code string, severity ErrorSeverity) *ErrorInfo {
	errorInfo := &ErrorInfo{
		Type:      errType,
		Code:      code,
		Message:   message,
		Severity:  severity,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
		Retryable: false,
		Source:    getCallerInfo(),
	}

	return h.processError(errorInfo)
}

// 判断错误类型的辅助函数
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否是 net 包中的错误
	if _, ok := err.(net.Error); ok {
		return true
	}

	// 检查错误字符串中的关键词
	errStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"timeout",
		"deadline exceeded",
		"network unreachable",
		"host unreachable",
		"i/o timeout",
	}

	for _, ne := range networkErrors {
		if strings.Contains(strings.ToLower(errStr), ne) {
			return true
		}
	}

	return false
}

func isAPIError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "API") || strings.Contains(errStr, "status code") || extractStatusCode(errStr) > 0
}

func isWebSocketError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	wsErrors := []string{
		"websocket",
		"ws:",
		"socket",
		"close",
		"connection closed",
		"not connected",
	}

	for _, wsErr := range wsErrors {
		if strings.Contains(errStr, wsErr) {
			return true
		}
	}

	return false
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	authErrors := []string{
		"auth",
		"authentication",
		"unauthorized",
		"permission",
		"token",
		"credential",
		"login",
		"access denied",
		"forbidden",
		"expired",
	}

	for _, authErr := range authErrors {
		if strings.Contains(errStr, authErr) {
			return true
		}
	}

	return false
}

func isSystemError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	sysErrors := []string{
		"file",
		"directory",
		"permission denied",
		"memory",
		"disk",
		"system",
		"out of",
		"resource",
	}

	for _, sysErr := range sysErrors {
		if strings.Contains(errStr, sysErr) {
			return true
		}
	}

	return false
}

// 从错误字符串中提取状态码
func extractStatusCode(errStr string) int {
	patterns := []string{
		"status code (%d)",
		"status (%d)",
		"(%d) ",
		"code=(%d)",
		"code: (%d)",
		"code %d",
	}

	for _, pattern := range patterns {
		// 替换格式化字符为正则表达式模式
		regexPattern := strings.Replace(pattern, "%d", "(\\d+)", 1)

		// 查找匹配
		if matches := strings.Split(errStr, regexPattern); len(matches) > 1 {
			if statusStr := strings.Split(matches[0], " "); len(statusStr) > 0 {
				if statusCode, err := strconv.Atoi(statusStr[len(statusStr)-1]); err == nil {
					return statusCode
				}
			}
		}
	}

	// 直接查找HTTP状态码
	statusCodes := []int{400, 401, 403, 404, 405, 408, 409, 429, 500, 501, 502, 503, 504}
	for _, code := range statusCodes {
		if strings.Contains(errStr, fmt.Sprintf("%d", code)) {
			return code
		}
	}

	return 0
}

// getCallerInfo 获取调用者信息
func getCallerInfo() string {
	return "unknown" // 实际应用中应该使用runtime包获取调用栈信息
}
