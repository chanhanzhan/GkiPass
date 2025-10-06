package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type CacheService struct {
	redis *redis.Client
	ctx   context.Context
}

func NewCacheService(redis *redis.Client) *CacheService {
	return &CacheService{
		redis: redis,
		ctx:   context.Background(),
	}
}

func (cs *CacheService) Get(key string, dest interface{}) error {
	val, err := cs.redis.Get(cs.ctx, key).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

func (cs *CacheService) Set(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return cs.redis.Set(cs.ctx, key, string(data), expiration).Err()
}

func (cs *CacheService) Delete(key string) error {
	return cs.redis.Del(cs.ctx, key).Err()
}

func (cs *CacheService) GetOrSet(key string, dest interface{}, expiration time.Duration, loader func() (interface{}, error)) error {
	if err := cs.Get(key, dest); err == nil {
		return nil
	}

	value, err := loader()
	if err != nil {
		return err
	}

	if err := cs.Set(key, value, expiration); err != nil {
		return err
	}

	data, _ := json.Marshal(value)
	return json.Unmarshal(data, dest)
}
