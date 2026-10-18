package funcs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ast/job"
)

// JobDetail 查询单个压测任务的进度与最终结果。
// 任务结束后 summary 字段会带上汇总统计（与 summary.json 内容一致）。
func JobDetail() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		j, ok := job.Default.Get(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"code":  http.StatusNotFound,
				"error": "任务不存在: " + id,
			})
			return
		}

		c.JSON(http.StatusOK, j.Snapshot())
	}
}

// CancelJob 取消一个正在执行的压测任务（DELETE /jobs/:id）。
//
// 取消是「协作式」的：只发出取消信号，在飞请求返回后任务自行收敛，
// 期间状态仍是 running，前端可通过 GET /jobs/:id 轮询到 canceled。
// 已经产生的记录照常落盘，summary.json 也会写出。
func CancelJob() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		j, ok := job.Default.Get(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"code":  http.StatusNotFound,
				"error": "任务不存在: " + id,
			})
			return
		}

		if !j.IsRunning() {
			c.JSON(http.StatusConflict, gin.H{
				"code":   http.StatusConflict,
				"error":  "任务已结束，无法取消",
				"status": j.Snapshot().Status,
			})
			return
		}

		j.Cancel()

		c.JSON(http.StatusAccepted, gin.H{
			"code":    http.StatusAccepted,
			"jobId":   id,
			"status":  j.Snapshot().Status,
			"message": "已发送取消信号，任务将在在飞请求结束后停止",
		})
	}
}

// JobList 列出内存中保留的任务，按创建时间从新到旧
func JobList() gin.HandlerFunc {
	return func(c *gin.Context) {
		list := job.Default.List()

		snapshots := make([]job.Snapshot, 0, len(list))
		for _, j := range list {
			snapshots = append(snapshots, j.Snapshot())
		}

		c.JSON(http.StatusOK, gin.H{
			"code": http.StatusOK,
			"jobs": snapshots,
		})
	}
}
