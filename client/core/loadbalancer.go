package core

import (
	"fmt"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// LoadBalancer 负载均衡器
type LoadBalancer struct {
	targets       []ws.TunnelTarget
	healthStatus  map[string]bool // "host:port" → healthy
	mu            sync.RWMutex
	checkInterval time.Duration
	stopChan      chan struct{}
}

// NewLoadBalancer 创建负载均衡器
func NewLoadBalancer(targets []ws.TunnelTarget) *LoadBalancer {
	lb := &LoadBalancer{
		targets:       targets,
		healthStatus:  make(map[string]bool),
		checkInterval: 30 * time.Second,
		stopChan:      make(chan struct{}),
	}

	// 初始化健康状态（假定所有目标健康）
	for _, target := range targets {
		key := fmt.Sprintf("%s:%d", target.Host, target.Port)
		lb.healthStatus[key] = true
	}

	return lb
}

// Start 启动健康检查
func (lb *LoadBalancer) Start() {
	go lb.healthCheckLoop()
}

// Stop 停止健康检查
func (lb *LoadBalancer) Stop() {
	close(lb.stopChan)
}

// SelectTarget 选择目标（加权随机，仅选择健康的目标）
func (lb *LoadBalancer) SelectTarget() (*ws.TunnelTarget, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// 筛选健康的目标
	healthyTargets := make([]ws.TunnelTarget, 0)
	for _, target := range lb.targets {
		key := fmt.Sprintf("%s:%d", target.Host, target.Port)
		if lb.healthStatus[key] {
			healthyTargets = append(healthyTargets, target)
		}
	}

	if len(healthyTargets) == 0 {
		return nil, fmt.Errorf("没有健康的目标")
	}

	if len(healthyTargets) == 1 {
		return &healthyTargets[0], nil
	}

	// 计算总权重
	totalWeight := 0
	for i := range healthyTargets {
		if healthyTargets[i].Weight <= 0 {
			healthyTargets[i].Weight = 1
		}
		totalWeight += healthyTargets[i].Weight
	}

	// 加权随机选择
	rand := int(time.Now().UnixNano() % int64(totalWeight))
	for _, t := range healthyTargets {
		rand -= t.Weight
		if rand < 0 {
			return &t, nil
		}
	}

	// 兜底
	return &healthyTargets[0], nil
}

// UpdateTargets 更新目标列表
func (lb *LoadBalancer) UpdateTargets(targets []ws.TunnelTarget) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.targets = targets

	// 初始化新目标的健康状态
	for _, target := range targets {
		key := fmt.Sprintf("%s:%d", target.Host, target.Port)
		if _, exists := lb.healthStatus[key]; !exists {
			lb.healthStatus[key] = true
		}
	}
}

// healthCheckLoop 健康检查循环
func (lb *LoadBalancer) healthCheckLoop() {
	ticker := time.NewTicker(lb.checkInterval)
	defer ticker.Stop()

	// 首次立即检查
	lb.performHealthCheck()

	for {
		select {
		case <-ticker.C:
			lb.performHealthCheck()
		case <-lb.stopChan:
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (lb *LoadBalancer) performHealthCheck() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for _, target := range lb.targets {
		key := fmt.Sprintf("%s:%d", target.Host, target.Port)
		healthy := lb.checkTarget(target)

		oldStatus := lb.healthStatus[key]
		lb.healthStatus[key] = healthy

		// 状态变化时记录日志
		if oldStatus != healthy {
			if healthy {
				logger.Info("目标恢复健康",
					zap.String("target", key))
			} else {
				logger.Warn("目标不健康",
					zap.String("target", key))
			}
		}
	}
}

// checkTarget 检查单个目标
func (lb *LoadBalancer) checkTarget(target ws.TunnelTarget) bool {
	addr := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// GetHealthStatus 获取所有目标的健康状态
func (lb *LoadBalancer) GetHealthStatus() map[string]bool {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	status := make(map[string]bool)
	for k, v := range lb.healthStatus {
		status[k] = v
	}
	return status
}

// GetHealthyCount 获取健康目标数量
func (lb *LoadBalancer) GetHealthyCount() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	count := 0
	for _, healthy := range lb.healthStatus {
		if healthy {
			count++
		}
	}
	return count
}
