package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// AlertLevel 告警级别
type AlertLevel string

const (
	AlertInfo     AlertLevel = "info"
	AlertWarning  AlertLevel = "warning"
	AlertCritical AlertLevel = "critical"
)

// Alert 告警
type Alert struct {
	Level   AlertLevel
	Title   string
	Message string
	Tags    []string
}

// AlertSystem 告警系统
type AlertSystem struct {
	emailConfig    *EmailConfig
	webhookURL     string
	telegramConfig *TelegramConfig
	mu             sync.RWMutex
}

type EmailConfig struct {
	SMTPHost string
	SMTPPort int
	Username string
	Password string
	To       []string
}

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// NewAlertSystem 创建告警系统
func NewAlertSystem() *AlertSystem {
	return &AlertSystem{}
}

// SetWebhook 设置Webhook URL
func (as *AlertSystem) SetWebhook(url string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.webhookURL = url
}

// SetTelegram 设置Telegram配置
func (as *AlertSystem) SetTelegram(config *TelegramConfig) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.telegramConfig = config
}

// Send 发送告警
func (as *AlertSystem) Send(alert Alert) error {
	as.mu.RLock()
	defer as.mu.RUnlock()

	var errs []error

	// Send to Webhook
	if as.webhookURL != "" {
		if err := as.sendWebhook(alert); err != nil {
			errs = append(errs, fmt.Errorf("webhook: %w", err))
		}
	}

	// Send to Telegram
	if as.telegramConfig != nil {
		if err := as.sendTelegram(alert); err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("alert errors: %v", errs)
	}

	return nil
}

func (as *AlertSystem) sendWebhook(alert Alert) error {
	data, err := json.Marshal(map[string]interface{}{
		"level":   alert.Level,
		"title":   alert.Title,
		"message": alert.Message,
		"tags":    alert.Tags,
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(as.webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (as *AlertSystem) sendTelegram(alert Alert) error {
	emoji := "ℹ️"
	switch alert.Level {
	case AlertWarning:
		emoji = "⚠️"
	case AlertCritical:
		emoji = "🚨"
	}

	text := fmt.Sprintf("%s *%s*\n\n%s", emoji, alert.Title, alert.Message)

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", as.telegramConfig.BotToken)
	data, _ := json.Marshal(map[string]interface{}{
		"chat_id":    as.telegramConfig.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}



















