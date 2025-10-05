package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	dbinit "gkipass/plane/db/init"

	"github.com/redis/go-redis/v9"
)

// RedisCache Redis缓存客户端
type RedisCache struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisCache 创建新的Redis缓存客户端
func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	ctx := context.Background()

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		ctx:    ctx,
	}, nil
}

// Close 关闭Redis连接
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// === Session 操作 ===

// SetSession 设置会话
func (r *RedisCache) SetSession(token string, session *dbinit.Session, ttl time.Duration) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := fmt.Sprintf("session:%s", token)
	return r.client.Set(r.ctx, key, data, ttl).Err()
}

// GetSession 获取会话
func (r *RedisCache) GetSession(token string) (*dbinit.Session, error) {
	key := fmt.Sprintf("session:%s", token)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	session := &dbinit.Session{}
	if err := json.Unmarshal(data, session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return session, nil
}

// DeleteSession 删除会话
func (r *RedisCache) DeleteSession(token string) error {
	key := fmt.Sprintf("session:%s", token)
	return r.client.Del(r.ctx, key).Err()
}

// === NodeStatus 操作 ===

// SetNodeStatus 设置节点状态
func (r *RedisCache) SetNodeStatus(nodeID string, status *dbinit.NodeStatus) error {
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal node status: %w", err)
	}

	key := fmt.Sprintf("node:status:%s", nodeID)
	// 节点状态缓存30秒（心跳间隔）
	return r.client.Set(r.ctx, key, data, 30*time.Second).Err()
}

// GetNodeStatus 获取节点状态
func (r *RedisCache) GetNodeStatus(nodeID string) (*dbinit.NodeStatus, error) {
	key := fmt.Sprintf("node:status:%s", nodeID)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	status := &dbinit.NodeStatus{}
	if err := json.Unmarshal(data, status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node status: %w", err)
	}

	return status, nil
}

// GetAllNodeStatus 获取所有节点状态
func (r *RedisCache) GetAllNodeStatus() (map[string]*dbinit.NodeStatus, error) {
	pattern := "node:status:*"
	iter := r.client.Scan(r.ctx, 0, pattern, 100).Iterator()

	statuses := make(map[string]*dbinit.NodeStatus)
	for iter.Next(r.ctx) {
		key := iter.Val()
		data, err := r.client.Get(r.ctx, key).Bytes()
		if err != nil {
			continue
		}

		status := &dbinit.NodeStatus{}
		if err := json.Unmarshal(data, status); err != nil {
			continue
		}

		statuses[status.NodeID] = status
	}

	return statuses, iter.Err()
}

// === 实时统计缓存 ===

// IncrementTraffic 增加流量统计（原子操作）
func (r *RedisCache) IncrementTraffic(nodeID string, bytesIn, bytesOut int64) error {
	pipe := r.client.Pipeline()

	keyIn := fmt.Sprintf("traffic:%s:in", nodeID)
	keyOut := fmt.Sprintf("traffic:%s:out", nodeID)

	pipe.IncrBy(r.ctx, keyIn, bytesIn)
	pipe.IncrBy(r.ctx, keyOut, bytesOut)

	// 设置过期时间（1小时）
	pipe.Expire(r.ctx, keyIn, time.Hour)
	pipe.Expire(r.ctx, keyOut, time.Hour)

	_, err := pipe.Exec(r.ctx)
	return err
}

// GetTraffic 获取流量统计
func (r *RedisCache) GetTraffic(nodeID string) (bytesIn, bytesOut int64, err error) {
	keyIn := fmt.Sprintf("traffic:%s:in", nodeID)
	keyOut := fmt.Sprintf("traffic:%s:out", nodeID)

	pipe := r.client.Pipeline()
	cmdIn := pipe.Get(r.ctx, keyIn)
	cmdOut := pipe.Get(r.ctx, keyOut)

	_, err = pipe.Exec(r.ctx)
	if err != nil && err != redis.Nil {
		return 0, 0, err
	}

	bytesIn, _ = cmdIn.Int64()
	bytesOut, _ = cmdOut.Int64()

	return bytesIn, bytesOut, nil
}

// === 策略分发缓存 ===

// SetPolicyForNode 为节点设置策略列表
func (r *RedisCache) SetPolicyForNode(nodeID string, policyIDs []string) error {
	key := fmt.Sprintf("node:policies:%s", nodeID)
	data, err := json.Marshal(policyIDs)
	if err != nil {
		return err
	}

	// 策略缓存5分钟
	return r.client.Set(r.ctx, key, data, 5*time.Minute).Err()
}

// GetPolicyForNode 获取节点的策略列表
func (r *RedisCache) GetPolicyForNode(nodeID string) ([]string, error) {
	key := fmt.Sprintf("node:policies:%s", nodeID)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var policyIDs []string
	if err := json.Unmarshal(data, &policyIDs); err != nil {
		return nil, err
	}

	return policyIDs, nil
}

// InvalidatePolicyCache 使策略缓存失效
func (r *RedisCache) InvalidatePolicyCache(nodeID string) error {
	key := fmt.Sprintf("node:policies:%s", nodeID)
	return r.client.Del(r.ctx, key).Err()
}

// === 通用缓存操作 ===

// Set 设置缓存
func (r *RedisCache) Set(key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(r.ctx, key, data, ttl).Err()
}

// Get 获取缓存
func (r *RedisCache) Get(key string, dest interface{}) error {
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Delete 删除缓存
func (r *RedisCache) Delete(key string) error {
	return r.client.Del(r.ctx, key).Err()
}

// Exists 检查缓存是否存在
func (r *RedisCache) Exists(key string) (bool, error) {
	n, err := r.client.Exists(r.ctx, key).Result()
	return n > 0, err
}

// SetWithExpire 设置带过期时间的字符串值
func (r *RedisCache) SetWithExpire(key, value string, ttl time.Duration) error {
	return r.client.Set(r.ctx, key, value, ttl).Err()
}

// GetString 获取字符串值
func (r *RedisCache) GetString(key string) (string, error) {
	val, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}
