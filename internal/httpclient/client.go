// Package httpclient 提供 HTTP 客户端功能
// 使用 resty 库封装 HTTP 请求，支持重试、超时和 context 取消
package httpclient

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"pipetGo/config"
	"pipetGo/internal/logger"
)

// Client 是全局 HTTP 客户端实例
var Client *resty.Client

// InitClient 初始化 HTTP 客户端
// 配置基础URL、超时时间和重试策略
func InitClient() {
	cfg := config.GetConfig()

	logger.Debug("初始化 HTTP 客户端",
		zap.String("基础URL", cfg.Target.BaseURL),
		zap.Int("超时秒", cfg.Target.Timeout),
		zap.Bool("跳过TLS验证", cfg.HTTP.InsecureSkipVerify),
		zap.Int("重试次数", 3),
		zap.Int("重试等待秒", 1),
		zap.Int("最大重试等待秒", 5))

	client := resty.New().
		SetBaseURL(cfg.Target.BaseURL).
		SetTimeout(time.Duration(cfg.Target.Timeout) * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	client.OnBeforeRequest(func(c *resty.Client, req *resty.Request) error {
		req.Header.Set("X-Debug-Request-Time", time.Now().Format(time.RFC3339Nano))
		return nil
	})

	client.OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
		logger.Debug("HTTP 响应后处理",
			zap.String("请求方法", resp.Request.Method),
			zap.String("请求URL", resp.Request.URL),
			zap.Int("状态码", resp.StatusCode()),
			zap.Int64("响应大小", resp.Size()),
			zap.Duration("响应耗时", resp.Time()))
		return nil
	})

	if cfg.HTTP.InsecureSkipVerify {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		logger.Debug("已启用 TLS 跳过验证 (InsecureSkipVerify=true)")
	}

	Client = client
	logger.Info("HTTP 客户端初始化完成",
		zap.String("基础URL", cfg.Target.BaseURL),
		zap.Duration("超时", time.Duration(cfg.Target.Timeout)*time.Second))
}

// NewRequestWithContext 创建带 context 的 HTTP 请求
// ctx: 用于超时、取消
// 返回: resty.Request
func NewRequestWithContext(ctx context.Context) *resty.Request {
	if Client == nil {
		logger.Debug("HTTP 客户端未初始化，调用 InitClient")
		InitClient()
	}
	req := Client.R().SetContext(ctx)
	logger.Debug("创建 HTTP 请求",
		zap.String("请求方法", req.Method),
		zap.String("请求URL", req.URL),
		zap.Int("请求头数", len(req.Header)),
		zap.Int("查询参数数", len(req.QueryParam))))
	return req
}

// NewRequest 创建 HTTP 请求（使用 background context）
func NewRequest() *resty.Request {
	return NewRequestWithContext(context.Background())
}