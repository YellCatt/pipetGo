package httpclient

import (
	"time"

	"github.com/go-resty/resty/v2"

	"pipet/config"
)

var Client *resty.Client

func InitClient() {
	Client = resty.New().
		SetBaseURL(config.AppConfig.Target.BaseURL).
		SetTimeout(time.Duration(config.AppConfig.Target.Timeout) * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)
}
