package funcs

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gin-gonic/gin"

	"ast/analysis"
	"ast/job"
)

// sampleFilePattern 样本文件名：sha256 前 8 字节的十六进制 + .json，
// 严格校验避免用文件名穿越目录
var sampleFilePattern = regexp.MustCompile(`^[0-9a-f]{16}\.json$`)

// Analysis 返回一次压测任务的「返回类型」分析结果（GET /jobs/:id/analysis）。
//
// 数据来自任务目录下的 result.db（SQLite）：
//   - 每个返回类型一行：HTTP 状态码、业务码、分类、错误归类、命中次数、首次/末次时间、样本文件名
//   - 所有类型的命中次数之和等于该任务已完成的请求数
//
// 旧版本任务没有 result.db，返回 404，前端会退化到原来的汇总 + 明细展示。
func Analysis() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !job.ValidJobID(id) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "非法的任务 ID: " + id + "（应为 8 位十六进制）",
			})
			return
		}

		dir, ok := job.FindDir(job.DataRoot, id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"code":  http.StatusNotFound,
				"error": "找不到任务结果目录: " + id,
			})
			return
		}

		report, err := analysis.Load(dir, id)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"code":  http.StatusNotFound,
					"error": "该任务没有类型分析数据（可能是旧版本产生的任务）",
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
			"dir":    dir,
			"report": report,
		})
	}
}

// Sample 返回某个返回类型的样本原文（GET /jobs/:id/samples/:file）。
//
// 样本是一份完整的请求/响应快照（响应体最多 4 KiB，凭证头已打码），
// 每个返回类型只保留首次出现的那一份。
func Sample() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !job.ValidJobID(id) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "非法的任务 ID: " + id,
			})
			return
		}

		file := c.Param("file")
		if !sampleFilePattern.MatchString(file) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "非法的样本文件名: " + file,
			})
			return
		}

		dir, ok := job.FindDir(job.DataRoot, id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"code":  http.StatusNotFound,
				"error": "找不到任务结果目录: " + id,
			})
			return
		}

		path := filepath.Join(dir, "samples", file)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"code":  http.StatusNotFound,
					"error": "样本文件不存在: " + file,
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":  http.StatusInternalServerError,
				"error": err.Error(),
			})
			return
		}

		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	}
}
