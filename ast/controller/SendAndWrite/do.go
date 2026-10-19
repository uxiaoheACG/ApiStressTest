package SendAndWrite

import (
	"context"
	"net/http"
	"time"

	"ast/analysis"
	"ast/job"
)

// resultInput 一次请求的完整结果
type resultInput struct {
	kind      string // success / unsuccess / timeout / error
	status    int
	fail      job.FailureKind
	err       error
	snap      reqSnapshot
	resp      *http.Response
	body      []byte
	size      int64
	truncated bool
}

// Do 发送一条请求，把结果按指纹归并到聚合器。
//
// 分类规则（与界面上的「返回类型」一致）：
//
//	2xx                    → success
//	非 2xx（有响应）        → unsuccess
//	err 且归类为 deadline   → timeout
//	err 其它                → error
//
// 无论哪一类都会：读尽响应体（保证连接复用）→ 原子计数 → 内存指纹聚合；
// 只有某一类首次出现时才序列化一份样本，因此热路径上没有 JSON 序列化与磁盘 IO。
// 该方法可被任意多个 goroutine 并发调用。
func (s *Sink) Do(ctx context.Context, index int, tmpl *http.Request, snap reqSnapshot, client *http.Client) {
	start := time.Now()

	req, err := deriveRequest(ctx, tmpl)
	if err != nil {
		s.finish(index, start, resultInput{
			kind: analysis.KindError, fail: job.FailError, err: err, snap: snap,
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		kind, fail := analysis.KindError, job.FailError
		if errKindOf(err) == "deadline" {
			kind, fail = analysis.KindTimeout, job.FailTimeout
		}
		s.finish(index, start, resultInput{kind: kind, fail: fail, err: err, snap: snap})
		return
	}
	defer resp.Body.Close()

	// 不论状态码如何都保留响应体开头：成功要提取业务码，失败要存进样本；
	// 剩余部分读尽以便复用连接
	body, truncated, size, readErr := readResponseBody(resp.Body)

	in := resultInput{
		kind:      analysis.KindSuccess,
		status:    resp.StatusCode,
		snap:      snap,
		resp:      resp,
		body:      body,
		size:      size,
		truncated: truncated,
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		in.kind = analysis.KindUnsuccess
		in.fail = job.FailUnsuccess
		in.err = readErr
	}

	s.finish(index, start, in)
}

// finish 计数、记录耗时，并按指纹聚合（首次出现时生成一份样本）
func (s *Sink) finish(index int, start time.Time, in resultInput) {
	ms := time.Since(start).Milliseconds()

	if in.kind == analysis.KindSuccess {
		s.j.AddSuccess(in.status)
	} else {
		s.j.AddFailure(in.fail, in.status)
	}
	s.setLatency(index, ms)

	errKind := errKindOf(in.err)
	bizCode := s.BizCode(in.body)

	s.Record(HitInput{
		StatusCode: in.status,
		BizCode:    bizCode,
		Kind:       in.kind,
		ErrKind:    errKind,
		ReqMethod:  in.snap.Method,
		ReqURL:     in.snap.URL,
		At:         start,
	}, func(fingerprint, effectiveBiz string) []byte {
		return marshalSample(s.buildSample(fingerprint, effectiveBiz, errKind, ms, start, in))
	})
}

// buildSample 组装一份样本（只在该返回类型首次出现时调用）
func (s *Sink) buildSample(
	fingerprint, bizCode, errKind string, ms int64, at time.Time, in resultInput,
) sampleFile {
	sample := sampleFile{
		Fingerprint:       fingerprint,
		Kind:              in.kind,
		StatusCode:        in.status,
		BizCode:           bizCode,
		ErrKind:           errKind,
		Method:            in.snap.Method,
		URL:               in.snap.URL,
		At:                recordTime(at),
		Ms:                ms,
		ReqHeader:         in.snap.Header,
		ReqBody:           in.snap.Body,
		ReqBodyTruncated:  in.snap.BodyTruncated,
		RespBytes:         in.size,
		RespBodyTruncated: in.truncated,
	}
	if in.err != nil {
		sample.Err = in.err.Error()
	}
	if in.resp != nil {
		sample.RespHeader = headerMap(in.resp.Header)
	}
	if len(in.body) > 0 {
		sample.RespBody = string(in.body)
	}
	return sample
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
