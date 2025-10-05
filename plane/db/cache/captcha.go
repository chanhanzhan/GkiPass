package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	dbinit "gkipass/plane/db/init"
)

const (
	captchaPrefix = "captcha:"
	captchaTTL    = 5 * time.Minute // 默认5分钟过期
)

// SetCaptchaSession 存储验证码会话
func (r *RedisCache) SetCaptchaSession(session *dbinit.CaptchaSession, ttl time.Duration) error {
	ctx := context.Background()
	key := captchaPrefix + session.ID

	if ttl == 0 {
		ttl = captchaTTL
	}

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal captcha session: %w", err)
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}

// GetCaptchaSession 获取验证码会话
func (r *RedisCache) GetCaptchaSession(id string) (*dbinit.CaptchaSession, error) {
	ctx := context.Background()
	key := captchaPrefix + id

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var session dbinit.CaptchaSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal captcha session: %w", err)
	}

	return &session, nil
}

// DeleteCaptchaSession 删除验证码会话
func (r *RedisCache) DeleteCaptchaSession(id string) error {
	ctx := context.Background()
	key := captchaPrefix + id
	return r.client.Del(ctx, key).Err()
}

// VerifyAndDeleteCaptcha 验证并删除验证码（一次性使用）
func (r *RedisCache) VerifyAndDeleteCaptcha(id, code string) (bool, error) {
	session, err := r.GetCaptchaSession(id)
	if err != nil {
		return false, err
	}

	if session == nil {
		return false, nil
	}

	// 检查是否过期
	if time.Now().After(session.ExpiresAt) {
		_ = r.DeleteCaptchaSession(id)
		return false, nil
	}

	// 验证码对比（不区分大小写）
	valid := session.Code == code

	// 删除已使用的验证码
	_ = r.DeleteCaptchaSession(id)

	return valid, nil
}
