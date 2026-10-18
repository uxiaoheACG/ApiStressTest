package newCLient

import (
	"ast/model"
	"net/http"
	"time"
)

func NewClient(config model.ConcurrencyConfig) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        2 * config.Count, //最多闲置
		MaxIdleConnsPerHost: config.Count,     //最多并发
		MaxConnsPerHost:     1500,
		IdleConnTimeout:     60 * time.Second, //一个连接超过60s不用就回收
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(config.Timeout) * time.Millisecond, //单个请求的超时机制
	}
}
