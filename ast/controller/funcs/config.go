package funcs

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"ast/job"
)

// TestConfig 返回一次压测任务使用的完整配置（GET /jobs/:id/config）。
//
// 数据来自磁盘上的 config_test/<jobID>.json，所以进程重启后依然可查；
// 历史页据此展示当次请求的参数，并支持一键「重新测试」。
func TestConfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !job.ValidJobID(id) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "非法的任务 ID: " + id + "（应为 8 位十六进制）",
			})
			return
		}

		raw, err := job.LoadTestConfig(id)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"code":  http.StatusNotFound,
					"error": "该任务没有配置存档（可能是早期版本产生的任务，或存档已被删除）",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":  http.StatusInternalServerError,
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":   http.StatusOK,
			"jobId":  id,
			"config": raw,
		})
	}
}
