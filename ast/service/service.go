package service

import (
	"log"

	"github.com/gin-gonic/gin"
)

// NewEngine 注册服务器：按配置设置运行模式、通用中间件，返回可直接注册路由的 Engine。
func NewEngine(cfg Config) *gin.Engine {
	gin.SetMode(cfg.Mode)

	engine := gin.New()
	// 等价于 gin.Default()：访问日志 + panic 恢复，但顺序显式可控；
	// CORS 放在最前面，保证预检请求不会进入业务路由
	engine.Use(Cors(), gin.Logger(), gin.Recovery())

	// 服务可能直接暴露或被反向代理，默认不信任任何代理头，避免 X-Forwarded-For 被伪造
	if err := engine.SetTrustedProxies(nil); err != nil {
		log.Printf("设置可信代理失败: %v", err)
	}

	// 方法不支持时返回 405，便于调用方区分「路由不存在」与「方法用错」
	engine.HandleMethodNotAllowed = true

	return engine
}
