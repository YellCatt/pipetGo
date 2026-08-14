// Package email 提供邮件发送功能（纯传输层）
package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"sync"

	"pipetGo/internal/logger"

	"go.uber.org/zap"
)

type EmailConfig struct {
	Enabled    bool
	FromEmail  string
	ToEmail    []string
	AuthCode   string
	SMTPServer string
	SMTPPort   int
	DeviceName string
}

var Config EmailConfig
var configMu sync.RWMutex

func InitEmail(cfg EmailConfig) {
	configMu.Lock()
	Config = cfg
	configMu.Unlock()
}

func GetConfig() EmailConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return Config
}

func UpdateConfig(cfg EmailConfig) {
	configMu.Lock()
	Config = cfg
	configMu.Unlock()
}

func GetDeviceName() string {
	cfg := GetConfig()
	if cfg.DeviceName != "" {
		return cfg.DeviceName
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "未知设备"
	}
	return hostname
}

func SendEmail(subject, body string) error {
	cfg := GetConfig()

	toEmails := strings.Join(cfg.ToEmail, ", ")
	msg := []byte("From: " + cfg.FromEmail + "\r\n" +
		"To: " + toEmails + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", cfg.SMTPServer, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.FromEmail, cfg.AuthCode, cfg.SMTPServer)

	logger.Info("开始连接 SMTP 服务器",
		zap.String("SMTP服务器", cfg.SMTPServer),
		zap.Int("SMTP端口", cfg.SMTPPort),
		zap.String("发件人", cfg.FromEmail),
		zap.Int("收件人数", len(cfg.ToEmail)))

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         cfg.SMTPServer,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		logger.Error("TLS 连接 SMTP 服务器失败",
			zap.String("addr", addr),
			zap.Error(err))
		return fmt.Errorf("TLS 连接失败(%s): %w", addr, err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, cfg.SMTPServer)
	if err != nil {
		logger.Error("创建 SMTP 客户端失败", zap.Error(err))
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		logger.Error("SMTP 认证失败",
			zap.String("from", cfg.FromEmail),
			zap.Error(err))
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}

	if err := client.Mail(cfg.FromEmail); err != nil {
		logger.Error("设置发件人失败",
			zap.String("from", cfg.FromEmail),
			zap.Error(err))
		return fmt.Errorf("设置发件人失败: %w", err)
	}

	for _, to := range cfg.ToEmail {
		if err := client.Rcpt(to); err != nil {
			logger.Error("设置收件人失败",
				zap.String("to", to),
				zap.Error(err))
			return fmt.Errorf("设置收件人 %s 失败: %w", to, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		logger.Error("获取数据写入器失败", zap.Error(err))
		return fmt.Errorf("获取数据写入器失败: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		logger.Error("写入邮件内容失败", zap.Error(err))
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}

	err = w.Close()
	if err != nil {
		logger.Error("关闭数据写入器失败", zap.Error(err))
		return fmt.Errorf("关闭数据写入器失败: %w", err)
	}

	logger.Info("邮件发送成功",
		zap.String("subject", subject),
		zap.String("recipients", toEmails))
	return nil
}
