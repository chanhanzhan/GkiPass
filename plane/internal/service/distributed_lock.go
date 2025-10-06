package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type DistributedLock struct {
	redis      *redis.Client
	key        string
	value      string
	expiration time.Duration
	ctx        context.Context
}

type LockManager struct {
	redis *redis.Client
}

func NewLockManager(redis *redis.Client) *LockManager {
	return &LockManager{redis: redis}
}

func (lm *LockManager) NewLock(ctx context.Context, key string, expiration time.Duration) *DistributedLock {
	if expiration == 0 {
		expiration = 30 * time.Second
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 如果随机数生成失败，使用时间戳作为备选方案
		b = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return &DistributedLock{
		redis:      lm.redis,
		key:        fmt.Sprintf("lock:%s", key),
		value:      hex.EncodeToString(b),
		expiration: expiration,
		ctx:        ctx,
	}
}

func (dl *DistributedLock) Lock() error {
	for i := 0; i < 10; i++ {
		success, err := dl.redis.SetNX(dl.ctx, dl.key, dl.value, dl.expiration).Result()
		if err != nil {
			return err
		}
		if success {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("lock timeout")
}

func (dl *DistributedLock) Unlock() error {
	script := `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
	_, err := dl.redis.Eval(dl.ctx, script, []string{dl.key}, dl.value).Result()
	return err
}

func (lm *LockManager) WithLock(ctx context.Context, key string, expiration time.Duration, fn func() error) error {
	lock := lm.NewLock(ctx, key, expiration)
	if err := lock.Lock(); err != nil {
		return err
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			// 记录解锁错误，但不影响主流程
			fmt.Printf("failed to unlock: %v\n", err)
		}
	}()
	return fn()
}
