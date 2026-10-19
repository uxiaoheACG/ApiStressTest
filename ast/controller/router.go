package controller

import (
	"ast/controller/funcs"
	"github.com/gin-gonic/gin"
	"log"
)

func RegisterRouter(r *gin.Engine) {
	log.Println("路由注册")

	r.GET("/", funcs.Health())

	// 启动压测任务：立刻返回 202 + jobId
	r.POST("/getData", funcs.Workers())

	// 任务进度与结果查询（内存态，进程重启后清空）
	r.GET("/jobs", funcs.JobList())
	r.GET("/jobs/:id", funcs.JobDetail())

	// 取消正在执行的任务（协作式取消，202 表示信号已发出）
	r.DELETE("/jobs/:id", funcs.CancelJob())

	// 结果明细分页读取（success.jsonl / failures.jsonl，数据来自磁盘）
	r.GET("/jobs/:id/records", funcs.Records())

	// 当次压测的完整参数（config_test/<jobID>.json），供历史页展示与重新测试
	r.GET("/jobs/:id/config", funcs.TestConfig())

	// 返回类型分析（任务目录下的 result.db）与类型样本
	r.GET("/jobs/:id/analysis", funcs.Analysis())
	r.GET("/jobs/:id/samples/:file", funcs.Sample())

	// 删除任务记录（?purge=true 时连同 Data 目录与 config_test 存档一起删除）
	// 注意：上面的 DELETE /jobs/:id 是「取消运行中的任务」，这里才是「删除记录」
	r.DELETE("/jobs/:id/record", funcs.DeleteJobRecord())

	// 历史任务查询（扫描 Data 目录，重启后仍可查）
	r.GET("/history", funcs.History())
}
