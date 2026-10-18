package funcs

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ast/job"
)

// recordsMaxLimit 单次返回的最大明细条数，避免一次拉取过多导致响应体过大
const recordsMaxLimit = 500

// recordsResponse 明细查询响应：公共字段 + 分页结果（job.RecordPage 内嵌展开）
type recordsResponse struct {
	Code  int    `json:"code"`
	JobID string `json:"jobId"`
	Dir   string `json:"dir"`
	job.RecordPage
}

// Records 分页读取一次压测任务的结果明细（GET /jobs/:id/records）。
//
// 查询参数：
//   - kind: success / failures（默认 failures），对应任务目录下的 success.jsonl / failures.jsonl
//   - offset: 起始行号，从 0 开始（默认 0）
//   - limit: 返回条数（默认 100，上限 500）
//
// 数据直接来自磁盘文件，所以进程重启后依然能查（只要结果目录还在），
// 任务还在跑的时候也能读到已经落盘的记录。
func Records() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !job.ValidJobID(id) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "非法的任务 ID: " + id + "（应为 8 位十六进制）",
			})
			return
		}

		kind := strings.ToLower(strings.TrimSpace(c.DefaultQuery("kind", "failures")))
		if kind != "success" && kind != "failure" && kind != "failures" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":  http.StatusBadRequest,
				"error": "kind 只支持 success / failures，收到: " + kind,
			})
			return
		}

		offset, err := queryInt(c, "offset", 0)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "error": err.Error()})
			return
		}

		limit, err := queryInt(c, "limit", 100)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "error": err.Error()})
			return
		}
		if limit <= 0 {
			limit = 100
		}
		if limit > recordsMaxLimit {
			limit = recordsMaxLimit
		}

		dir, ok := job.FindDir(job.DataRoot, id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"code":  http.StatusNotFound,
				"error": "找不到任务结果目录: " + id,
			})
			return
		}

		page, err := job.LoadRecords(dir, kind, offset, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":  http.StatusInternalServerError,
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, recordsResponse{
			Code:       http.StatusOK,
			JobID:      id,
			Dir:        dir,
			RecordPage: page,
		})
	}
}

// queryInt 读取非负整数查询参数，缺省或为空时返回 def。
func queryInt(c *gin.Context, key string, def int) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, &queryError{key: key, value: raw}
	}
	return n, nil
}

// queryError 查询参数非法时的错误，文案与 gin.H 返回保持一致
type queryError struct {
	key   string
	value string
}

func (e *queryError) Error() string {
	return e.key + " 需为非负整数，收到: " + e.value
}
