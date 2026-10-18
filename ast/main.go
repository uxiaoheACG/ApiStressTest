package main

import (
	"log"

	"ast/controller"
	"ast/service"
)

func main() {
	// 读取启动配置：默认值 → serverConfig.yaml → 环境变量
	cfg, err := service.LoadConfig()
	if err != nil {
		log.Fatalf("加载启动配置失败: %v", err)
	}

	// 注册服务器：设置运行模式与中间件
	server := service.NewEngine(cfg)

	// 注册路由
	controller.RegisterRouter(server)

	// 启动服务器并等待退出信号
	if err := service.Run(server, cfg); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
