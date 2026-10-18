package SendAndWrite

import (
	"context"
	"io"
	"net/http"
	"time"

	"ast/job"
)

// failureInput 组装一条失败记录所需的全部信息
type failureInput struct {
	index     int
	start     time.Time
	kind      string // unsuccess / timeout / error / success
	fail      job.FailureKind
	err       error
	snap      reqSnapshot
	resp      *http.Response
	body      []byte
	size      int64
	truncated bool
}

// Do 发送一条请求，并按响应状态把结果写入 success.jsonl 或 failures.jsonl。
//
// 成功（2xx）只写简写记录；超时、其它网络错误、非 2xx 响应写详细记录（含请求快照与响应头体）。
// 该方法可被任意多个 goroutine 并发调用：内部只做原子计数与 channel 投递，不直接操作文件。
func (s *Sink) Do(ctx context.Context, index int, tmpl *http.Request, snap reqSnapshot, client *http.Client) {
	start := time.Now()

	req, err := deriveRequest(ctx, tmpl)
	if err != nil {
		s.recordFailure(failureInput{
			index: index, start: start, kind: "error", fail: job.FailError, err: err, snap: snap,
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		kind, fail := "error", job.FailError
		if errKindOf(err) == "deadline" {
			kind, fail = "timeout", job.FailTimeout
		}
		s.recordFailure(failureInput{
			index: index, start: start, kind: kind, fail: fail, err: err, snap: snap,
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		s.recordSuccess(index, start, resp, snap)
		return
	}

	// 非 2xx：保留响应体开头部分用于详细记录，剩余读尽以便连接复用
	body, truncated, size, readErr := readResponseBody(resp.Body)
	s.recordFailure(failureInput{
		index: index, start: start, kind: "unsuccess", fail: job.FailUnsuccess,
		err: readErr, snap: snap, resp: resp, body: body, size: size, truncated: truncated,
	})
}

// recordSuccess 记录一次成功响应
func (s *Sink) recordSuccess(index int, start time.Time, resp *http.Response, snap reqSnapshot) {
	// SaveData=3：成功也写详细记录（含请求/响应体）
	if s.detailSuccess() {
		body, truncated, size, _ := readResponseBody(resp.Body)
		ms := time.Since(start).Milliseconds()

		s.j.AddSuccess(resp.StatusCode)
		s.setLatency(index, ms)
		s.Push(kindSuccess, marshalLine(s.detailedRecord(failureInput{
			index: index, start: start, kind: "success", snap: snap,
			resp: resp, body: body, size: size, truncated: truncated,
		}, ms)))
		return
	}

	// 其它模式：只需要字节数，读完即丢，保证连接可复用
	size, _ := io.Copy(io.Discard, resp.Body)
	ms := time.Since(start).Milliseconds()

	s.j.AddSuccess(resp.StatusCode)
	s.setLatency(index, ms)

	if !s.needFile(kindSuccess) {
		return
	}
	s.Push(kindSuccess, marshalLine(successRecord{
		Index:  index,
		Status: resp.StatusCode,
		Ms:     ms,
		Size:   size,
		At:     recordTime(start),
	}))
}

// recordFailure 记录一次失败/超时响应
func (s *Sink) recordFailure(in failureInput) {
	ms := time.Since(in.start).Milliseconds()

	code := 0
	if in.resp != nil {
		code = in.resp.StatusCode
	}
	s.j.AddFailure(in.fail, code)
	s.setLatency(in.index, ms)

	if !s.needFile(kindFailure) {
		return
	}
	s.Push(kindFailure, marshalLine(s.detailedRecord(in, ms)))
}

// detailedRecord 组装一条详细记录（失败/超时，以及 SaveData=3 时的成功）
func (s *Sink) detailedRecord(in failureInput, ms int64) failureRecord {
	rec := failureRecord{
		Index:            in.index,
		Kind:             in.kind,
		ErrKind:          errKindOf(in.err),
		Method:           in.snap.Method,
		URL:              in.snap.URL,
		At:               recordTime(in.start),
		Ms:               ms,
		ReqHeader:        in.snap.Header,
		ReqBody:          in.snap.Body,
		ReqBodyTruncated: in.snap.BodyTruncated,
		RespBytes:        in.size,
	}
	if in.err != nil {
		rec.Err = in.err.Error()
	}
	if in.resp != nil {
		rec.Status = in.resp.StatusCode
		rec.RespHeader = headerMap(in.resp.Header)
	}
	if len(in.body) > 0 {
		rec.RespBody = string(in.body)
	}
	rec.RespBodyTruncated = in.truncated

	return rec
}

// deriveRequest 从模板派生一条独立请求。
// Body 每次通过 GetBody 重新取，避免多个 goroutine 共用同一个 reader。
func deriveRequest(ctx context.Context, tmpl *http.Request) (*http.Request, error) {
	req := tmpl.Clone(ctx)
	if tmpl.GetBody != nil {
		body, err := tmpl.GetBody()
		if err != nil {
			return nil, err
		}
		req.Body = body
	}
	return req, nil
}
