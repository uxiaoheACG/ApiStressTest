package clone

import (
	"bytes"
	"io"
	"net/http"
)

// CloneRequest 复制一份可安全并发使用的请求副本。
//
// 说明：原始 req 的 Body 会被读出后重置，副本通过 GetBody 提供独立的 Body 副本，
// 因此并发发送副本时不会出现多个 goroutine 抢读同一个 reader 的问题。
// 没有请求体（如 GET）时不会额外设置 Body，行为与原请求保持一致。
func CloneRequest(req *http.Request) (*http.Request, error) {
	if req == nil {
		return nil, nil
	}

	var bodyBytes []byte
	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		bodyBytes = data
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	clone := req.Clone(req.Context())
	if req.Body != nil {
		clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		clone.ContentLength = int64(len(bodyBytes))
		clone.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	return clone, nil
}
