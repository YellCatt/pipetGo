// Package httpclient 提供 HTTP 客户端功能
// 使用 resty 库封装 HTTP 请求，支持重试、超时和 context 取消
package httpclient

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/go-resty/resty/v2"

	"pipetGo/config"
)

// Client 是全局 HTTP 客户端实例
var Client *resty.Client

// InitClient 初始化 HTTP 客户端
// 配置基础URL、超时时间和重试策略
func InitClient() {
	cfg := config.GetConfig()

	client := resty.New().
		SetBaseURL(cfg.Target.BaseURL).
		SetTimeout(time.Duration(cfg.Target.Timeout) * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	if cfg.HTTP.InsecureSkipVerify {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	Client = client
}

// NewRequestWithContext 创建带 context 的 HTTP 请求
// ctx: 用于超时、取消
// 返回: resty.Request
func NewRequestWithContext(ctx context.Context) *resty.Request {
	if Client == nil {
		InitClient()
	}
	return Client.R().SetContext(ctx)
}

// NewRequest 创建 HTTP 请求（使用 background context）
func NewRequest() *resty.Request {
	return NewRequestWithContext(context.Background())
}
