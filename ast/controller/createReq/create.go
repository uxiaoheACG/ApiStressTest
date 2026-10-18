package createReq

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"ast/model"
)

// multipart 请求体的大小上限：单个文件解码后 16 MB，整体请求体 32 MB
const (
	maxMultipartFileBytes = 16 << 20
	maxMultipartBodyBytes = 32 << 20
)

// Create 根据压测配置构造一条 HTTP 请求，作为并发执行时的请求模板。
//
// 取值规则（对应 model.TestConfig 的字段）：
//   - Method  为空时按 post 处理；
//   - URL     必须非空；Params 以查询参数追加到 URL，同名参数覆盖 URL 上已有的值；
//   - Header  逐项写入请求头（键名由标准库统一规范大小写）；
//   - Body    按 Body.Type 处理：json（默认）、form、text / raw、multipart；
//   - Authorization.Type 支持 bearer / basic / apikey，为空表示不带认证。
//
// 这里只构造请求，不发送请求，请求使用 context.Background()；
// 需要超时或取消时由调用方用 req.WithContext(ctx)（并发时用 req.Clone(ctx) 派生副本）
// 配合 http.Client.Timeout 控制。因此 Timeout / Count / TotalRequests / Rate
// 这些执行期参数（config.Concurrency）不在这里使用。
func Create(config model.TestConfig) (*http.Request, error) {
	// 1. 请求方法：默认 GET
	method := strings.ToUpper(strings.TrimSpace(config.Method))
	if method == "" {
		method = http.MethodPost
	}

	// 2. 请求地址与查询参数
	if strings.TrimSpace(config.URL) == "" {
		return nil, errors.New("请求地址不能为空")
	}

	target, err := url.Parse(strings.TrimSpace(config.URL))
	if err != nil {
		return nil, fmt.Errorf("请求地址解析失败: %w", err)
	}

	if query := target.Query(); len(config.Params) > 0 {
		for key, value := range config.Params {
			query.Set(key, value)
		}
		target.RawQuery = query.Encode()
	}

	// 3. 请求体：json（默认）/ form / text / raw
	var body io.Reader
	contentType := ""

	switch strings.ToLower(strings.TrimSpace(config.Body.Type)) {
	case "json", "":
		if config.Body.Data != nil {
			data, err := json.Marshal(config.Body.Data)
			if err != nil {
				return nil, fmt.Errorf("请求体 JSON 序列化失败: %w", err)
			}
			body = bytes.NewReader(data)
			contentType = "application/json; charset=utf-8"
		}

	case "form":
		form := url.Values{}

		switch data := config.Body.Data.(type) {
		case nil:
			// 无请求体
		case map[string]string:
			for key, value := range data {
				form.Set(key, value)
			}
		case map[string]any:
			for key, value := range data {
				form.Set(key, fmt.Sprint(value))
			}
		default:
			return nil, fmt.Errorf("body.type=form 时 body.data 需为对象，实际类型为 %T", config.Body.Data)
		}

		body = strings.NewReader(form.Encode())
		contentType = "application/x-www-form-urlencoded"

	case "text", "raw":
		switch data := config.Body.Data.(type) {
		case nil:
			body = strings.NewReader("")
		case string:
			body = strings.NewReader(data)
		default:
			body = strings.NewReader(fmt.Sprint(data))
		}
		contentType = "text/plain; charset=utf-8"

	case "multipart":
		buf, formContentType, err := buildMultipartBody(config.Body.Data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
		contentType = formContentType

	default:
		return nil, fmt.Errorf("不支持的请求体类型: %q", config.Body.Type)
	}

	// 4. 构造请求
	req, err := http.NewRequestWithContext(context.Background(), method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// 5. 请求头（写在 Content-Type 之后，配置里的同名 Header 可以覆盖默认值）
	for key, value := range config.Header {
		req.Header.Set(key, value)
	}

	// 6. 认证信息
	switch strings.ToLower(strings.TrimSpace(config.Authorization.Type)) {
	case "":
		// 未配置认证

	case "bearer":
		req.Header.Set("Authorization", "Bearer "+config.Authorization.Token)

	case "basic":
		// token 写成 "user:pass" 时按账号密码处理，否则整体作为凭据做 base64
		if user, pass, ok := strings.Cut(config.Authorization.Token, ":"); ok {
			req.SetBasicAuth(user, pass)
		} else {
			req.Header.Set("Authorization", "Basic "+
				base64.StdEncoding.EncodeToString([]byte(config.Authorization.Token)))
		}

	case "apikey":
		req.Header.Set("X-API-Key", config.Authorization.Token)

	default:
		return nil, fmt.Errorf("不支持的认证类型: %q", config.Authorization.Type)
	}

	return req, nil
}

// buildMultipartBody 按 model.MultipartData 拼出 multipart/form-data 请求体，
// 返回字节内容与 Content-Type（含随机 boundary）。
//
// data 允许是前端直接传的对象（map[string]any），也允许是已解析的 model.MultipartData；
// 统一先做一次 JSON 往返，避免在多层 map 上做反射取值。
//
// 文件内容走 base64，因此后端不需要访问压测机的文件系统；单文件与整体体积都有上限，
// 超过时直接报错而不是静默截断（截断的文件会让被测服务收到损坏数据）。
func buildMultipartBody(data any) ([]byte, string, error) {
	if data == nil {
		return nil, "", errors.New("body.type=multipart 时 body.data 不能为空")
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, "", fmt.Errorf("multipart 数据序列化失败: %w", err)
	}

	var spec model.MultipartData
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, "", fmt.Errorf("body.type=multipart 时 body.data 需为 {fields, files} 结构: %w", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for key, value := range spec.Fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("写入表单字段 %q 失败: %w", key, err)
		}
	}

	for _, part := range spec.TextParts {
		field := strings.TrimSpace(part.Field)
		if field == "" {
			return nil, "", errors.New("multipart 文本字段缺少字段名 field")
		}

		partType := strings.TrimSpace(part.ContentType)
		if partType == "" {
			// 没指定 Content-Type 时按普通表单字段写入，与 Fields 行为一致
			if err := writer.WriteField(field, part.Value); err != nil {
				return nil, "", fmt.Errorf("写入表单字段 %q 失败: %w", field, err)
			}
			continue
		}

		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", formDataNameDisposition(field))
		header.Set("Content-Type", partType)

		w, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", fmt.Errorf("创建文本字段 %q 失败: %w", field, err)
		}
		if _, err := w.Write([]byte(part.Value)); err != nil {
			return nil, "", fmt.Errorf("写入文本字段 %q 失败: %w", field, err)
		}
	}

	for _, file := range spec.Files {
		field := strings.TrimSpace(file.Field)
		if field == "" {
			return nil, "", errors.New("multipart 文件缺少字段名 field")
		}

		content, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.ContentBase64))
		if err != nil {
			return nil, "", fmt.Errorf("文件 %q 的内容不是合法 base64: %w", file.Filename, err)
		}
		if len(content) > maxMultipartFileBytes {
			return nil, "", fmt.Errorf("文件 %q 超过单文件上限 %d MB", file.Filename, maxMultipartFileBytes>>20)
		}

		filename := strings.TrimSpace(file.Filename)
		if filename == "" {
			filename = field
		}

		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", formDataDisposition(field, filename))

		partType := strings.TrimSpace(file.ContentType)
		if partType == "" {
			partType = "application/octet-stream"
		}
		header.Set("Content-Type", partType)

		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", fmt.Errorf("创建文件字段 %q 失败: %w", field, err)
		}
		if _, err := part.Write(content); err != nil {
			return nil, "", fmt.Errorf("写入文件 %q 失败: %w", filename, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("结束 multipart 写入失败: %w", err)
	}

	if buf.Len() > maxMultipartBodyBytes {
		return nil, "", fmt.Errorf("multipart 请求体超过上限 %d MB", maxMultipartBodyBytes>>20)
	}

	return buf.Bytes(), writer.FormDataContentType(), nil
}

// formDataDisposition 生成 multipart 的 Content-Disposition 头。
//
// 浏览器对 name / filename 一律加引号，这里保持一致，兼容只认带引号形式的解析器；
// 含中文等非 ASCII 字符时退回 mime.FormatMediaType（按 RFC 2231 编码）。
func formDataDisposition(field, filename string) string {
	if isASCII(field) && isASCII(filename) &&
		!strings.ContainsAny(field, "\"\\") && !strings.ContainsAny(filename, "\"\\") {
		return `form-data; name="` + field + `"; filename="` + filename + `"`
	}
	return mime.FormatMediaType("form-data", map[string]string{
		"name":     field,
		"filename": filename,
	})
}

// isASCII 判断字符串是否全部为 ASCII 字符
func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// formDataNameDisposition 生成只有 name 的 Content-Disposition（文本段用）
func formDataNameDisposition(field string) string {
	if isASCII(field) && !strings.ContainsAny(field, "\"\\") {
		return `form-data; name="` + field + `"`
	}
	return mime.FormatMediaType("form-data", map[string]string{"name": field})
}
