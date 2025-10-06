package core

import (
	"testing"
	"time"
)

// ExponentialBackoff 计算指数退避的重连间隔
func ExponentialBackoff(initialInterval, maxInterval time.Duration, multiplier float64, attempt int) time.Duration {
	if attempt <= 0 {
		return initialInterval
	}
	interval := float64(initialInterval)
	for i := 0; i < attempt; i++ {
		interval *= multiplier
	}
	if time.Duration(interval) > maxInterval {
		return maxInterval
	}
	return time.Duration(interval)
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{"第1次重连", 0, 1 * time.Second},
		{"第2次重连", 1, 2 * time.Second},
		{"第3次重连", 2, 4 * time.Second},
		{"第4次重连", 3, 8 * time.Second},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			result := ExponentialBackoff(1*time.Second, 30*time.Second, 2.0, tt.attempt)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHealthScorer(t *testing.T) {
	scorer := NewHealthScorer()

	// 记录一些成功和失败
	for i := 0; i < 90; i++ {
		scorer.RecordSuccess()
	}
	for i := 0; i < 10; i++ {
		scorer.RecordError()
	}

	scorer.RecordLatency(50 * time.Millisecond)
	scorer.SetLoad(30)

	score := scorer.CalculateScore()
	if score < 80 || score > 100 {
		t.Errorf("unexpected health score: %d", score)
	}

	metrics := scorer.GetMetrics()
	if metrics.Stability != 90 {
		t.Errorf("expected stability 90, got %d", metrics.Stability)
	}
}

func TestE2EEManager(t *testing.T) {
	tests := []struct {
		algorithm string
		plaintext string
	}{
		{"aes-gcm", "Hello, World!"},
		{"chacha20-poly1305", "Test message 123"},
	}

	for _, tt := range tests {
		t.Run(tt.algorithm, func(t *testing.T) {
			manager, err := NewE2EEManager(tt.algorithm, "test-passphrase")
			if err != nil {
				t.Fatal(err)
			}

			// 加密
			ciphertext, err := manager.Encrypt([]byte(tt.plaintext))
			if err != nil {
				t.Fatal(err)
			}

			// 解密
			decrypted, err := manager.Decrypt(ciphertext)
			if err != nil {
				t.Fatal(err)
			}

			if string(decrypted) != tt.plaintext {
				t.Errorf("expected %s, got %s", tt.plaintext, string(decrypted))
			}
		})
	}
}

func BenchmarkZeroCopyBuffer(b *testing.B) {
	data := make([]byte, 64*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := GetManagedBuffer(64 * 1024)
		copy(buf.Bytes(), data)
		buf.Release()
	}
}
