package job

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Status 压测任务的状态
type Status int32

const (
	StatusPending   Status = iota // 已创建，尚未开始派发
	StatusRunning                 // 压测进行中
	StatusCompleted               // 全部请求执行完成
	StatusFailed                  // 任务异常结束（结果文件写失败、panic 等）
	StatusCanceled                // 被取消（主动取消或服务退出）
)

// String 返回状态名，用于接口返回与日志
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// FailureKind 失败类型，用于分类计数
type FailureKind int32

const (
	FailUnsuccess FailureKind = iota // 收到响应但状态码非 2xx
	FailTimeout                      // 超时（连接超时、读取超时）
	FailError                        // 其它网络 / 协议错误
)

// Latency 耗时统计，单位毫秒
type Latency struct {
	Min int64 `json:"min"`
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	P99 int64 `json:"p99"`
	Max int64 `json:"max"`
}

// Summary 任务结束后的汇总结果，内容与 summary.json 一致
type Summary struct {
	JobID       string        `json:"jobId"`
	Total       int           `json:"total"`
	Success     int64         `json:"success"`
	Failure     int64         `json:"failure"`
	Timeout     int64         `json:"timeout"`
	Error       int64         `json:"error"`
	Unsuccess   int64         `json:"unsuccess"`
	StatusCodes map[int]int64 `json:"statusCodes,omitempty"`
	QPS         float64       `json:"qps"`
	ElapsedMs   int64         `json:"elapsedMs"`
	Latency     Latency       `json:"latency"`
	// TypeCount 本次任务出现过的返回类型数（jobId + 状态码 + 业务码 去重后）
	TypeCount int `json:"typeCount,omitempty"`
}

// Progress 任务的实时进度
type Progress struct {
	Done        int64         `json:"done"`
	Total       int           `json:"total"`
	Percent     float64       `json:"percent"`
	Success     int64         `json:"success"`
	Failure     int64         `json:"failure"`
	Timeout     int64         `json:"timeout"`
	Error       int64         `json:"error"`
	Unsuccess   int64         `json:"unsuccess"`
	QPS         float64       `json:"qps"`
	StatusCodes map[int]int64 `json:"statusCodes,omitempty"`
}

// Snapshot 对外暴露的任务快照（GET /jobs/:id 的响应体）
type Snapshot struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Dir       string    `json:"dir"`
	StartedAt time.Time `json:"startedAt"`
	ElapsedMs int64     `json:"elapsedMs"`
	Progress  Progress  `json:"progress"`
	Err       string    `json:"err,omitempty"`
	Summary   *Summary  `json:"summary,omitempty"`
}

// Job 一次压测任务的状态。
// 计数字段用原子操作累加，状态字段用 RWMutex 保护，可被多个 goroutine 并发读取。
type Job struct {
	ID    string
	Dir   string
	Total int

	createdAt time.Time

	done      atomic.Int64
	success   atomic.Int64
	failure   atomic.Int64
	timeout   atomic.Int64
	errCount  atomic.Int64
	unsuccess atomic.Int64
	codes     [500]atomic.Int64 // 下标 = 状态码 - 100

	mu       sync.RWMutex
	status   Status
	started  time.Time
	finished time.Time
	errMsg   string
	summary  *Summary

	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

// New 创建任务，初始状态为 pending
func New(id, dir string, total int) *Job {
	return &Job{
		ID:        id,
		Dir:       dir,
		Total:     total,
		createdAt: time.Now(),
		status:    StatusPending,
	}
}

// CreatedAt 返回任务创建时间
func (j *Job) CreatedAt() time.Time { return j.createdAt }

// Start 把任务标记为 running 并记录开始时间；重复调用无副作用
func (j *Job) Start() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status != StatusPending {
		return
	}
	j.status = StatusRunning
	j.started = time.Now()
}

// AddSuccess 记录一次成功请求（状态码 2xx）
func (j *Job) AddSuccess(code int) {
	j.success.Add(1)
	j.done.Add(1)
	j.addCode(code)
}

// AddFailure 记录一次失败请求
func (j *Job) AddFailure(kind FailureKind, code int) {
	j.failure.Add(1)
	j.done.Add(1)
	switch kind {
	case FailTimeout:
		j.timeout.Add(1)
	case FailUnsuccess:
		j.unsuccess.Add(1)
	default:
		j.errCount.Add(1)
	}
	j.addCode(code)
}

// addCode 累加状态码分布，非 HTTP 状态码（如网络错误的 0）忽略
func (j *Job) addCode(code int) {
	if code < 100 || code > 599 {
		return
	}
	j.codes[code-100].Add(1)
}

// StatusCodes 返回状态码分布快照
func (j *Job) StatusCodes() map[int]int64 {
	codes := make(map[int]int64)
	for i := range j.codes {
		if v := j.codes[i].Load(); v > 0 {
			codes[i+100] = v
		}
	}
	return codes
}

// Complete 标记任务正常结束（canceled 表示因取消而提前结束），同时保存汇总结果
func (j *Job) Complete(sum *Summary, canceled bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.terminalLocked() {
		return
	}
	if canceled {
		j.status = StatusCanceled
	} else {
		j.status = StatusCompleted
	}
	j.finished = time.Now()
	j.summary = sum
}

// Fail 标记任务异常结束
func (j *Job) Fail(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.terminalLocked() {
		return
	}
	j.status = StatusFailed
	j.finished = time.Now()
	if err != nil {
		j.errMsg = err.Error()
	}
}

// terminalLocked 判断是否已进入终态，调用方需持有 j.mu
func (j *Job) terminalLocked() bool {
	switch j.status {
	case StatusCompleted, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

// IsRunning 任务是否仍在执行（含尚未开始的 pending）
func (j *Job) IsRunning() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status == StatusPending || j.status == StatusRunning
}

// SetCancel 注册取消函数，供外部（取消接口 / 服务退出）中断任务
func (j *Job) SetCancel(cancel context.CancelFunc) {
	j.cancelMu.Lock()
	j.cancel = cancel
	j.cancelMu.Unlock()
}

// Cancel 取消任务；未注册取消函数时无副作用
func (j *Job) Cancel() {
	j.cancelMu.Lock()
	cancel := j.cancel
	j.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Snapshot 返回任务当前状态与进度的只读快照
func (j *Job) Snapshot() Snapshot {
	j.mu.RLock()
	status := j.status
	started := j.started
	finished := j.finished
	errMsg := j.errMsg
	summary := j.summary
	j.mu.RUnlock()

	if started.IsZero() {
		started = j.createdAt
	}
	end := time.Now()
	if !finished.IsZero() {
		end = finished
	}
	elapsed := end.Sub(started).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}

	done := j.done.Load()
	progress := Progress{
		Done:        done,
		Total:       j.Total,
		Success:     j.success.Load(),
		Failure:     j.failure.Load(),
		Timeout:     j.timeout.Load(),
		Error:       j.errCount.Load(),
		Unsuccess:   j.unsuccess.Load(),
		StatusCodes: j.StatusCodes(),
	}
	if j.Total > 0 {
		progress.Percent = float64(done) * 100 / float64(j.Total)
	}
	if elapsed > 0 {
		progress.QPS = float64(done) * 1000 / float64(elapsed)
	}

	return Snapshot{
		ID:        j.ID,
		Status:    status.String(),
		Dir:       j.Dir,
		StartedAt: started,
		ElapsedMs: elapsed,
		Progress:  progress,
		Err:       errMsg,
		Summary:   summary,
	}
}
