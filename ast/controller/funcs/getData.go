package funcs

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"ast/controller/SendAndWrite"
	"ast/controller/createReq"
	"ast/job"
	"ast/model"
)

// Workers 接收压测配置，构造请求后异步启动压测任务，立刻返回任务 ID。
// 进度与结果通过 GET /jobs/:id 查询。
func Workers() gin.HandlerFunc {
	return func(c *gin.Context) {
		var apiConfig model.TestConfig

		if err := c.ShouldBindJSON(&apiConfig); err != nil {
			log.Println("数据解析失败:", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": err.Error(),
			})
			return
		}

		// 按配置构造请求（SendAndWrite 内部会克隆副本，原请求保留）
		req, err := createReq.Create(apiConfig)
		if err != nil {
			log.Println("请求构造失败:", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": err.Error(),
			})
			return
		}

		j, err := SendAndWrite.CreateChanAndFile(req, apiConfig)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, job.ErrTooManyJobs) {
				status = http.StatusTooManyRequests
			}
			log.Println("启动压测任务失败:", err.Error())
			c.JSON(status, gin.H{
				"code":  status,
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"code":   http.StatusAccepted,
			"jobId":  j.ID,
			"status": j.Snapshot().Status,
			"total":  j.Total,
			"dir":    j.Dir,
		})
	}
}
