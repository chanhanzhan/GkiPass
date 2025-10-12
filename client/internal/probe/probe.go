package probe

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ProbeType 探针类型
type ProbeType string

const (
	ProbeTCP       ProbeType = "tcp"
	ProbeUDP       ProbeType = "udp"
	ProbeICMP      ProbeType = "icmp"
	ProbeHTTP      ProbeType = "http"
	ProbeHTTPS     ProbeType = "https"
	ProbeWebSocket ProbeType = "ws"
)

// ProbeResult 探针结果
type ProbeResult struct {
	Success      bool                   `json:"success"`
	Type         ProbeType              `json:"type"`
	Target       string                 `json:"target"`
	RTT          time.Duration          `json:"rtt"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// ProbeOptions 探针选项
type ProbeOptions struct {
	Type          ProbeType              `json:"type"`
	Target        string                 `json:"target"`
	Count         int                    `json:"count"`
	Interval      time.Duration          `json:"interval"`
	Timeout       time.Duration          `json:"timeout"`
	MaxConcurrent int                    `json:"max_concurrent"`
	ExtraOptions  map[string]interface{} `json:"extra_options,omitempty"`
}

// DefaultProbeOptions 默认探针选项
func DefaultProbeOptions() *ProbeOptions {
	return &ProbeOptions{
		Type:          ProbeTCP,
		Count:         5,
		Interval:      time.Second,
		Timeout:       5 * time.Second,
		MaxConcurrent: 1,
	}
}

// ProbeManager 探针管理器
type ProbeManager struct {
	logger *zap.Logger
	active sync.Map
}

// NewProbeManager 创建探针管理器
func NewProbeManager() *ProbeManager {
	return &ProbeManager{
		logger: zap.L().Named("probe-manager"),
	}
}

// Probe 执行探测
func (m *ProbeManager) Probe(ctx context.Context, options *ProbeOptions) ([]*ProbeResult, error) {
	if options == nil {
		options = DefaultProbeOptions()
	}

	// 验证选项
	if options.Target == "" {
		return nil, fmt.Errorf("目标地址不能为空")
	}

	if options.Count <= 0 {
		options.Count = 1
	}

	if options.Interval <= 0 {
		options.Interval = time.Second
	}

	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Second
	}

	if options.MaxConcurrent <= 0 {
		options.MaxConcurrent = 1
	}

	// 创建结果通道
	resultChan := make(chan *ProbeResult, options.Count)
	defer close(resultChan)

	// 创建信号量控制并发
	sem := make(chan struct{}, options.MaxConcurrent)
	defer close(sem)

	// 启动探测协程
	var wg sync.WaitGroup
	for i := 0; i < options.Count; i++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// 继续执行
		}

		// 等待信号量
		sem <- struct{}{}

		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()

			// 执行单次探测
			result := m.probeOnce(ctx, options)
			result.Details["sequence"] = index + 1
			resultChan <- result

			// 等待间隔时间
			if index < options.Count-1 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(options.Interval):
					// 继续下一次探测
				}
			}
		}(i)
	}

	// 等待所有探测完成
	wg.Wait()

	// 收集结果
	var results []*ProbeResult
	for i := 0; i < options.Count; i++ {
		select {
		case result := <-resultChan:
			results = append(results, result)
		default:
			// 通道已关闭或没有更多结果
			break
		}
	}

	return results, nil
}

// probeOnce 执行单次探测
func (m *ProbeManager) probeOnce(ctx context.Context, options *ProbeOptions) *ProbeResult {
	result := &ProbeResult{
		Type:      options.Type,
		Target:    options.Target,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// 创建超时上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	// 根据探针类型执行不同的探测
	startTime := time.Now()
	var err error

	switch options.Type {
	case ProbeTCP:
		err = m.probeTCP(timeoutCtx, options, result)
	case ProbeUDP:
		err = m.probeUDP(timeoutCtx, options, result)
	case ProbeICMP:
		err = m.probeICMP(timeoutCtx, options, result)
	case ProbeHTTP, ProbeHTTPS:
		err = m.probeHTTP(timeoutCtx, options, result)
	case ProbeWebSocket:
		err = m.probeWebSocket(timeoutCtx, options, result)
	default:
		err = fmt.Errorf("不支持的探针类型: %s", options.Type)
	}

	// 计算RTT
	result.RTT = time.Since(startTime)

	// 设置结果
	if err != nil {
		result.Success = false
		result.ErrorMessage = err.Error()
	} else {
		result.Success = true
	}

	return result
}

// probeTCP 执行TCP探测
func (m *ProbeManager) probeTCP(ctx context.Context, options *ProbeOptions, result *ProbeResult) error {
	// 解析地址
	target := options.Target
	if _, _, err := net.SplitHostPort(target); err != nil {
		// 如果没有端口，默认使用80
		target = fmt.Sprintf("%s:80", target)
	}

	// 创建TCP连接
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return fmt.Errorf("TCP连接失败: %w", err)
	}
	defer conn.Close()

	// 获取本地地址
	localAddr := conn.LocalAddr().String()
	result.Details["local_addr"] = localAddr

	// 获取远程地址
	remoteAddr := conn.RemoteAddr().String()
	result.Details["remote_addr"] = remoteAddr

	return nil
}

// probeUDP 执行UDP探测
func (m *ProbeManager) probeUDP(ctx context.Context, options *ProbeOptions, result *ProbeResult) error {
	// 解析地址
	target := options.Target
	if _, _, err := net.SplitHostPort(target); err != nil {
		// 如果没有端口，默认使用53
		target = fmt.Sprintf("%s:53", target)
	}

	// 解析UDP地址
	addr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return fmt.Errorf("解析UDP地址失败: %w", err)
	}

	// 创建UDP连接
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("UDP连接失败: %w", err)
	}
	defer conn.Close()

	// 发送探测包
	_, err = conn.Write([]byte("PING"))
	if err != nil {
		return fmt.Errorf("发送UDP数据失败: %w", err)
	}

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(options.Timeout))

	// 尝试读取响应
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		// UDP探测不一定会收到响应，所以不返回错误
		result.Details["response"] = false
		return nil
	}

	result.Details["response"] = true
	result.Details["response_size"] = n
	result.Details["response_data"] = string(buf[:n])

	return nil
}

// probeICMP 执行ICMP探测
func (m *ProbeManager) probeICMP(ctx context.Context, options *ProbeOptions, result *ProbeResult) error {
	// ICMP探测需要特殊权限，这里使用系统ping命令
	return fmt.Errorf("ICMP探测未实现")
}

// probeHTTP 执行HTTP探测
func (m *ProbeManager) probeHTTP(ctx context.Context, options *ProbeOptions, result *ProbeResult) error {
	// HTTP探测
	return fmt.Errorf("HTTP探测未实现")
}

// probeWebSocket 执行WebSocket探测
func (m *ProbeManager) probeWebSocket(ctx context.Context, options *ProbeOptions, result *ProbeResult) error {
	// WebSocket探测
	return fmt.Errorf("WebSocket探测未实现")
}

// ProbeTunnel 探测隧道
func (m *ProbeManager) ProbeTunnel(ctx context.Context, ingressNodeID, egressNodeID, targetAddr string, probeType ProbeType) ([]*ProbeResult, error) {
	// 创建探测选项
	options := DefaultProbeOptions()
	options.Type = probeType
	options.Target = targetAddr
	options.Count = 5
	options.Timeout = 20 * time.Second

	// 执行探测
	return m.Probe(ctx, options)
}

// ProbeNodeToNode 节点间探测
func (m *ProbeManager) ProbeNodeToNode(ctx context.Context, sourceNodeID, targetNodeID string, probeType ProbeType) ([]*ProbeResult, error) {
	// 创建探测选项
	options := DefaultProbeOptions()
	options.Type = probeType
	options.Target = targetNodeID
	options.Count = 5
	options.Timeout = 20 * time.Second

	// 执行探测
	return m.Probe(ctx, options)
}

// ProbeNodeToServer 节点到服务器探测
func (m *ProbeManager) ProbeNodeToServer(ctx context.Context, nodeID, serverAddr string, probeType ProbeType) ([]*ProbeResult, error) {
	// 创建探测选项
	options := DefaultProbeOptions()
	options.Type = probeType
	options.Target = serverAddr
	options.Count = 5
	options.Timeout = 20 * time.Second

	// 执行探测
	return m.Probe(ctx, options)
}

// AnalyzeResults 分析探测结果
func (m *ProbeManager) AnalyzeResults(results []*ProbeResult) map[string]interface{} {
	if len(results) == 0 {
		return map[string]interface{}{
			"success_rate": 0.0,
			"min_rtt":      0,
			"max_rtt":      0,
			"avg_rtt":      0,
			"status":       "failed",
			"message":      "无探测结果",
		}
	}

	// 统计成功次数
	successCount := 0
	var minRTT, maxRTT, totalRTT time.Duration

	for i, result := range results {
		if result.Success {
			successCount++

			// 初始化最小/最大RTT
			if i == 0 || result.RTT < minRTT {
				minRTT = result.RTT
			}
			if i == 0 || result.RTT > maxRTT {
				maxRTT = result.RTT
			}

			totalRTT += result.RTT
		}
	}

	// 计算成功率
	successRate := float64(successCount) / float64(len(results)) * 100

	// 计算平均RTT
	var avgRTT time.Duration
	if successCount > 0 {
		avgRTT = totalRTT / time.Duration(successCount)
	}

	// 确定状态
	status := "failed"
	message := "探测失败"
	if successRate >= 100 {
		status = "excellent"
		message = "连接良好"
	} else if successRate >= 80 {
		status = "good"
		message = "连接正常，有少量丢包"
	} else if successRate >= 50 {
		status = "fair"
		message = "连接不稳定，丢包率较高"
	} else if successRate > 0 {
		status = "poor"
		message = "连接质量差，大量丢包"
	}

	return map[string]interface{}{
		"success_rate":  successRate,
		"min_rtt":       minRTT.Milliseconds(),
		"max_rtt":       maxRTT.Milliseconds(),
		"avg_rtt":       avgRTT.Milliseconds(),
		"status":        status,
		"message":       message,
		"total_count":   len(results),
		"success_count": successCount,
		"failed_count":  len(results) - successCount,
	}
}
