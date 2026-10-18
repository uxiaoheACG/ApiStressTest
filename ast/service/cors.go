package service

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// Cors 允许桌面端与本地开发页面跨域调用接口。
//
// 与「放行所有来源」不同，这里只认本机来源：
//   - Origin 为空：curl / 服务端调用，本来就不受同源策略限制
//   - "null"：Electron 以 file:// 加载页面时浏览器给出的 Origin
//   - http(s)://localhost、http(s)://127.0.0.1、http(s)://[::1]（任意端口）：前端开发服务器
//
// 其它来源不会收到 Access-Control-Allow-Origin，浏览器会拦下请求，
// 避免用户随便打开一个网页就被恶意脚本拿本机压测服务当跳板。
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" {
			if !isLocalOrigin(origin) {
				c.Next()
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			c.Header("Access-Control-Max-Age", "600")
			c.Header("Vary", "Origin")
		}

		// 预检请求直接返回 204，不进入业务路由
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isLocalOrigin 判断来源是否为本机页面（含 Electron 的 "null" 来源）。
func isLocalOrigin(origin string) bool {
	if origin == "null" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
