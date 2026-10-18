package funcs

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ast/job"
)

// historyMaxLimit ?limit= 允许的最大条数，防止一次拉取过多历史
const historyMaxLimit = 1000

// History 查询磁盘上已落盘的历史任务（GET /history），按创建时间从新到旧。
//
// 与 GET /jobs 的区别：/jobs 只反映当前进程内存中的任务，进程重启即清空；
// /history 直接扫描 Data 目录下的 summary.json，因此能查到重启前的结果，
// 但只有汇总数据（无实时进度），且这些任务不可再取消。
//
// limit 参数：不传或 0 表示返回全部匹配项，正数表示最多返回多少条。
func History() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 0
		if raw := c.Query("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":  http.StatusBadRequest,
					"error": "limit 需为非负整数，0 表示全部: " + raw,
				})
				return
			}
			limit = n
			if limit > historyMaxLimit {
				limit = historyMaxLimit
			}
		}

		list, err := job.LoadHistory(job.DataRoot, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":  http.StatusInternalServerError,
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    http.StatusOK,
			"count":   len(list),
			"history": list,
		})
	}
}
