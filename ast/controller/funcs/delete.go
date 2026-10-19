package funcs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ast/job"
)

// DeleteJobRecord 删除一次压测任务的记录（DELETE /jobs/:id/record）。
//
// 默认只把任务从内存列表移除（内存只保留最近 50 条，重启也会清空）；
// 带 ?purge=true 时同时删除磁盘数据：
//   - Data/<时间>_<jobID>/（result.db、samples/、summary.json）
//   - config_test/<jobID>.json（参数存档）
//
// 正在运行的任务不允许删除，需先取消（返回 409）。
// 注意与 DELETE /jobs/:id（取消任务）区分：那个是取消，这个是删除记录。
func DeleteJobRecord() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !job.ValidJobID(id) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "非法的任务 ID: " + id + "（应为 8 位十六进制）",
			})
			return
		}

		purge := c.Query("purge") == "true" || c.Query("purge") == "1"

		j, inMemory := job.Default.Get(id)
		if inMemory && j.IsRunning() {
			c.JSON(http.StatusConflict, gin.H{
				"code":  http.StatusConflict,
				"error": "任务正在运行，请先取消后再删除",
			})
			return
		}

		// 先定位目录（内存里还在时能直接拿到），再移除内存记录
		dir, hasDir := job.FindDir(job.DataRoot, id)

		if !inMemory && !hasDir {
			c.JSON(http.StatusNotFound, gin.H{
				"code":  http.StatusNotFound,
				"error": "任务不存在: " + id,
			})
			return
		}

		removedFromMemory := false
		if inMemory {
			removedFromMemory = job.Default.Remove(id)
		}

		purged := false
		if purge {
			if hasDir {
				if err := job.RemoveJobData(dir); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"code":  http.StatusInternalServerError,
						"error": err.Error(),
					})
					return
				}
			}
			if err := job.RemoveTestConfig(id); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":  http.StatusInternalServerError,
					"error": err.Error(),
				})
				return
			}
			purged = true
		}

		c.JSON(http.StatusOK, gin.H{
			"code":              http.StatusOK,
			"jobId":             id,
			"removedFromMemory": removedFromMemory,
			"purged":            purged,
			"dir":               dir,
			"message": func() string {
				if purged {
					return "任务记录与本地数据已删除"
				}
				return "任务记录已从列表移除（本地数据保留）"
			}(),
		})
	}
}
