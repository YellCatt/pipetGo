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
	logger.Debug("邮件配置已初始化",
		zap.Bool("启用", cfg.Enabled),
		zap.String("发件人", cfg.FromEmail),
		zap.Int("收件人数", len(cfg.ToEmail)),
		zap.Bool("有授权码", cfg.AuthCode != ""),
		zap.String("SMTP", cfg.SMTPServer),
		zap.Int("端口", cfg.SMTPPort),
		zap.String("设备名", cfg.DeviceName))
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

	logger.Debug("准备发送邮件",
		zap.String("主题", subject),
		zap.Int("正文长度", len(body)),
		zap.String("收件人", toEmails))

	logger.Info("开始连接 SMTP 服务器",
		zap.String("SMTP服务器", cfg.SMTPServer),
		zap.Int("SMTP端口", cfg.SMTPPort),
		zap.String("发件人", cfg.FromEmail),
		zap.Int("收件人数", len(cfg.ToEmail)))

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         cfg.SMTPServer,
	}

	logger.Debug("开始 TLS 拨号", zap.String("地址", addr))
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		logger.Error("TLS 连接 SMTP 服务器失败",
			zap.String("地址", addr),
			zap.Error(err))
		return fmt.Errorf("TLS 连接失败(%s): %w", addr, err)
	}
	defer conn.Close()
	logger.Debug("TLS 连接建立成功")

	logger.Debug("创建 SMTP 客户端")
	client, err := smtp.NewClient(conn, cfg.SMTPServer)
	if err != nil {
		logger.Error("创建 SMTP 客户端失败", zap.Error(err))
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()
	logger.Debug("SMTP 客户端创建成功")

	logger.Debug("开始 SMTP 认证", zap.String("用户", cfg.FromEmail))
	if err := client.Auth(auth); err != nil {
		logger.Error("SMTP 认证失败",
			zap.String("发件人", cfg.FromEmail),
			zap.Error(err))
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	logger.Debug("SMTP 认证成功")

	logger.Debug("设置发件人", zap.String("发件人", cfg.FromEmail))
	if err := client.Mail(cfg.FromEmail); err != nil {
		logger.Error("设置发件人失败",
			zap.String("发件人", cfg.FromEmail),
			zap.Error(err))
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	logger.Debug("发件人设置成功")

	for _, to := range cfg.ToEmail {
		logger.Debug("设置收件人", zap.String("收件人", to))
		if err := client.Rcpt(to); err != nil {
			logger.Error("设置收件人失败",
				zap.String("收件人", to),
				zap.Error(err))
			return fmt.Errorf("设置收件人 %s 失败: %w", to, err)
		}
		logger.Debug("收件人设置成功", zap.String("收件人", to))
	}

	logger.Debug("获取数据写入器")
	w, err := client.Data()
	if err != nil {
		logger.Error("获取数据写入器失败", zap.Error(err))
		return fmt.Errorf("获取数据写入器失败: %w", err)
	}

	logger.Debug("写入邮件内容", zap.Int("字节数", len(msg)))
	_, err = w.Write(msg)
	if err != nil {
		logger.Error("写入邮件内容失败", zap.Error(err))
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}

	logger.Debug("关闭数据写入器")
	err = w.Close()
	if err != nil {
		logger.Error("关闭数据写入器失败", zap.Error(err))
		return fmt.Errorf("关闭数据写入器失败: %w", err)
	}

	logger.Debug("关闭 SMTP 客户端连接")
	if err := client.Quit(); err != nil {
		logger.Warn("SMTP 客户端关闭警告", zap.Error(err))
	}

	logger.Info("邮件发送成功",
		zap.String("主题", subject),
		zap.String("收件人", toEmails))
	return nil
}