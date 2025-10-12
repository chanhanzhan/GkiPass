package common

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StringUtils 字符串工具函数
type StringUtils struct{}

// IsEmpty 检查字符串是否为空
func (StringUtils) IsEmpty(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// IsNotEmpty 检查字符串是否不为空
func (StringUtils) IsNotEmpty(s string) bool {
	return !StringUtils{}.IsEmpty(s)
}

// Contains 检查字符串是否包含子字符串（忽略大小写）
func (StringUtils) Contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// TruncateWithEllipsis 截断字符串并添加省略号
func (StringUtils) TruncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// PadLeft 左填充字符串
func (StringUtils) PadLeft(s string, length int, pad rune) string {
	if len(s) >= length {
		return s
	}
	return strings.Repeat(string(pad), length-len(s)) + s
}

// PadRight 右填充字符串
func (StringUtils) PadRight(s string, length int, pad rune) string {
	if len(s) >= length {
		return s
	}
	return s + strings.Repeat(string(pad), length-len(s))
}

// RandomString 生成随机字符串
func (StringUtils) RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}
	return string(bytes)
}

// Hash 计算字符串的SHA256哈希
func (StringUtils) Hash(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

// SliceUtils 切片工具函数
type SliceUtils struct{}

// Contains 检查切片是否包含指定元素
func (SliceUtils) Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ContainsInt 检查整数切片是否包含指定元素
func (SliceUtils) ContainsInt(slice []int, item int) bool {
	for _, i := range slice {
		if i == item {
			return true
		}
	}
	return false
}

// Unique 去重字符串切片
func (SliceUtils) Unique(slice []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// RemoveEmpty removes empty strings from slice
func (SliceUtils) RemoveEmpty(slice []string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// Chunk 将切片分块
func (SliceUtils) Chunk(slice []string, size int) [][]string {
	if size <= 0 {
		return nil
	}

	var chunks [][]string
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}

	return chunks
}

// MapUtils Map工具函数
type MapUtils struct{}

// Keys 获取map的所有键
func (MapUtils) Keys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Values 获取map的所有值
func (MapUtils) Values(m map[string]interface{}) []interface{} {
	values := make([]interface{}, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// Merge 合并多个map
func (MapUtils) Merge(maps ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// DeepCopy 深拷贝map
func (MapUtils) DeepCopy(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for key, value := range original {
		copy[key] = deepCopyValue(value)
	}
	return copy
}

// deepCopyValue 递归深拷贝值
func deepCopyValue(original interface{}) interface{} {
	switch v := original.(type) {
	case map[string]interface{}:
		return MapUtils{}.DeepCopy(v)
	case []interface{}:
		copy := make([]interface{}, len(v))
		for i, item := range v {
			copy[i] = deepCopyValue(item)
		}
		return copy
	default:
		return v
	}
}

// TimeUtils 时间工具函数
type TimeUtils struct{}

// Now 获取当前时间
func (TimeUtils) Now() time.Time {
	return time.Now()
}

// Unix 获取Unix时间戳
func (TimeUtils) Unix() int64 {
	return time.Now().Unix()
}

// UnixMilli 获取毫秒级Unix时间戳
func (TimeUtils) UnixMilli() int64 {
	return time.Now().UnixMilli()
}

// FormatDuration 格式化持续时间
func (TimeUtils) FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.2fm", d.Minutes())
	}
	return fmt.Sprintf("%.2fh", d.Hours())
}

// ParseDuration 解析持续时间字符串
func (TimeUtils) ParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// Sleep 休眠指定时间
func (TimeUtils) Sleep(d time.Duration) {
	time.Sleep(d)
}

// SleepWithContext 带上下文的休眠
func (TimeUtils) SleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// NetworkUtils 网络工具函数
type NetworkUtils struct{}

// IsValidIP 检查IP地址是否有效
func (NetworkUtils) IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsValidPort 检查端口是否有效
func (NetworkUtils) IsValidPort(port int) bool {
	return port > 0 && port <= 65535
}

// IsValidPortString 检查端口字符串是否有效
func (NetworkUtils) IsValidPortString(port string) bool {
	p, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return NetworkUtils{}.IsValidPort(p)
}

// ParseHostPort 解析主机和端口
func (NetworkUtils) ParseHostPort(hostport string) (host string, port int, err error) {
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", 0, err
	}

	port, err = strconv.Atoi(p)
	if err != nil {
		return "", 0, err
	}

	return h, port, nil
}

// GetLocalIP 获取本机IP地址
func (NetworkUtils) GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("未找到有效的本机IP地址")
}

// IsPortOpen 检查端口是否开放
func (NetworkUtils) IsPortOpen(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// JSONUtils JSON工具函数
type JSONUtils struct{}

// Marshal 序列化为JSON
func (JSONUtils) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// MarshalIndent 格式化序列化为JSON
func (JSONUtils) MarshalIndent(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// Unmarshal 反序列化JSON
func (JSONUtils) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// ToString 转换为JSON字符串
func (JSONUtils) ToString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// ToStringIndent 转换为格式化JSON字符串
func (JSONUtils) ToStringIndent(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

// ErrorUtils 错误工具函数
type ErrorUtils struct{}

// Wrap 包装错误
func (ErrorUtils) Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// WrapWithContext 带上下文包装错误
func (ErrorUtils) WrapWithContext(err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	contextStr := JSONUtils{}.ToString(context)
	return fmt.Errorf("%w (context: %s)", err, contextStr)
}

// IsContextError 检查是否是上下文错误
func (ErrorUtils) IsContextError(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

// IsNetworkError 检查是否是网络错误
func (ErrorUtils) IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"network unreachable",
		"no route to host",
		"broken pipe",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(errStr, netErr) {
			return true
		}
	}

	return false
}

// ReflectUtils 反射工具函数
type ReflectUtils struct{}

// GetTypeName 获取类型名称
func (ReflectUtils) GetTypeName(v interface{}) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}

// IsNil 检查是否为nil
func (ReflectUtils) IsNil(v interface{}) bool {
	if v == nil {
		return true
	}

	value := reflect.ValueOf(v)
	kind := value.Kind()

	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// IsZero 检查是否为零值
func (ReflectUtils) IsZero(v interface{}) bool {
	if v == nil {
		return true
	}
	return reflect.ValueOf(v).IsZero()
}

// RuntimeUtils 运行时工具函数
type RuntimeUtils struct{}

// GetFunctionName 获取函数名称
func (RuntimeUtils) GetFunctionName() string {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		return "unknown"
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}

	name := fn.Name()
	if lastSlash := strings.LastIndex(name, "/"); lastSlash >= 0 {
		name = name[lastSlash+1:]
	}
	if lastDot := strings.LastIndex(name, "."); lastDot >= 0 {
		name = name[lastDot+1:]
	}

	return name
}

// GetGoroutineID 获取当前Goroutine ID
func (RuntimeUtils) GetGoroutineID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)

	// 解析 "goroutine 1 [running]:"
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, _ := strconv.Atoi(idField)
	return id
}

// GetMemStats 获取内存统计信息
func (RuntimeUtils) GetMemStats() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}

// ForceGC 强制垃圾回收
func (RuntimeUtils) ForceGC() {
	runtime.GC()
}

// ConcurrencyUtils 并发工具函数
type ConcurrencyUtils struct{}

// SafeGo 安全启动Goroutine
func (ConcurrencyUtils) SafeGo(fn func(), logger *zap.Logger) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error("Goroutine panic recovered",
						zap.Any("panic", r),
						zap.String("stack", string(debug.Stack())))
				}
			}
		}()
		fn()
	}()
}

// WithTimeout 带超时执行函数
func (ConcurrencyUtils) WithTimeout(fn func() error, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("操作超时")
	}
}

// ParallelRun 并行执行多个函数
func (ConcurrencyUtils) ParallelRun(fns ...func() error) []error {
	errors := make([]error, len(fns))
	var wg sync.WaitGroup

	for i, fn := range fns {
		wg.Add(1)
		go func(index int, f func() error) {
			defer wg.Done()
			errors[index] = f()
		}(i, fn)
	}

	wg.Wait()
	return errors
}

// RetryUtils 重试工具函数
type RetryUtils struct{}

// Retry 简单重试
func (RetryUtils) Retry(fn func() error, maxAttempts int, delay time.Duration) error {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(delay)
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("重试失败，最后错误: %w", lastErr)
}

// RetryWithBackoff 指数退避重试
func (RetryUtils) RetryWithBackoff(fn func() error, maxAttempts int, initialDelay time.Duration) error {
	var lastErr error
	delay := initialDelay

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(delay)
				delay *= 2 // 指数退避
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("重试失败，最后错误: %w", lastErr)
}

// RetryWithContext 带上下文的重试
func (RetryUtils) RetryWithContext(ctx context.Context, fn func() error, maxAttempts int, delay time.Duration) error {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := fn(); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("重试失败，最后错误: %w", lastErr)
}

// ValidationUtils 验证工具函数
type ValidationUtils struct{}

// IsEmail 验证邮箱格式
func (ValidationUtils) IsEmail(email string) bool {
	// 简单的邮箱验证
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

// IsURL 验证URL格式
func (ValidationUtils) IsURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// IsNumeric 验证是否为数字
func (ValidationUtils) IsNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// IsAlphanumeric 验证是否为字母数字
func (ValidationUtils) IsAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// ConversionUtils 转换工具函数
type ConversionUtils struct{}

// ToInt 转换为整数
func (ConversionUtils) ToInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, fmt.Errorf("无法转换为整数: %v", v)
	}
}

// ToString 转换为字符串
func (ConversionUtils) ToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ToBool 转换为布尔值
func (ConversionUtils) ToBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return strconv.ParseBool(val)
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case float64:
		return val != 0, nil
	default:
		return false, fmt.Errorf("无法转换为布尔值: %v", v)
	}
}

// 实例化全局工具对象
var (
	String      = StringUtils{}
	Slice       = SliceUtils{}
	Map         = MapUtils{}
	Time        = TimeUtils{}
	Network     = NetworkUtils{}
	JSON        = JSONUtils{}
	Error       = ErrorUtils{}
	Reflect     = ReflectUtils{}
	Runtime     = RuntimeUtils{}
	Concurrency = ConcurrencyUtils{}
	Retry       = RetryUtils{}
	Validation  = ValidationUtils{}
	Conversion  = ConversionUtils{}
)
