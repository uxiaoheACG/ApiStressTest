package SendAndWrite

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// maxBodyBytes 单条记录里请求体/响应体最多保留的字节数，超出部分丢弃并标记截断
	maxBodyBytes = 4 << 10
)

// successRecord 成功请求的简写记录（一行一条 NDJSON）
type successRecord struct {
	Index  int    `json:"i"`
	Status int    `json:"s"`
	Ms     int64  `json:"ms"`
	Size   int64  `json:"size"`
	At     string `json:"at"`
}

// failureRecord 失败/超时请求的详细记录（SaveData=3 时成功请求也用这个结构）
type failureRecord struct {
	Index   int    `json:"i"`
	Kind    string `json:"kind"`              // unsuccess / timeout / error / success
	ErrKind string `json:"errKind,omitempty"` // deadline / canceled / dns / tls / eof / refused / reset / other
	Err     string `json:"err,omitempty"`

	Method string `json:"method"`
	URL    string `json:"url"`
	At     string `json:"at"`
	Ms     int64  `json:"ms"`

	ReqHeader        map[string]string `json:"reqHeader,omitempty"`
	ReqBody          string            `json:"reqBody,omitempty"`
	ReqBodyTruncated bool              `json:"reqBodyTruncated,omitempty"`

	Status            int               `json:"status"`
	RespHeader        map[string]string `json:"respHeader,omitempty"`
	RespBody          string            `json:"respBody,omitempty"`
	RespBodyTruncated bool              `json:"respBodyTruncated,omitempty"`
	RespBytes         int64             `json:"respBytes"`
}

// reqSnapshot 请求模板的只读快照：失败记录复用这一份，避免每次请求都去读 req.Body。
// Body 走 GetBody 读取，不会消费模板本身的 Body。
type reqSnapshot struct {
	Method        string
	URL           string
	Header        map[string]string
	Body          string
	BodyTruncated bool
}

// snapshotRequest 生成请求快照（header 里的凭证做掩码，body 截断）
func snapshotRequest(req *http.Request) reqSnapshot {
	snap := reqSnapshot{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: headerMap(req.Header),
	}
	body, truncated, err := readRequestBody(req)
	if err == nil {
		snap.Body = body
		snap.BodyTruncated = truncated
	}
	return snap
}

// readRequestBody 读取请求体（最多 maxBodyBytes 字节）
func readRequestBody(req *http.Request) (string, bool, error) {
	rc, err := bodyReader(req)
	if err != nil || rc == nil {
		return "", false, err
	}
	defer rc.Close()

	buf, err := io.ReadAll(io.LimitReader(rc, maxBodyBytes+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(buf) > maxBodyBytes
	if truncated {
		buf = buf[:maxBodyBytes]
	}
	return string(buf), truncated, nil
}

// bodyReader 优先用 GetBody 拿一份新副本，避免消费模板自身的 Body
func bodyReader(req *http.Request) (io.ReadCloser, error) {
	if req.GetBody != nil {
		return req.GetBody()
	}
	if req.Body != nil {
		return req.Body, nil
	}
	return nil, nil
}

// headerMap 把 http.Header 转成 map[string]string，并对凭证类字段打码
func headerMap(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	m := make(map[string]string, len(h))
	for key, values := range h {
		m[key] = maskHeader(key, strings.Join(values, ", "))
	}
	return m
}

// maskHeader 对 Authorization / Cookie 等敏感头做掩码，避免把凭证写进结果文件
func maskHeader(key, value string) string {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "x-api-key", "cookie", "set-cookie":
		if len(value) <= 8 {
			return "***"
		}
		return value[:4] + "***"
	default:
		return value
	}
}

// errKindOf 把请求错误归类，便于落盘后按类统计
func errKindOf(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "deadline"
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "eof"
	}

	if isTLSError(err) {
		return "tls"
	}

	// 连接级错误在不同平台的文案不一致（例如 Windows 的 "actively refused"），
	// 这里用关键字兜底，保证跨平台可用
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return "refused"
	case strings.Contains(msg, "reset"):
		return "reset"
	case strings.Contains(msg, "no such host"):
		return "dns"
	case strings.Contains(msg, "tls") || strings.Contains(msg, "certificate"):
		return "tls"
	default:
		return "other"
	}
}

// isTLSError 判断是否为 TLS/证书相关错误
func isTLSError(err error) bool {
	var verifyErr *tls.CertificateVerificationError
	if errors.As(err, &verifyErr) {
		return true
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return true
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}
	var certInvalid x509.CertificateInvalidError
	if errors.As(err, &certInvalid) {
		return true
	}
	return false
}

// readResponseBody 读取响应体：最多保留 maxBodyBytes 字节用于记录，其余读尽以便复用连接。
// 返回保留内容、是否截断、响应体总字节数、读取错误。
func readResponseBody(body io.Reader) (kept []byte, truncated bool, total int64, err error) {
	kept, err = io.ReadAll(io.LimitReader(body, maxBodyBytes+1))
	if err != nil {
		return nil, false, 0, err
	}

	total = int64(len(kept))
	if len(kept) > maxBodyBytes {
		kept = kept[:maxBodyBytes]
		truncated = true
	}

	rest, err := io.Copy(io.Discard, body)
	total += rest
	return kept, truncated, total, err
}

// marshalLine 把记录序列化成一行（带结尾换行），失败返回 nil
func marshalLine(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return append(data, '\n')
}

// recordTime 统一记录时间格式，精确到纳秒并带时区
func recordTime(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}
